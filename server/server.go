package server

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/config"
	"github.com/DrmagicE/gmqtt/persistence/queue"
	"github.com/DrmagicE/gmqtt/persistence/session"
	"github.com/DrmagicE/gmqtt/persistence/unack"
	"github.com/DrmagicE/gmqtt/pkg/codes"
	retained_trie "github.com/DrmagicE/gmqtt/retained/trie"

	"github.com/DrmagicE/gmqtt/persistence/subscription"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/DrmagicE/gmqtt/retained"
)

var (
	// ErrInvalWsMsgType [MQTT-6.0.0-1]
	ErrInvalWsMsgType    = errors.New("invalid websocket message type")
	statusPanic          = "invalid server status"
	plugins              = make(map[string]NewPlugin)
	topicAliasMgrFactory = make(map[string]NewTopicAliasManager)
	persistenceFactories = make(map[string]NewPersistence)
)

func defaultIterateOptions(topicName string) subscription.IterationOptions {
	return subscription.IterationOptions{
		Type:      subscription.TypeAll,
		TopicName: topicName,
		MatchType: subscription.MatchFilter,
	}
}

func RegisterPersistenceFactory(name string, new NewPersistence) {
	if _, ok := persistenceFactories[name]; ok {
		panic("duplicated persistence factory: " + name)
	}
	persistenceFactories[name] = new
}

func RegisterTopicAliasMgrFactory(name string, new NewTopicAliasManager) {
	if _, ok := topicAliasMgrFactory[name]; ok {
		panic("duplicated topic alias manager factory: " + name)
	}
	topicAliasMgrFactory[name] = new
}

func RegisterPlugin(name string, new NewPlugin) {
	if _, ok := plugins[name]; ok {
		panic("duplicated plugin: " + name)
	}
	plugins[name] = new
}

// Server status
const (
	serverStatusInit = iota
	serverStatusStarted
)

var zaplog *zap.Logger

func init() {
	zaplog = zap.NewNop()
}

// LoggerWithField release fields to a new logger.
// Plugins can use this method to release plugin name field.
func LoggerWithField(fields ...zap.Field) *zap.Logger {
	return zaplog.With(fields...)
}

// Server interface represents a mqtt server instance.
type Server interface {
	// Publisher returns the Publisher
	Publisher() Publisher
	// GetConfig returns the config of the server
	GetConfig() config.Config
	// StatsManager returns StatsReader
	StatsManager() StatsReader
	// Stop stop the server gracefully
	Stop(ctx context.Context) error
	// ApplyConfig will replace the config of the server
	ApplyConfig(config config.Config)

	ClientService() ClientService

	SubscriptionService() SubscriptionService

	RetainedService() RetainedService
	// Plugins returns all enabled plugins
	Plugins() []Plugin
	APIRegistrar() APIRegistrar
}

type clientService struct {
	srv          *server
	sessionStore session.Store
}

func (c *clientService) IterateSession(fn session.IterateFn) error {
	return c.sessionStore.Iterate(fn)
}

func (c *clientService) IterateClient(fn ClientIterateFn) {
	c.srv.mu.Lock()
	defer c.srv.mu.Unlock()

	for _, v := range c.srv.clients {
		if !fn(v) {
			return
		}
	}
}

func (c *clientService) GetClient(clientID string) Client {
	c.srv.mu.Lock()
	defer c.srv.mu.Unlock()
	if c, ok := c.srv.clients[clientID]; ok {
		return c
	}
	return nil
}

func (c *clientService) GetSession(clientID string) (*gmqtt.Session, error) {
	return c.sessionStore.Get(clientID)
}

func (c *clientService) TerminateSession(clientID string) {
	c.srv.mu.Lock()
	defer c.srv.mu.Unlock()
	if cli, ok := c.srv.clients[clientID]; ok {
		atomic.StoreInt32(&cli.forceRemoveSession, 1)
		cli.Close()
		return
	}
	if _, ok := c.srv.offlineClients[clientID]; ok {
		err := c.srv.sessionTerminatedLocked(clientID, NormalTermination)
		if err != nil {
			err = fmt.Errorf("session terminated fail: %s", err.Error())
			zaplog.Error("session terminated fail", zap.Error(err))
		}
	}

}

// server represents a mqtt server instance.
// Create a server by using New()
type server struct {
	wg       sync.WaitGroup
	initOnce sync.Once
	stopOnce sync.Once
	mu       sync.RWMutex //gard clients & offlineClients map
	status   int32        //server status
	// clients stores the  online clients
	clients map[string]*client
	// offlineClients store the expired time of all disconnected clients
	// with valid session(not expired). Key by clientID
	offlineClients  map[string]time.Time
	willMessage     map[string]*willMsg
	tcpListener     []net.Listener //tcp listeners
	websocketServer []*WsServer    //websocket serverStop
	errOnce         sync.Once
	err             error
	exitChan        chan struct{}
	exitedChan      chan struct{}

	retainedDB      retained.Store
	subscriptionsDB subscription.Store //store subscriptions

	persistence  Persistence
	queueStore   map[string]queue.Store
	unackStore   map[string]unack.Store
	sessionStore session.Store

	// guards config
	configMu             sync.RWMutex
	config               config.Config
	hooks                Hooks
	plugins              []Plugin
	statsManager         *statsManager
	publishService       Publisher
	newTopicAliasManager NewTopicAliasManager

	clientService *clientService
	apiRegistrar  *apiRegistrar
}

func (srv *server) APIRegistrar() APIRegistrar {
	return srv.apiRegistrar
}

func (srv *server) Plugins() []Plugin {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	p := make([]Plugin, len(srv.plugins))
	copy(p, srv.plugins)
	return p

}

func (srv *server) RetainedService() RetainedService {
	return srv.retainedDB
}

func (srv *server) ClientService() ClientService {
	return srv.clientService
}

func (srv *server) ApplyConfig(config config.Config) {
	srv.configMu.Lock()
	defer srv.configMu.Unlock()
	srv.config = config

}

func (srv *server) SubscriptionService() SubscriptionService {
	return srv.subscriptionsDB
}

func (srv *server) RetainedStore() retained.Store {
	return srv.retainedDB
}

