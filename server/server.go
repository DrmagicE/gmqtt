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
	"github.com/DrmagicE/gmqtt/persistence/queue"
	"github.com/DrmagicE/gmqtt/persistence/session"
	subscription_trie "github.com/DrmagicE/gmqtt/persistence/subscription/mem"
	"github.com/DrmagicE/gmqtt/persistence/unack"
	"github.com/DrmagicE/gmqtt/pkg/codes"
	retained_trie "github.com/DrmagicE/gmqtt/retained/trie"

	"github.com/DrmagicE/gmqtt/persistence/subscription"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/DrmagicE/gmqtt/retained"
)

var (
	// ErrInvalWsMsgType [MQTT-6.0.0-1]
	ErrInvalWsMsgType = errors.New("invalid websocket message type")
	statusPanic       = "invalid server status"

	plugins []Plugable

	// DefaultTopicAliasMgrFactory is optional, see the topicalias package
	DefaultTopicAliasMgrFactory topicAliasMgrFactory

	persistenceFactories = make(map[string]PersistenceFactory)
)

func RegisterPersistenceFactory(name string, factory PersistenceFactory) {
	persistenceFactories[name] = factory
}

type topicAliasMgrFactory interface {
	New() TopicAliasManager
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
	// SubscriptionStore returns the subscription.Store.
	SubscriptionStore() subscription.Store
	// RetainedStore returns the retained.Store.
	RetainedStore() retained.Store
	// PublishService returns the PublishService
	PublishService() PublishService
	// Client return the client specified by clientID.
	Client(clientID string) Client
	// GetConfig returns the config of the server
	GetConfig() Config
	// GetStatsManager returns StatsManager
	GetStatsManager() StatsManager
	// Stop stop the server gracefully
	Stop(ctx context.Context) error
	// ApplyConfig will replace the config of the server
	ApplyConfig(config Config)
}

// PublishService provides the ability to Publish messages to the broker.
type PublishService interface {
	// Publish Publish a message to broker.
	// Calling this method will not trigger OnMsgArrived hook.
	Publish(message *gmqtt.Message)
	// PublishToClient Publish a message to a specific client.
	// The message will send to the client only if the client is subscribed to a topic that matches the message.
	// Calling this method will not trigger OnMsgArrived hook.
	PublishToClient(clientID string, message *gmqtt.Message)
}

// server represents a mqtt server instance.
// Create a server by using NewServer()
type server struct {
	wg     sync.WaitGroup
	mu     sync.RWMutex //gard clients & offlineClients map
	status int32        //server status
	// clients stores the  online clients
	clients map[string]*client
	// offlineClients store the expired time of all disconnected clients
	// with valid session(not expired). Key by clientID
	offlineClients  map[string]time.Time
	willMessage     map[string]*willMsg
	tcpListener     []net.Listener //tcp listeners
	websocketServer []*WsServer    //websocket serverStop
	exitChan        chan struct{}

	retainedDB      retained.Store
	subscriptionsDB subscription.Store //store subscriptions

	persistence Persistence
	//
	queueStore   map[string]queue.Store
	unackStore   map[string]unack.Store
	sessionStore session.Store

	// gard config
	configMu sync.RWMutex
	config   Config
	hooks    Hooks
	plugins  []Plugable

	statsManager   StatsManager
	publishService PublishService

	// manage topic alias for V5 clients
	topicAliasManager TopicAliasManager
	// for testing
	deliverMessageHandler func(srcClientID string, msg *gmqtt.Message) (matched bool)
}

func (srv *server) ApplyConfig(config Config) {
	srv.configMu.Lock()
	defer srv.configMu.Unlock()
	srv.config = config

}

func (srv *server) SubscriptionStore() subscription.Store {
	return srv.subscriptionsDB
}

func (srv *server) RetainedStore() retained.Store {
	return srv.retainedDB
}