func (srv *server) Publisher() Publisher {
	return srv.publishService
}

func (srv *server) checkStatus() {
	if srv.Status() != serverStatusInit {
		panic(statusPanic)
	}
}

type DeliveryMode = string

const (
	Overlap  DeliveryMode = "overlap"
	OnlyOnce DeliveryMode = "onlyonce"
)

// GetConfig returns the config of the server
func (srv *server) GetConfig() config.Config {
	srv.configMu.Lock()
	defer srv.configMu.Unlock()
	return srv.config
}

// StatsManager returns StatsReader
func (srv *server) StatsManager() StatsReader {
	return srv.statsManager
}

// Status returns the server status
func (srv *server) Status() int32 {
	return atomic.LoadInt32(&srv.status)
}

func (srv *server) sessionTerminatedLocked(clientID string, reason SessionTerminatedReason) (err error) {
	err = srv.removeSessionLocked(clientID)
	if srv.hooks.OnSessionTerminated != nil {
		srv.hooks.OnSessionTerminated(context.Background(), clientID, reason)
	}
	srv.statsManager.sessionTerminated(clientID, reason)
	return err
}

func uint16P(v uint16) *uint16 {
	return &v
}

func setWillProperties(willPpt *packets.Properties, msg *gmqtt.Message) {
	if willPpt != nil {
		if willPpt.PayloadFormat != nil {
			msg.PayloadFormat = *willPpt.PayloadFormat
		}
		if willPpt.MessageExpiry != nil {
			msg.MessageExpiry = *willPpt.MessageExpiry
		}
		if willPpt.ContentType != nil {
			msg.ContentType = string(willPpt.ContentType)
		}
		if willPpt.ResponseTopic != nil {
			msg.ResponseTopic = string(willPpt.ResponseTopic)
		}
		if willPpt.CorrelationData != nil {
			msg.CorrelationData = willPpt.CorrelationData
		}
		msg.UserProperties = willPpt.User
	}
}

func (srv *server) lockDuplicatedID(c *client) (oldSession *gmqtt.Session, err error) {
	for {
		srv.mu.Lock()
		oldSession, err = srv.sessionStore.Get(c.opts.ClientID)
		if err != nil {
			srv.mu.Unlock()
			zaplog.Error("fail to get session",
				zap.String("remote_addr", c.rwc.RemoteAddr().String()),
				zap.String("client_id", c.opts.ClientID))
			return
		}
		if oldSession != nil {
			var oldClient *client
			oldClient = srv.clients[oldSession.ClientID]
			srv.mu.Unlock()
			if oldClient == nil {
				srv.mu.Lock()
				break
			}
			// if there is a duplicated online client, close if first.
			zaplog.Info("logging with duplicate ClientID",
				zap.String("remote", c.rwc.RemoteAddr().String()),
				zap.String("client_id", oldSession.ClientID),
			)
			oldClient.setError(codes.NewError(codes.SessionTakenOver))
			oldClient.Close()
			<-oldClient.closed
			continue
		}
		break
	}
	return
}

// 已经判断是成功了，注册
func (srv *server) registerClient(connect *packets.Connect, client *client) (sessionResume bool, err error) {
	var qs queue.Store
	var ua unack.Store
	var sess *gmqtt.Session
	var oldSession *gmqtt.Session
	now := time.Now()
	oldSession, err = srv.lockDuplicatedID(client)
	if err != nil {
		return
	}
	defer func() {
		if err == nil {
			var willMsg *gmqtt.Message
			var willDelayInterval, expiryInterval uint32
			if connect.WillFlag {
				willMsg = &gmqtt.Message{
					QoS:     connect.WillQos,
					Topic:   string(connect.WillTopic),
					Payload: connect.WillMsg,
				}
				setWillProperties(connect.WillProperties, willMsg)
			}
			// use default expiry if the client version is version3.1.1
			if packets.IsVersion3X(client.version) && !connect.CleanStart {
				expiryInterval = uint32(srv.config.MQTT.SessionExpiry.Seconds())
			} else if connect.Properties != nil {
				willDelayInterval = convertUint32(connect.WillProperties.WillDelayInterval, 0)
				expiryInterval = client.opts.SessionExpiry
			}
			sess = &gmqtt.Session{
				ClientID:          client.opts.ClientID,
				Will:              willMsg,
				ConnectedAt:       time.Now(),
				WillDelayInterval: willDelayInterval,
				ExpiryInterval:    expiryInterval,
			}
			err = srv.sessionStore.Set(sess)
		}

		if err == nil {
			client.session = sess
			if sessionResume {
				// If a new Network Connection to this Session is made before the Will Delay Interval has passed,
				// the Server MUST NOT send the Will Message [MQTT-3.1.3-9].
				if w, ok := srv.willMessage[client.opts.ClientID]; ok {
					w.signal(false)
				}
				if srv.hooks.OnSessionResumed != nil {
					srv.hooks.OnSessionResumed(context.Background(), client)
				}
				srv.statsManager.sessionActive(false)
			} else {
				if srv.hooks.OnSessionCreated != nil {
					srv.hooks.OnSessionCreated(context.Background(), client)
				}
				srv.statsManager.sessionActive(true)
			}
			srv.clients[client.opts.ClientID] = client
			srv.unackStore[client.opts.ClientID] = ua
			srv.queueStore[client.opts.ClientID] = qs
			client.queueStore = qs
			client.unackStore = ua
			if client.version == packets.Version5 {
				client.topicAliasManager = srv.newTopicAliasManager(client.config, client.opts.ClientTopicAliasMax, client.opts.ClientID)
			}
		}
		srv.mu.Unlock()
	}()

	client.setConnected(time.Now())
	if srv.hooks.OnConnected != nil {
		srv.hooks.OnConnected(context.Background(), client)
	}
	srv.statsManager.clientConnected(client.opts.ClientID)

	if oldSession != nil {
		if !oldSession.IsExpired(now) && !connect.CleanStart {
			sessionResume = true
		}
		// clean old session
		if !sessionResume {
			err = srv.sessionTerminatedLocked(oldSession.ClientID, TakenOverTermination)
			if err != nil {
				err = fmt.Errorf("session terminated fail: %w", err)
				zaplog.Error("session terminated fail", zap.Error(err))
			}
			// Send will message because the previous session is ended.
			if w, ok := srv.willMessage[client.opts.ClientID]; ok {
				w.signal(true)
			}
		} else {
			qs = srv.queueStore[client.opts.ClientID]
			if qs != nil {
				err = qs.Init(&queue.InitOptions{
					CleanStart:     false,
					Version:        client.version,
					ReadBytesLimit: client.opts.ClientMaxPacketSize,
					Notifier:       client.queueNotifier,
				})
				if err != nil {
					return
				}
			}
			ua = srv.unackStore[client.opts.ClientID]
			if ua != nil {
				err = ua.Init(false)
				if err != nil {
					return
				}
			}
			if ua == nil || qs == nil {
				// This could happen if backend store loss some data which will bring the session into "inconsistent state".
				// We should create a new session and prevent the client reuse the inconsistent one.
				sessionResume = false
				zaplog.Error("detect inconsistent session state",
					zap.String("remote_addr", client.rwc.RemoteAddr().String()),
					zap.String("client_id", client.opts.ClientID))
			} else {
				zaplog.Info("logged in with session reuse",
					zap.String("remote_addr", client.rwc.RemoteAddr().String()),
					zap.String("client_id", client.opts.ClientID))
			}

		}
	}
	if !sessionResume {
		// create new session
		// It is ok to pass nil to defaultNotifier, because we will call Init to override it.
		qs, err = srv.persistence.NewQueueStore(srv.config, nil, client.opts.ClientID)
		if err != nil {
			return
		}
		err = qs.Init(&queue.InitOptions{
			CleanStart:     true,
			Version:        client.version,
			ReadBytesLimit: client.opts.ClientMaxPacketSize,
			Notifier:       client.queueNotifier,
		})
		if err != nil {
			return
		}

		ua, err = srv.persistence.NewUnackStore(srv.config, client.opts.ClientID)
		if err != nil {
			return
		}
		err = ua.Init(true)
		if err != nil {
			return
		}
		zaplog.Info("logged in with new session",
			zap.String("remote_addr", client.rwc.RemoteAddr().String()),
			zap.String("client_id", client.opts.ClientID),
		)
	}
	delete(srv.offlineClients, client.opts.ClientID)
	return
}

type willMsg struct {
	msg *gmqtt.Message
	// If true, send the msg.
	// If false, discard the msg.
	send chan bool
}

func (w *willMsg) signal(send bool) {
	select {
	case w.send <- send:
	default:
	}
}

// sendWillLocked sends the will message for the client, this function must be guard by srv.Lock.
func (srv *server) sendWillLocked(msg *gmqtt.Message, clientID string) {
	req := &WillMsgRequest{
		Message: msg,
	}
	if srv.hooks.OnWillPublish != nil {
		srv.hooks.OnWillPublish(context.Background(), clientID, req)
	}
	// the will message is dropped
	if req.Message == nil {
		return
	}
	srv.deliverMessage(clientID, msg, defaultIterateOptions(msg.Topic))
	if srv.hooks.OnWillPublished != nil {
		srv.hooks.OnWillPublished(context.Background(), clientID, req.Message)
	}
}

func (srv *server) unregisterClient(client *client) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	now := time.Now()
	var storeSession bool
	if sess, err := srv.sessionStore.Get(client.opts.ClientID); sess != nil {
		forceRemove := atomic.LoadInt32(&client.forceRemoveSession)
		if forceRemove != 1 {
			if client.version == packets.Version5 && client.disconnect != nil {
				sess.ExpiryInterval = convertUint32(client.disconnect.Properties.SessionExpiryInterval, sess.ExpiryInterval)
			}
			if sess.ExpiryInterval != 0 {
				storeSession = true
			}
		}
		// need to send will message
		if !client.cleanWillFlag && sess.Will != nil {
			willDelayInterval := sess.WillDelayInterval
			if sess.ExpiryInterval <= sess.WillDelayInterval {
				willDelayInterval = sess.ExpiryInterval
			}
			msg := sess.Will.Copy()
			if willDelayInterval != 0 && storeSession {
				wm := &willMsg{
					msg:  msg,
					send: make(chan bool, 1),
				}
				srv.willMessage[client.opts.ClientID] = wm
				t := time.NewTimer(time.Duration(willDelayInterval) * time.Second)
				go func(clientID string) {
					var send bool
					select {
					case send = <-wm.send:
						t.Stop()
					case <-t.C:
						send = true
					}
					srv.mu.Lock()
					defer srv.mu.Unlock()
					delete(srv.willMessage, clientID)
					if !send {
						return
					}
					srv.sendWillLocked(msg, clientID)
				}(client.opts.ClientID)
			} else {
				srv.sendWillLocked(msg, client.opts.ClientID)
			}
		}
		if storeSession {
			expiredTime := now.Add(time.Duration(sess.ExpiryInterval) * time.Second)
			srv.offlineClients[client.opts.ClientID] = expiredTime
			delete(srv.clients, client.opts.ClientID)
			zaplog.Info("logged out and storing session",
				zap.String("remote_addr", client.rwc.RemoteAddr().String()),
				zap.String("client_id", client.opts.ClientID),
				zap.Time("expired_at", expiredTime),
			)
			return
		}
	} else {
		zaplog.Error("fail to get session",
			zap.String("remote_addr", client.rwc.RemoteAddr().String()),
			zap.String("client_id", client.opts.ClientID),
			zap.Error(err))
	}
	zaplog.Info("logged out and cleaning session",
		zap.String("remote_addr", client.rwc.RemoteAddr().String()),
		zap.String("client_id", client.opts.ClientID),
	)
	_ = srv.sessionTerminatedLocked(client.opts.ClientID, NormalTermination)
}