func (srv *server) PublishService() PublishService {
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
func (srv *server) GetConfig() Config {
	return srv.config
}

// GetStatsManager returns StatsManager
func (srv *server) GetStatsManager() StatsManager {
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
	return err
}

func uint32P(v uint32) *uint32 {
	return &v
}
func uint16P(v uint16) *uint16 {
	return &v
}
func byteP(v byte) *byte {
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

// 已经判断是成功了，注册
func (srv *server) registerClient(connect *packets.Connect, connackPpt *packets.Properties, client *client) (err error) {
	var sessionResume bool
	var qs queue.Store
	var ua unack.Store
	var sess *gmqtt.Session
	now := time.Now()
	srv.mu.Lock()
	defer func() {
		var connack *packets.Connack
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
			if client.version == packets.Version311 && !connect.CleanStart {
				expiryInterval = uint32(srv.config.SessionExpiry.Seconds())
			} else if connect.Properties != nil {
				willDelayInterval = convertUint32(connect.WillProperties.WillDelayInterval, 0)
				expiryInterval = convertUint32(connect.Properties.SessionExpiryInterval, 0)
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
			if sessionResume && srv.hooks.OnSessionResumed != nil {
				srv.hooks.OnSessionResumed(context.Background(), client)
			} else if srv.hooks.OnSessionCreated != nil {
				srv.hooks.OnSessionCreated(context.Background(), client)
			}
			srv.clients[client.opts.ClientID] = client
			srv.unackStore[client.opts.ClientID] = ua
			srv.queueStore[client.opts.ClientID] = qs
			client.queueStore = qs
			client.unackStore = ua
			connack = connect.NewConnackPacket(codes.Success, sessionResume)
		} else {
			connack = connect.NewConnackPacket(codes.UnspecifiedError, sessionResume)
		}
		srv.mu.Unlock()
		connack.Properties = connackPpt
		client.out <- connack

	}()

	srv.statsManager.addClientConnected()
	srv.statsManager.addSessionActive()
	client.setConnected(time.Now())
	if srv.hooks.OnConnected != nil {
		srv.hooks.OnConnected(context.Background(), client)
	}
	var oldSession *gmqtt.Session
	oldSession, err = srv.sessionStore.Get(client.opts.ClientID)
	if err != nil {
		zaplog.Error("fail to get session",
			zap.String("remote_addr", client.rwc.RemoteAddr().String()),
			zap.String("client_id", client.opts.ClientID))
		return
	}
	if oldSession != nil {
		if !oldSession.IsExpired(now) && !connect.CleanStart {
			sessionResume = true
		}
		// session still active, close it first
		if c := srv.clients[oldSession.ClientID]; c != nil {
			zaplog.Info("logging with duplicate ClientID",
				zap.String("remote", client.rwc.RemoteAddr().String()),
			)
			c.setSwitching()
			c.setError(codes.NewError(codes.SessionTakenOver))
			<-c.Close()
		}

		if will := oldSession.Will; will != nil {
			// send will message because session ends
			if !sessionResume {
				srv.deliverMessageHandler(oldSession.ClientID, will.Copy())
			} else if oldSession.WillDelayInterval == 0 {
				// send will message because connection ends with will interval == 0
				srv.deliverMessageHandler(oldSession.ClientID, will.Copy())
			} else if m, ok := srv.willMessage[client.opts.ClientID]; ok {
				close(m.cancel)
				delete(srv.willMessage, client.opts.ClientID)
			}
		}
		// clean old session
		if !sessionResume {
			err = srv.sessionTerminatedLocked(oldSession.ClientID, ConflictTermination)
			if err != nil {
				err = fmt.Errorf("session terminated fail: %w", err)
				zaplog.Error("session terminated fail", zap.Error(err))
			}
		} else {
			qs = srv.queueStore[client.opts.ClientID]
			if qs != nil {
				err := qs.Init(false)
				if err != nil {
					return err
				}
			}
			ua = srv.unackStore[client.opts.ClientID]
			if ua != nil {
				err := ua.Init(false)
				if err != nil {
					return err
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
		qs, err = srv.persistence.NewQueueStore(srv.config, client.opts.ClientID)
		if err != nil {
			return err
		}
		err = qs.Init(true)
		if err != nil {
			return err
		}

		ua, err = srv.persistence.NewUnackStore(srv.config, client.opts.ClientID)
		if err != nil {
			return err
		}
		err = ua.Init(true)
		if err != nil {
			return err
		}
		zaplog.Info("logged in with new session",
			zap.String("remote_addr", client.rwc.RemoteAddr().String()),
			zap.String("client_id", client.opts.ClientID),
		)
	}
	delete(srv.offlineClients, client.opts.ClientID)
	if srv.topicAliasManager != nil {
		srv.topicAliasManager.Create(client)
	}
	return
}

type willMsg struct {
	msg    *gmqtt.Message
	cancel chan struct{}
}

func (srv *server) unregisterClient(client *client) {
	if !client.IsConnected() {
		return
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	now := time.Now()
	client.setDisConnected()

	var storeSession bool
	if sess, err := srv.sessionStore.Get(client.opts.ClientID); sess != nil {
		if client.version == packets.Version5 && client.disconnect != nil {
			sess.ExpiryInterval = convertUint32(client.disconnect.Properties.SessionExpiryInterval, sess.ExpiryInterval)
		}
		if sess.ExpiryInterval != 0 {
			storeSession = true
		}
		if !client.cleanWillFlag && sess.Will != nil {
			willDelayInterval := sess.WillDelayInterval
			if sess.ExpiryInterval <= sess.WillDelayInterval {
				willDelayInterval = sess.ExpiryInterval
			}
			msg := sess.Will.Copy()
			if willDelayInterval != 0 && storeSession {
				wm := &willMsg{
					msg:    msg,
					cancel: make(chan struct{}),
				}
				srv.willMessage[client.opts.ClientID] = wm
				t := time.NewTimer(time.Duration(willDelayInterval) * time.Second)
				go func(clientID string) {
					select {
					case <-wm.cancel:
						t.Stop()
					case <-t.C:
						srv.mu.Lock()
						srv.deliverMessageHandler(clientID, msg)
						srv.mu.Unlock()
					}
				}(client.opts.ClientID)
			} else {
				srv.deliverMessageHandler(client.opts.ClientID, msg)
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
			srv.statsManager.addSessionInactive()
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
	srv.statsManager.messageDequeue(client.statsManager.GetStats().MessageStats.QueuedCurrent)
}

// deliverMessage send msg to matched client, must call under srv.Lock
func (srv *server) deliverMessage(srcClientID string, msg *gmqtt.Message) (matched bool) {
	// subscriber (client id) list of shared subscriptions, key by share name.
	sharedList := make(map[string][]struct {
		clientID string
		sub      *gmqtt.Subscription
	})
	// key by clientid
	maxQos := make(map[string]*struct {
		sub    *gmqtt.Subscription
		subIDs []uint32
	})
	now := time.Now()
	// 先取的所有的share 和unshare
	srv.subscriptionsDB.Iterate(func(clientID string, sub *gmqtt.Subscription) bool {
		if sub.NoLocal && clientID == srcClientID {
			return true
		}
		matched = true
		if qs := srv.queueStore[clientID]; qs != nil {
			// shared
			if sub.ShareName != "" {
				sharedList[sub.ShareName] = append(sharedList[sub.ShareName], struct {
					clientID string
					sub      *gmqtt.Subscription
				}{clientID: clientID, sub: sub})
			} else {
				if srv.config.DeliveryMode == Overlap {
					newMsg := msg.Copy()
					if newMsg.QoS > sub.QoS {
						newMsg.QoS = sub.QoS
					}
					if sub.ID != 0 {
						newMsg.SubscriptionIdentifier = []uint32{sub.ID}
					}
					newMsg.Dup = false
					if !sub.RetainAsPublished {
						newMsg.Retained = false
					}
					var expiry time.Time
					if msg.MessageExpiry != 0 {
						expiry = now.Add(time.Duration(newMsg.MessageExpiry) * time.Second)
					}
					err := qs.Add(&queue.Elem{
						At:     now,
						Expiry: expiry,
						MessageWithID: &queue.Publish{
							Message: msg.Copy(),
						},
					})
					if err != nil {
						zaplog.Error("fail to add message to queue",
							zap.String("topic", subscription.GetFullTopicName(sub.ShareName, sub.TopicFilter)),
							zap.Uint8("qos", sub.QoS))
					}

				} else {
					// OnlyOnce
					if maxQos[clientID] == nil {
						maxQos[clientID] = &struct {
							sub    *gmqtt.Subscription
							subIDs []uint32
						}{sub: sub, subIDs: []uint32{sub.ID}}
					} else {
						if maxQos[clientID].sub.QoS < sub.QoS {
							maxQos[clientID].sub = sub
						}
						maxQos[clientID].subIDs = append(maxQos[clientID].subIDs, sub.ID)
					}

				}
			}
		}
		return true
	}, subscription.IterationOptions{
		Type:      subscription.TypeAll,
		MatchType: subscription.MatchFilter,
		TopicName: msg.Topic,
	})
	if srv.config.DeliveryMode == OnlyOnce {
		for clientID, v := range maxQos {
			if qs := srv.queueStore[clientID]; qs != nil {
				newMsg := msg.Copy()
				if newMsg.QoS > v.sub.QoS {
					newMsg.QoS = v.sub.QoS
				}

				for _, id := range v.subIDs {
					if id != 0 {
						newMsg.SubscriptionIdentifier = append(newMsg.SubscriptionIdentifier, id)
					}
				}

				newMsg.Dup = false
				if !v.sub.RetainAsPublished {
					newMsg.Retained = false
				}
				var expiry time.Time
				if newMsg.MessageExpiry != 0 {
					expiry = now.Add(time.Duration(newMsg.MessageExpiry) * time.Second)
				}
				err := qs.Add(&queue.Elem{
					At:     now,
					Expiry: expiry,
					MessageWithID: &queue.Publish{
						Message: newMsg,
					},
				})
				if err != nil {
					zaplog.Error("fail to add message to queue",
						zap.String("topic", subscription.GetFullTopicName(v.sub.ShareName, v.sub.TopicFilter)),
						zap.Uint8("qos", v.sub.QoS))
				}
			}
		}
	}
	// shared subscription
	// TODO 实现一个钩子函数，自定义随机逻辑。 在shared里面挑一个
	for _, v := range sharedList {
		var rs struct {
			clientID string
			sub      *gmqtt.Subscription
		}
		// random
		rs = v[rand.Intn(len(v))]
		if c, ok := srv.clients[rs.clientID]; ok {
			newMsg := msg.Copy()
			if newMsg.QoS > rs.sub.QoS {
				newMsg.QoS = rs.sub.QoS
			}
			newMsg.SubscriptionIdentifier = []uint32{v[0].sub.ID}
			newMsg.Dup = false
			if !rs.sub.RetainAsPublished {
				newMsg.Retained = false
			}
			var expiry time.Time
			if msg.MessageExpiry != 0 {
				expiry = now.Add(time.Duration(newMsg.MessageExpiry) * time.Second)
			}
			err := c.queueStore.Add(&queue.Elem{
				At:     now,
				Expiry: expiry,
				MessageWithID: &queue.Publish{
					Message: newMsg,
				},
			})
			if err != nil {
				c.setError(err)
			}
		}
	}
	return
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
			srv.statsManager.addSessionExpired()
			srv.statsManager.decSessionInactive()

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
	subStore := subscription_trie.NewStore()
	statsMgr := newStatsManager(subStore)
	srv := &server{
		status:          serverStatusInit,
		exitChan:        make(chan struct{}),
		clients:         make(map[string]*client),
		offlineClients:  make(map[string]time.Time),
		willMessage:     make(map[string]*willMsg),
		retainedDB:      retained_trie.NewStore(),
		subscriptionsDB: subStore,
		config:          DefaultConfig,
		statsManager:    statsMgr,
		queueStore:      make(map[string]queue.Store),
		unackStore:      make(map[string]unack.Store),
	}
	if DefaultTopicAliasMgrFactory != nil {
		srv.topicAliasManager = DefaultTopicAliasMgrFactory.New()
	}

	srv.deliverMessageHandler = srv.deliverMessage
	srv.publishService = &publishService{server: srv}
	return srv
}

// New returns a gmqtt server instance with the given options
func New(opts ...Options) *server {
	// statistics
	srv := defaultServer()
	for _, fn := range opts {
		fn(srv)
	}
	return srv
}

// Init initialises the options.
func (srv *server) Init(opts ...Options) {
	for _, fn := range opts {
		fn(srv)
	}
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

//实现io.ReadWriter接口
// wsConn implements the io.readWriter
type wsConn struct {
	net.Conn
	c *websocket.Conn
}

func (ws *wsConn) Close() error {
	return ws.Conn.Close()
}

func (ws *wsConn) Read(p []byte) (n int, err error) {
	msgType, r, err := ws.c.NextReader()
	if err != nil {
		return 0, err
	}
	if msgType != websocket.BinaryMessage {
		return 0, ErrInvalWsMsgType
	}
	return r.Read(p)
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
	if err != http.ErrServerClosed {
		panic(err.Error())
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
		closeComplete: make(chan struct{}),
		error:         make(chan error, 1),
		in:            make(chan packets.Packet, readBufferSize),
		out:           make(chan packets.Packet, writeBufferSize),
		status:        Connecting,
		opts:          &ClientOptions{},
		cleanWillFlag: false,
		statsManager:  newSessionStatsManager(),
		config:        cfg,
	}
	client.packetReader = packets.NewReader(client.bufr)
	client.packetWriter = packets.NewWriter(client.bufw)
	client.setConnecting()

	return client, nil
}

func (srv *server) loadPlugins() error {

	var (
		onAcceptWrappers           []OnAcceptWrapper
		onBasicAuthWrappers        []OnBasicAuthWrapper
		onEnhancedAuthWrappers     []OnEnhancedAuthWrapper
		onConnectedWrappers        []OnConnectedWrapper
		onSessionCreatedWrapper    []OnSessionCreatedWrapper
		onSessionResumedWrapper    []OnSessionResumedWrapper
		onSessionTerminatedWrapper []OnSessionTerminatedWrapper
		onSubscribeWrappers        []OnSubscribeWrapper
		onSubscribedWrappers       []OnSubscribedWrapper
		onUnsubscribedWrappers     []OnUnsubscribedWrapper
		onMsgArrivedWrappers       []OnMsgArrivedWrapper
		onDeliverWrappers          []OnDeliverWrapper
		// TODO remove onAck wrappers
		onAckedWrappers      []OnAckedWrapper
		onCloseWrappers      []OnCloseWrapper
		onStopWrappers       []OnStopWrapper
		onMsgDroppedWrappers []OnMsgDroppedWrapper
	)
	for _, p := range srv.plugins {
		zaplog.Info("loading plugin", zap.String("name", p.Name()))
		err := p.Load(srv)
		if err != nil {
			return err
		}
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
		if hooks.OnUnsubscribedWrapper != nil {
			onUnsubscribedWrappers = append(onUnsubscribedWrappers, hooks.OnUnsubscribedWrapper)
		}
		if hooks.OnMsgArrivedWrapper != nil {
			onMsgArrivedWrappers = append(onMsgArrivedWrappers, hooks.OnMsgArrivedWrapper)
		}
		if hooks.OnMsgDroppedWrapper != nil {
			onMsgDroppedWrappers = append(onMsgDroppedWrappers, hooks.OnMsgDroppedWrapper)
		}
		if hooks.OnDeliverWrapper != nil {
			onDeliverWrappers = append(onDeliverWrappers, hooks.OnDeliverWrapper)
		}
		if hooks.OnAckedWrapper != nil {
			onAckedWrappers = append(onAckedWrappers, hooks.OnAckedWrapper)
		}
		if hooks.OnCloseWrapper != nil {
			onCloseWrappers = append(onCloseWrappers, hooks.OnCloseWrapper)
		}
		if hooks.OnStopWrapper != nil {
			onStopWrappers = append(onStopWrappers, hooks.OnStopWrapper)
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
		onBasicAuth := func(ctx context.Context, client Client, req *ConnectRequest) (resp *ConnectResponse) {
			return &ConnectResponse{
				Code: codes.Success,
			}
		}
		for i := len(onBasicAuthWrappers); i > 0; i-- {
			onBasicAuth = onBasicAuthWrappers[i-1](onBasicAuth)
		}
		srv.hooks.OnBasicAuth = onBasicAuth
	}
	if onEnhancedAuthWrappers != nil {
		onEnhancedAuth := func(ctx context.Context, client Client, req *ConnectRequest) (resp *EnhancedAuthResponse) {
			return &EnhancedAuthResponse{
				Code: codes.Success,
			}
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
		onSubscribe := func(ctx context.Context, client Client, subscribe *packets.Subscribe) (*SubscribeResponse, *codes.ErrorDetails) {
			return &SubscribeResponse{
				Topics: subscribe.Topics,
			}, nil
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
	if onUnsubscribedWrappers != nil {
		onUnsubscribed := func(ctx context.Context, client Client, topicName string) {}
		for i := len(onUnsubscribedWrappers); i > 0; i-- {
			onUnsubscribed = onUnsubscribedWrappers[i-1](onUnsubscribed)
		}
		srv.hooks.OnUnsubscribed = onUnsubscribed
	}
	if onMsgArrivedWrappers != nil {
		onMsgArrived := func(ctx context.Context, client Client, pub *packets.Publish) (*gmqtt.Message, error) {
			return gmqtt.MessageFromPublish(pub), nil
		}
		for i := len(onMsgArrivedWrappers); i > 0; i-- {
			onMsgArrived = onMsgArrivedWrappers[i-1](onMsgArrived)
		}
		srv.hooks.OnMsgArrived = onMsgArrived
	}
	if onDeliverWrappers != nil {
		onDeliver := func(ctx context.Context, client Client, msg *gmqtt.Message) {}
		for i := len(onDeliverWrappers); i > 0; i-- {
			onDeliver = onDeliverWrappers[i-1](onDeliver)
		}
		srv.hooks.OnDeliver = onDeliver
	}
	if onAckedWrappers != nil {
		onAcked := func(ctx context.Context, client Client, msg *gmqtt.Message) {}
		for i := len(onAckedWrappers); i > 0; i-- {
			onAcked = onAckedWrappers[i-1](onAcked)
		}
		srv.hooks.OnAcked = onAcked
	}
	if onCloseWrappers != nil {
		onClose := func(ctx context.Context, client Client, err error) {}
		for i := len(onCloseWrappers); i > 0; i-- {
			onClose = onCloseWrappers[i-1](onClose)
		}
		srv.hooks.OnClose = onClose
	}
	if onStopWrappers != nil {
		onStop := func(ctx context.Context) {}
		for i := len(onStopWrappers); i > 0; i-- {
			onStop = onStopWrappers[i-1](onStop)
		}
		srv.hooks.OnStop = onStop
	}
	if onMsgDroppedWrappers != nil {
		onMsgDropped := func(ctx context.Context, clientID string, msg *gmqtt.Message) {}
		for i := len(onMsgDroppedWrappers); i > 0; i-- {
			onMsgDropped = onMsgDroppedWrappers[i-1](onMsgDropped)
		}
		srv.hooks.OnMsgDropped = onMsgDropped
	}
	return nil
}

func (srv *server) wsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := defaultUpgrader.Upgrade(w, r, nil)
		if err != nil {
			zaplog.Warn("websocket upgrade error", zap.String("Msg", err.Error()))
			return
		}
		defer c.Close()
		conn := &wsConn{c.UnderlyingConn(), c}
		client, err := srv.newClient(conn)
		if err != nil {
			zaplog.Error("new client fail", zap.Error(err))
			return
		}
		client.serve()
	}
}

// Run starts the mqtt server. This method is non-blocking
func (srv *server) Run() error {

	// TODO read from configration
	persistence, err := persistenceFactories["redis"].New(srv.config, srv.hooks)
	if err != nil {
		return err
	}
	err = persistence.Open()
	if err != nil {
		return err
	}
	zaplog.Info("open redis persistence")
	srv.persistence = persistence

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
	// init queue store & unack store from persistence
	for _, v := range sts {
		q, err := srv.persistence.NewQueueStore(srv.config, v.ClientID)
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
	zaplog.Info("init subscription store")
	err = srv.subscriptionsDB.Init(cids)
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
	zaplog.Info("starting gmqtt server", zap.Strings("tcp server listen on", tcps), zap.Strings("websocket server listen on", ws))

	err = srv.loadPlugins()
	if err != nil {
		return err
	}
	srv.status = serverStatusStarted
	srv.wg.Add(1)
	go srv.eventLoop()
	for _, ln := range srv.tcpListener {
		go srv.serveTCP(ln)
	}
	for _, server := range srv.websocketServer {
		mux := http.NewServeMux()
		mux.Handle(server.Path, srv.wsHandler())
		server.Server.Handler = mux
		go srv.serveWebSocket(server)
	}
	return nil
}

// Stop gracefully stops the mqtt server by the following steps:
//  1. Closing all opening TCP listeners and shutting down all opening websocket servers
//  2. Closing all idle connections
//  3. Waiting for all connections have been closed
//  4. Triggering OnStop()
func (srv *server) Stop(ctx context.Context) error {
	zaplog.Info("stopping gmqtt server")
	defer func() {
		zaplog.Info("server stopped")
		//zaplog.Sync()
	}()
	select {
	case <-srv.exitChan:
		return nil
	default:
		close(srv.exitChan)
	}
	srv.wg.Wait()
	for _, l := range srv.tcpListener {
		l.Close()
	}
	for _, ws := range srv.websocketServer {
		ws.Server.Shutdown(ctx)
	}

	//关闭所有的client
	//closing all idle clients
	srv.mu.Lock()
	closeCompleteSet := make([]<-chan struct{}, len(srv.clients))
	i := 0
	for _, c := range srv.clients {
		closeCompleteSet[i] = c.Close()
		i++
	}
	srv.mu.Unlock()
	done := make(chan struct{})
	go func() {
		for _, v := range closeCompleteSet {
			//等所有的session退出完毕
			//waiting for all sessions to unregister
			<-v
		}
		close(done)
	}()
	select {
	case <-ctx.Done():
		zaplog.Warn("server stop timeout, forced exit", zap.String("error", ctx.Err().Error()))
		return ctx.Err()
	case <-done:
		for _, v := range srv.plugins {
			err := v.Unload()
			if err != nil {
				zaplog.Warn("plugin unload error", zap.String("error", err.Error()))
			}
		}
		if srv.hooks.OnStop != nil {
			srv.hooks.OnStop(context.Background())
		}
		return nil
	}

}