func (srv *server) addMsgToQueueLocked(now time.Time, clientID string, msg *gmqtt.Message, sub *gmqtt.Subscription, ids []uint32, q queue.Store) {
	mqttCfg := srv.config.MQTT
	if !mqttCfg.QueueQos0Msg {
		// If the client with the clientID is not connected, skip qos0 messages.
		if c := srv.clients[clientID]; c == nil && msg.QoS == packets.Qos0 {
			return
		}
	}
	if msg.QoS > sub.QoS {
		msg.QoS = sub.QoS
	}
	for _, id := range ids {
		if id != 0 {
			msg.SubscriptionIdentifier = append(msg.SubscriptionIdentifier, id)
		}
	}
	msg.Dup = false
	if !sub.RetainAsPublished {
		msg.Retained = false
	}
	var expiry time.Time
	if mqttCfg.MessageExpiry != 0 {
		if msg.MessageExpiry != 0 && int(msg.MessageExpiry) <= int(mqttCfg.MessageExpiry) {
			expiry = now.Add(time.Duration(msg.MessageExpiry) * time.Second)
		} else {
			expiry = now.Add(mqttCfg.MessageExpiry)
		}
	} else if msg.MessageExpiry != 0 {
		expiry = now.Add(time.Duration(msg.MessageExpiry) * time.Second)
	}
	err := q.Add(&queue.Elem{
		At:     now,
		Expiry: expiry,
		MessageWithID: &queue.Publish{
			Message: msg,
		},
	})
	if err != nil {
		srv.clients[clientID].queueNotifier.notifyDropped(msg, &queue.InternalError{Err: err})
		return
	}
}

// sharedList is the subscriber (client id) list of shared subscriptions. (key by topic name).
type sharedList map[string][]struct {
	clientID string
	sub      *gmqtt.Subscription
}

// maxQos records the maximum qos subscription for the non-shared topic. (key by topic name).
type maxQos map[string]*struct {
	sub    *gmqtt.Subscription
	subIDs []uint32
}

// deliverHandler controllers the delivery behaviors according to the DeliveryMode config. (overlap or onlyonce)
type deliverHandler struct {
	fn      subscription.IterateFn
	sl      sharedList
	mq      maxQos
	matched bool
	now     time.Time
	msg     *gmqtt.Message
	srv     *server
}

func newDeliverHandler(mode string, srcClientID string, msg *gmqtt.Message, now time.Time, srv *server) *deliverHandler {
	d := &deliverHandler{
		sl:  make(sharedList),
		mq:  make(maxQos),
		msg: msg,
		srv: srv,
		now: now,
	}
	var iterateFn subscription.IterateFn
	d.fn = func(clientID string, sub *gmqtt.Subscription) bool {
		if sub.NoLocal && clientID == srcClientID {
			return true
		}
		d.matched = true
		if sub.ShareName != "" {
			fullTopic := sub.GetFullTopicName()
			d.sl[fullTopic] = append(d.sl[fullTopic], struct {
				clientID string
				sub      *gmqtt.Subscription
			}{clientID: clientID, sub: sub})
			return true
		}
		return iterateFn(clientID, sub)
	}
	if mode == Overlap {
		iterateFn = func(clientID string, sub *gmqtt.Subscription) bool {
			if qs := srv.queueStore[clientID]; qs != nil {
				srv.addMsgToQueueLocked(now, clientID, msg.Copy(), sub, []uint32{sub.ID}, qs)
			}
			return true
		}
	} else {
		iterateFn = func(clientID string, sub *gmqtt.Subscription) bool {
			// If the delivery mode is onlyOnce, set the message qos to the maximum qos in matched subscriptions.
			if d.mq[clientID] == nil {
				d.mq[clientID] = &struct {
					sub    *gmqtt.Subscription
					subIDs []uint32
				}{sub: sub, subIDs: []uint32{sub.ID}}
				return true
			}
			if d.mq[clientID].sub.QoS < sub.QoS {
				d.mq[clientID].sub = sub
			}
			d.mq[clientID].subIDs = append(d.mq[clientID].subIDs, sub.ID)
			return true
		}
	}
	return d
}

func (d *deliverHandler) flush() {
	// shared subscription
	// TODO enable customize balance strategy of shared subscription
	for _, v := range d.sl {
		var rs struct {
			clientID string
			sub      *gmqtt.Subscription
		}
		// random
		rs = v[rand.Intn(len(v))]
		if c, ok := d.srv.queueStore[rs.clientID]; ok {
			d.srv.addMsgToQueueLocked(d.now, rs.clientID, d.msg.Copy(), rs.sub, []uint32{rs.sub.ID}, c)
		}
	}
	// For onlyonce mode, send the non-shared messages.
	for clientID, v := range d.mq {
		if qs := d.srv.queueStore[clientID]; qs != nil {
			d.srv.addMsgToQueueLocked(d.now, clientID, d.msg.Copy(), v.sub, v.subIDs, qs)
		}
	}
}

// deliverMessage send msg to matched client, must call under srv.mu.Lock
func (srv *server) deliverMessage(srcClientID string, msg *gmqtt.Message, options subscription.IterationOptions) (matched bool) {
	now := time.Now()
	d := newDeliverHandler(srv.config.MQTT.DeliveryMode, srcClientID, msg, now, srv)
	srv.subscriptionsDB.Iterate(d.fn, options)
	d.flush()
	return d.matched
}

func (srv *server) removeSessionLocked(clientID string) (err error) {
	delete(srv.clients, clientID)
	delete(srv.offlineClients, clientID)

	var errs []string
	var queueErr, sessionErr, subErr error
	if qs := srv.queueStore[clientID]; qs != nil {
		queueErr = qs.Clean()
		if queueErr != nil {
			zaplog.Error("fail to clean message queue",
				zap.String("client_id", clientID),
				zap.Error(queueErr))
			errs = append(errs, "fail to clean message queue: "+queueErr.Error())
		}
		delete(srv.queueStore, clientID)
	}
	sessionErr = srv.sessionStore.Remove(clientID)
	if sessionErr != nil {
		zaplog.Error("fail to remove session",
			zap.String("client_id", clientID),
			zap.Error(sessionErr))

		errs = append(errs, "fail to remove session: "+sessionErr.Error())
	}
	subErr = srv.subscriptionsDB.UnsubscribeAll(clientID)
	if subErr != nil {
		zaplog.Error("fail to remove subscription",
			zap.String("client_id", clientID),
			zap.Error(subErr))

		errs = append(errs, "fail to remove subscription: "+subErr.Error())
	}

	if errs != nil {
		return errors.New(strings.Join(errs, ";"))
	}
	return nil
}

// sessionExpireCheck 判断是否超时
// sessionExpireCheck check and terminate expired sessions
func (srv *server) sessionExpireCheck() {
	now := time.Now()
	srv.mu.Lock()
	for cid, expiredTime := range srv.offlineClients {
		if now.After(expiredTime) {
			zaplog.Info("session expired", zap.String("client_id", cid))
			_ = srv.sessionTerminatedLocked(cid, ExpiredTermination)

		}
	}
	srv.mu.Unlock()
}

// server event loop
func (srv *server) eventLoop() {
	sessionExpireTimer := time.NewTicker(time.Second * 20)
	defer func() {
		sessionExpireTimer.Stop()
		srv.wg.Done()
	}()
	for {
		select {
		case <-srv.exitChan:
			return
		case <-sessionExpireTimer.C:
			srv.sessionExpireCheck()
		}

	}
}

// WsServer is used to build websocket server
type WsServer struct {
	Server   *http.Server
	Path     string // Url path
	CertFile string //TLS configration
	KeyFile  string //TLS configration
}

func defaultServer() *server {
	srv := &server{
		status:         serverStatusInit,
		exitChan:       make(chan struct{}),
		exitedChan:     make(chan struct{}),
		clients:        make(map[string]*client),
		offlineClients: make(map[string]time.Time),
		willMessage:    make(map[string]*willMsg),
		retainedDB:     retained_trie.NewStore(),
		config:         config.DefaultConfig(),
		queueStore:     make(map[string]queue.Store),
		unackStore:     make(map[string]unack.Store),
	}
	srv.publishService = &publishService{server: srv}
	return srv
}

// New returns a gmqtt server instance with the given options
func New(opts ...Options) *server {
	srv := defaultServer()
	for _, fn := range opts {
		fn(srv)
	}
	return srv
}

func (srv *server) init(opts ...Options) (err error) {
	for _, fn := range opts {
		fn(srv)
	}
	err = srv.initPluginHooks()
	if err != nil {
		return err
	}
	var pe Persistence
	peType := srv.config.Persistence.Type
	if newFn := persistenceFactories[peType]; newFn != nil {
		pe, err = newFn(srv.config)
		if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("persistence factory: %s not found", peType)
	}
	err = pe.Open()
	if err != nil {
		return err
	}
	zaplog.Info("open persistence succeeded", zap.String("type", peType))
	srv.persistence = pe

	srv.subscriptionsDB, err = srv.persistence.NewSubscriptionStore(srv.config)
	if err != nil {
		return err
	}
	st, err := srv.persistence.NewSessionStore(srv.config)
	if err != nil {
		return err
	}
	srv.sessionStore = st
	var sts []*gmqtt.Session
	var cids []string

	err = st.Iterate(func(session *gmqtt.Session) bool {
		sts = append(sts, session)
		cids = append(cids, session.ClientID)
		return true
	})
	if err != nil {
		return err
	}
	zaplog.Info("init session store succeeded", zap.String("type", peType), zap.Int("session_total", len(cids)))

	srv.statsManager = newStatsManager(srv.subscriptionsDB)
	srv.clientService = &clientService{
		srv:          srv,
		sessionStore: srv.sessionStore,
	}

	// init queue store & unack store from persistence
	for _, v := range sts {
		q, err := srv.persistence.NewQueueStore(srv.config, defaultNotifier(srv.hooks.OnMsgDropped, srv.statsManager, v.ClientID), v.ClientID)
		if err != nil {
			return err
		}
		srv.queueStore[v.ClientID] = q
		srv.offlineClients[v.ClientID] = time.Now().Add(time.Duration(v.ExpiryInterval) * time.Second)

		ua, err := srv.persistence.NewUnackStore(srv.config, v.ClientID)
		if err != nil {
			return err
		}
		srv.unackStore[v.ClientID] = ua
	}
	zaplog.Info("init queue store succeeded", zap.String("type", peType), zap.Int("session_total", len(cids)))
	zaplog.Info("init subscription store succeeded", zap.String("type", peType), zap.Int("client_total", len(cids)))
	err = srv.subscriptionsDB.Init(cids)
	if err != nil {
		return err
	}

	topicAliasMgrFactory := topicAliasMgrFactory[srv.config.TopicAliasManager.Type]
	if topicAliasMgrFactory != nil {
		srv.newTopicAliasManager = topicAliasMgrFactory
	} else {
		return fmt.Errorf("topic alias manager : %s not found", srv.config.TopicAliasManager.Type)
	}
	err = srv.initAPIRegistrar()
	if err != nil {
		return err
	}
	return srv.loadPlugins()
}

func (srv *server) initAPIRegistrar() error {
	registrar := &apiRegistrar{}
	for _, v := range srv.config.API.HTTP {
		server, err := buildHTTPServer(v)
		if err != nil {
			return err
		}
		registrar.httpServers = append(registrar.httpServers, server)

	}
	for _, v := range srv.config.API.GRPC {
		server, err := buildGRPCServer(v)
		if err != nil {
			return err
		}
		registrar.gRPCServers = append(registrar.gRPCServers, server)
	}
	srv.apiRegistrar = registrar
	return nil
}

// Init initialises the options.
func (srv *server) Init(opts ...Options) (err error) {
	srv.initOnce.Do(func() {
		err = srv.init(opts...)
	})
	return err
}

// Client returns the client for given clientID
func (srv *server) Client(clientID string) Client {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	return srv.clients[clientID]
}

func (srv *server) serveTCP(l net.Listener) {
	defer func() {
		l.Close()
	}()
	var tempDelay time.Duration
	for {
		rw, e := l.Accept()
		if e != nil {
			if ne, ok := e.(net.Error); ok && ne.Temporary() {
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}
				if max := 1 * time.Second; tempDelay > max {
					tempDelay = max
				}
				time.Sleep(tempDelay)
				continue
			}
			return
		}
		if srv.hooks.OnAccept != nil {
			if !srv.hooks.OnAccept(context.Background(), rw) {
				rw.Close()
				continue
			}
		}
		client, err := srv.newClient(rw)
		if err != nil {
			zaplog.Error("new client fail", zap.Error(err))
			return
		}
		go client.serve()
	}
}

var defaultUpgrader = &websocket.Upgrader{
	ReadBufferSize:  readBufferSize,
	WriteBufferSize: writeBufferSize,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
	Subprotocols: []string{"mqtt"},
}

// 实现io.ReadWriter接口
// wsConn implements the io.readWriter
type wsConn struct {
	net.Conn
	c   *websocket.Conn
	buf []byte
	r   int // buf copy positions
}

func (ws *wsConn) Close() error {
	return ws.Conn.Close()
}

func (ws *wsConn) Read(p []byte) (n int, err error) {
	if ws.buf == nil {
		msgType, buf, err := ws.c.ReadMessage()
		if err != nil {
			return 0, err
		}
		if msgType != websocket.BinaryMessage {
			return 0, ErrInvalWsMsgType
		}
		ws.buf = buf
	}
	n = copy(p, ws.buf[ws.r:])
	ws.r += n
	// reset reader buffer
	if ws.r+1 >= len(ws.buf) {
		ws.buf = nil
		ws.r = 0
	}
	return
}

func (ws *wsConn) Write(p []byte) (n int, err error) {
	err = ws.c.WriteMessage(websocket.BinaryMessage, p)
	if err != nil {
		return 0, err
	}
	return len(p), err
}

func (srv *server) serveWebSocket(ws *WsServer) {
	var err error
	if ws.CertFile != "" && ws.KeyFile != "" {
		err = ws.Server.ListenAndServeTLS(ws.CertFile, ws.KeyFile)
	} else {
		err = ws.Server.ListenAndServe()
	}
	if err != nil && err != http.ErrServerClosed {
		srv.setError(fmt.Errorf("serveWebSocket error: %s", err.Error()))
	}
}

func (srv *server) newClient(c net.Conn) (*client, error) {
	srv.configMu.Lock()
	cfg := srv.config
	srv.configMu.Unlock()
	client := &client{
		server:        srv,
		rwc:           c,
		bufr:          newBufioReaderSize(c, readBufferSize),
		bufw:          newBufioWriterSize(c, writeBufferSize),
		close:         make(chan struct{}),
		closed:        make(chan struct{}),
		connected:     make(chan struct{}),
		error:         make(chan error, 1),
		in:            make(chan packets.Packet, 8),
		out:           make(chan packets.Packet, 8),
		status:        Connecting,
		opts:          &ClientOptions{},
		cleanWillFlag: false,
		config:        cfg,
		register:      srv.registerClient,
		unregister:    srv.unregisterClient,
		deliverMessage: func(srcClientID string, msg *gmqtt.Message, options subscription.IterationOptions) (matched bool) {
			srv.mu.Lock()
			defer srv.mu.Unlock()
			return srv.deliverMessage(srcClientID, msg, options)
		},
	}
	client.packetReader = packets.NewReader(client.bufr)
	client.packetWriter = packets.NewWriter(client.bufw)
	client.queueNotifier = &queueNotifier{
		dropHook: srv.hooks.OnMsgDropped,
		sts:      srv.statsManager,
		cli:      client,
	}
	client.setConnecting()

	return client, nil
}

func (srv *server) initPluginHooks() error {
	zaplog.Info("init plugin hook wrappers")
	var (
		onAcceptWrappers           []OnAcceptWrapper
		onBasicAuthWrappers        []OnBasicAuthWrapper
		onEnhancedAuthWrappers     []OnEnhancedAuthWrapper
		onReAuthWrappers           []OnReAuthWrapper
		onConnectedWrappers        []OnConnectedWrapper
		onSessionCreatedWrapper    []OnSessionCreatedWrapper
		onSessionResumedWrapper    []OnSessionResumedWrapper
		onSessionTerminatedWrapper []OnSessionTerminatedWrapper
		onSubscribeWrappers        []OnSubscribeWrapper
		onSubscribedWrappers       []OnSubscribedWrapper
		onUnsubscribeWrappers      []OnUnsubscribeWrapper
		onUnsubscribedWrappers     []OnUnsubscribedWrapper
		onMsgArrivedWrappers       []OnMsgArrivedWrapper
		OnDeliveredWrappers        []OnDeliveredWrapper
		OnClosedWrappers           []OnClosedWrapper
		onStopWrappers             []OnStopWrapper
		onMsgDroppedWrappers       []OnMsgDroppedWrapper
		onWillPublishWrappers      []OnWillPublishWrapper
		onWillPublishedWrappers    []OnWillPublishedWrapper
	)
	for _, v := range srv.config.PluginOrder {
		plg, err := plugins[v](srv.config)
		if err != nil {
			return err
		}
		srv.plugins = append(srv.plugins, plg)
	}

	for _, p := range srv.plugins {
		hooks := p.HookWrapper()
		// init all hook wrappers
		if hooks.OnAcceptWrapper != nil {
			onAcceptWrappers = append(onAcceptWrappers, hooks.OnAcceptWrapper)
		}
		if hooks.OnBasicAuthWrapper != nil {
			onBasicAuthWrappers = append(onBasicAuthWrappers, hooks.OnBasicAuthWrapper)
		}
		if hooks.OnEnhancedAuthWrapper != nil {
			onEnhancedAuthWrappers = append(onEnhancedAuthWrappers, hooks.OnEnhancedAuthWrapper)
		}
		if hooks.OnReAuthWrapper != nil {
			onReAuthWrappers = append(onReAuthWrappers, hooks.OnReAuthWrapper)
		}
		if hooks.OnConnectedWrapper != nil {
			onConnectedWrappers = append(onConnectedWrappers, hooks.OnConnectedWrapper)
		}
		if hooks.OnSessionCreatedWrapper != nil {
			onSessionCreatedWrapper = append(onSessionCreatedWrapper, hooks.OnSessionCreatedWrapper)
		}
		if hooks.OnSessionResumedWrapper != nil {
			onSessionResumedWrapper = append(onSessionResumedWrapper, hooks.OnSessionResumedWrapper)
		}
		if hooks.OnSessionTerminatedWrapper != nil {
			onSessionTerminatedWrapper = append(onSessionTerminatedWrapper, hooks.OnSessionTerminatedWrapper)
		}
		if hooks.OnSubscribeWrapper != nil {
			onSubscribeWrappers = append(onSubscribeWrappers, hooks.OnSubscribeWrapper)
		}
		if hooks.OnSubscribedWrapper != nil {
			onSubscribedWrappers = append(onSubscribedWrappers, hooks.OnSubscribedWrapper)
		}
		if hooks.OnUnsubscribeWrapper != nil {
			onUnsubscribeWrappers = append(onUnsubscribeWrappers, hooks.OnUnsubscribeWrapper)
		}
		if hooks.OnUnsubscribedWrapper != nil {
			onUnsubscribedWrappers = append(onUnsubscribedWrappers, hooks.OnUnsubscribedWrapper)
		}
		if hooks.OnMsgArrivedWrapper != nil {
			onMsgArrivedWrappers = append(onMsgArrivedWrappers, hooks.OnMsgArrivedWrapper)
		}
		if hooks.OnMsgDroppedWrapper != nil {
			onMsgDroppedWrappers = append(onMsgDroppedWrappers, hooks.OnMsgDroppedWrapper)
		}
		if hooks.OnDeliveredWrapper != nil {
			OnDeliveredWrappers = append(OnDeliveredWrappers, hooks.OnDeliveredWrapper)
		}
		if hooks.OnClosedWrapper != nil {
			OnClosedWrappers = append(OnClosedWrappers, hooks.OnClosedWrapper)
		}
		if hooks.OnStopWrapper != nil {
			onStopWrappers = append(onStopWrappers, hooks.OnStopWrapper)
		}
		if hooks.OnWillPublishWrapper != nil {
			onWillPublishWrappers = append(onWillPublishWrappers, hooks.OnWillPublishWrapper)
		}
		if hooks.OnWillPublishedWrapper != nil {
			onWillPublishedWrappers = append(onWillPublishedWrappers, hooks.OnWillPublishedWrapper)
		}
	}
	if onAcceptWrappers != nil {
		onAccept := func(ctx context.Context, conn net.Conn) bool {
			return true
		}
		for i := len(onAcceptWrappers); i > 0; i-- {
			onAccept = onAcceptWrappers[i-1](onAccept)
		}
		srv.hooks.OnAccept = onAccept
	}
	if onBasicAuthWrappers != nil {
		onBasicAuth := func(ctx context.Context, client Client, req *ConnectRequest) error {
			return nil
		}
		for i := len(onBasicAuthWrappers); i > 0; i-- {
			onBasicAuth = onBasicAuthWrappers[i-1](onBasicAuth)
		}
		srv.hooks.OnBasicAuth = onBasicAuth
	}
	if onEnhancedAuthWrappers != nil {
		onEnhancedAuth := func(ctx context.Context, client Client, req *ConnectRequest) (resp *EnhancedAuthResponse, err error) {
			return &EnhancedAuthResponse{
				Continue: false,
			}, nil
		}
		for i := len(onEnhancedAuthWrappers); i > 0; i-- {
			onEnhancedAuth = onEnhancedAuthWrappers[i-1](onEnhancedAuth)
		}
		srv.hooks.OnEnhancedAuth = onEnhancedAuth
	}

	if onConnectedWrappers != nil {
		onConnected := func(ctx context.Context, client Client) {}
		for i := len(onConnectedWrappers); i > 0; i-- {
			onConnected = onConnectedWrappers[i-1](onConnected)
		}
		srv.hooks.OnConnected = onConnected
	}
	if onSessionCreatedWrapper != nil {
		onSessionCreated := func(ctx context.Context, client Client) {}
		for i := len(onSessionCreatedWrapper); i > 0; i-- {
			onSessionCreated = onSessionCreatedWrapper[i-1](onSessionCreated)
		}
		srv.hooks.OnSessionCreated = onSessionCreated
	}
	if onSessionResumedWrapper != nil {
		onSessionResumed := func(ctx context.Context, client Client) {}
		for i := len(onSessionResumedWrapper); i > 0; i-- {
			onSessionResumed = onSessionResumedWrapper[i-1](onSessionResumed)
		}
		srv.hooks.OnSessionResumed = onSessionResumed
	}
	if onSessionTerminatedWrapper != nil {
		onSessionTerminated := func(ctx context.Context, clientID string, reason SessionTerminatedReason) {}
		for i := len(onSessionTerminatedWrapper); i > 0; i-- {
			onSessionTerminated = onSessionTerminatedWrapper[i-1](onSessionTerminated)
		}
		srv.hooks.OnSessionTerminated = onSessionTerminated
	}
	if onSubscribeWrappers != nil {
		onSubscribe := func(ctx context.Context, client Client, req *SubscribeRequest) error {
			return nil
		}
		for i := len(onSubscribeWrappers); i > 0; i-- {
			onSubscribe = onSubscribeWrappers[i-1](onSubscribe)
		}
		srv.hooks.OnSubscribe = onSubscribe
	}
	if onSubscribedWrappers != nil {
		onSubscribed := func(ctx context.Context, client Client, subscription *gmqtt.Subscription) {}
		for i := len(onSubscribedWrappers); i > 0; i-- {
			onSubscribed = onSubscribedWrappers[i-1](onSubscribed)
		}
		srv.hooks.OnSubscribed = onSubscribed
	}
	if onUnsubscribeWrappers != nil {
		onUnsubscribe := func(ctx context.Context, client Client, req *UnsubscribeRequest) error {
			return nil
		}
		for i := len(onUnsubscribeWrappers); i > 0; i-- {
			onUnsubscribe = onUnsubscribeWrappers[i-1](onUnsubscribe)
		}
		srv.hooks.OnUnsubscribe = onUnsubscribe
	}
	if onUnsubscribedWrappers != nil {
		onUnsubscribed := func(ctx context.Context, client Client, topicName string) {}
		for i := len(onUnsubscribedWrappers); i > 0; i-- {
			onUnsubscribed = onUnsubscribedWrappers[i-1](onUnsubscribed)
		}
		srv.hooks.OnUnsubscribed = onUnsubscribed
	}
	if onMsgArrivedWrappers != nil {
		onMsgArrived := func(ctx context.Context, client Client, req *MsgArrivedRequest) error {
			return nil
		}
		for i := len(onMsgArrivedWrappers); i > 0; i-- {
			onMsgArrived = onMsgArrivedWrappers[i-1](onMsgArrived)
		}
		srv.hooks.OnMsgArrived = onMsgArrived
	}
	if OnDeliveredWrappers != nil {
		OnDelivered := func(ctx context.Context, client Client, msg *gmqtt.Message) {}
		for i := len(OnDeliveredWrappers); i > 0; i-- {
			OnDelivered = OnDeliveredWrappers[i-1](OnDelivered)
		}
		srv.hooks.OnDelivered = OnDelivered
	}
	if OnClosedWrappers != nil {
		OnClosed := func(ctx context.Context, client Client, err error) {}
		for i := len(OnClosedWrappers); i > 0; i-- {
			OnClosed = OnClosedWrappers[i-1](OnClosed)
		}
		srv.hooks.OnClosed = OnClosed
	}
	if onStopWrappers != nil {
		onStop := func(ctx context.Context) {}
		for i := len(onStopWrappers); i > 0; i-- {
			onStop = onStopWrappers[i-1](onStop)
		}
		srv.hooks.OnStop = onStop
	}
	if onMsgDroppedWrappers != nil {
		onMsgDropped := func(ctx context.Context, clientID string, msg *gmqtt.Message, err error) {}
		for i := len(onMsgDroppedWrappers); i > 0; i-- {
			onMsgDropped = onMsgDroppedWrappers[i-1](onMsgDropped)
		}
		srv.hooks.OnMsgDropped = onMsgDropped
	}
	if onWillPublishWrappers != nil {
		onWillPublish := func(ctx context.Context, clientID string, req *WillMsgRequest) {}
		for i := len(onWillPublishWrappers); i > 0; i-- {
			onWillPublish = onWillPublishWrappers[i-1](onWillPublish)
		}
		srv.hooks.OnWillPublish = onWillPublish
	}
	if onWillPublishedWrappers != nil {
		onWillPublished := func(ctx context.Context, clientID string, msg *gmqtt.Message) {}
		for i := len(onWillPublishedWrappers); i > 0; i-- {
			onWillPublished = onWillPublishedWrappers[i-1](onWillPublished)
		}
		srv.hooks.OnWillPublished = onWillPublished
	}
	return nil
}

func (srv *server) loadPlugins() error {
	for _, p := range srv.plugins {
		zaplog.Info("loading plugin", zap.String("name", p.Name()))
		err := p.Load(srv)
		if err != nil {
			return err
		}
	}
	return nil
}

func (srv *server) wsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := defaultUpgrader.Upgrade(w, r, nil)
		if err != nil {
			zaplog.Error("websocket upgrade error", zap.String("Msg", err.Error()))
			return
		}
		defer c.Close()
		conn := &wsConn{Conn: c.UnderlyingConn(), c: c}
		client, err := srv.newClient(conn)
		if err != nil {
			zaplog.Error("new client fail", zap.Error(err))
			return
		}
		client.serve()
	}
}

func (srv *server) setError(err error) {
	srv.errOnce.Do(func() {
		srv.err = err
		srv.exit()
	})
}

// Run starts the mqtt server.
func (srv *server) Run() (err error) {
	err = srv.Init()
	if err != nil {
		return err
	}
	var tcps []string
	var ws []string
	for _, v := range srv.tcpListener {
		tcps = append(tcps, v.Addr().String())
	}
	for _, v := range srv.websocketServer {
		ws = append(ws, v.Server.Addr)
	}
	zaplog.Info("gmqtt server started", zap.Strings("tcp server listen on", tcps), zap.Strings("websocket server listen on", ws))

	srv.status = serverStatusStarted
	srv.wg.Add(2)
	go srv.eventLoop()
	go srv.serveAPIServer()
	for _, ln := range srv.tcpListener {
		go srv.serveTCP(ln)
	}
	for _, server := range srv.websocketServer {
		mux := http.NewServeMux()
		mux.Handle(server.Path, srv.wsHandler())
		server.Server.Handler = mux
		go srv.serveWebSocket(server)
	}
	srv.wg.Wait()
	<-srv.exitedChan
	return srv.err
}

// Stop gracefully stops the mqtt server by the following steps:
//  1. Closing all opening TCP listeners and shutting down all opening websocket servers
//  2. Closing all idle connections
//  3. Waiting for all connections have been closed
//  4. Triggering OnStop()
func (srv *server) Stop(ctx context.Context) error {
	var err error
	srv.stopOnce.Do(func() {
		zaplog.Info("stopping gmqtt server")
		defer func() {
			defer close(srv.exitedChan)
			zaplog.Info("server stopped")
		}()
		srv.exit()

		for _, l := range srv.tcpListener {
			l.Close()
		}
		for _, ws := range srv.websocketServer {
			ws.Server.Shutdown(ctx)
		}
		// close all idle clients
		srv.mu.Lock()
		chs := make([]chan struct{}, len(srv.clients))
		i := 0
		for _, c := range srv.clients {
			chs[i] = c.closed
			i++
			c.Close()
		}
		srv.mu.Unlock()

		done := make(chan struct{})
		if len(chs) != 0 {
			go func() {
				for _, v := range chs {
					<-v
				}
				close(done)
			}()
		} else {
			close(done)
		}

		select {
		case <-ctx.Done():
			zaplog.Warn("server stop timeout, force exit", zap.String("error", ctx.Err().Error()))
			err = ctx.Err()
			return
		case <-done:
			for _, v := range srv.plugins {
				zaplog.Info("unloading plugin", zap.String("name", v.Name()))
				err := v.Unload()
				if err != nil {
					zaplog.Warn("plugin unload error", zap.String("error", err.Error()))
				}
			}
			if srv.hooks.OnStop != nil {
				srv.hooks.OnStop(context.Background())
			}
		}
	})
	return err
}
