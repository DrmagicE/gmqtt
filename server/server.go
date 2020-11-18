package server

import (
	"context"
	"errors"
	"math/rand"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/persistence/queue"
	"github.com/DrmagicE/gmqtt/pkg/codes"
	retained_trie "github.com/DrmagicE/gmqtt/retained/trie"
	subscription_trie "github.com/DrmagicE/gmqtt/subscription/trie"

	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/DrmagicE/gmqtt/retained"
	"github.com/DrmagicE/gmqtt/subscription"
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
	wg      sync.WaitGroup
	mu      sync.RWMutex //gard clients & offlineClients map
	status  int32        //server status
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
	queueStore map[string]queue.Store

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
	deliverMessageHandler func(srcClientID string, dstClientID string, msg *gmqtt.Message) (matched bool)
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

func (srv *server) sessionTerminated(client *client, reason SessionTerminatedReason) (err error) {
	switch reason {
	case NormalTermination:
		err = srv.removeSession(client)
	case ConflictTermination:
		err = srv.removeSession(client)
	case ExpiredTermination:
		err = srv.removeSession(client)
	}
	if srv.hooks.OnSessionTerminated != nil {
		srv.hooks.OnSessionTerminated(context.Background(), client, reason)
	}
	return err
}

// 已经判断是成功了，注册
func (srv *server) registerClient(connect *packets.Connect, ppt *packets.Properties, client *client) (err error) {
	var sessionResume bool
	srv.mu.Lock()
	defer func() {
		var connack *packets.Connack
		if err == nil {
			srv.clients[client.opts.ClientID] = client
			connack = connect.NewConnackPacket(codes.Success, sessionResume)
		} else {
			connack = connect.NewConnackPacket(codes.UnspecifiedError, sessionResume)
		}
		srv.mu.Unlock()
		connack.Properties = ppt
		client.out <- connack

	}()

	srv.statsManager.addClientConnected()
	srv.statsManager.addSessionActive()
	client.setConnected(time.Now())
	if srv.hooks.OnConnected != nil {
		srv.hooks.OnConnected(context.Background(), client)
	}
	oldClient, oldExist := srv.clients[client.opts.ClientID]
	if oldExist {
		if oldClient.shouldResumeSession(client) {
			sessionResume = true
		}
		if oldClient.IsConnected() {
			zaplog.Info("logging with duplicate ClientID",
				zap.String("remote", client.rwc.RemoteAddr().String()),
			)
			oldClient.setSwitching()
			oldClient.setError(codes.NewError(codes.SessionTakenOver))
			<-oldClient.Close()
		}
		if !sessionResume {
			err := srv.sessionTerminated(oldClient, ConflictTermination)
			if err != nil {
				zaplog.Error("session terminated fail", zap.Error(err))
			}
		}

		if oldClient.opts.Will.Flag {
			willMsg := &packets.Publish{
				Dup:        false,
				Qos:        oldClient.opts.Will.Qos,
				Retain:     oldClient.opts.Will.Retain,
				TopicName:  oldClient.opts.Will.Topic,
				Payload:    oldClient.opts.Will.Payload,
				Properties: oldClient.opts.Will.Properties,
			}
			// send will message because session ends
			if !sessionResume {
				srv.deliverMessageHandler(oldClient.opts.ClientID, "", gmqtt.MessageFromPublish(willMsg))
			} else if oldClient.version == packets.Version5 && convertUint32(oldClient.opts.Will.Properties.WillDelayInterval, 0) == 0 {
				// send will message because connection ends when will interval == 0
				srv.deliverMessageHandler(oldClient.opts.ClientID, "", gmqtt.MessageFromPublish(willMsg))
			} else if m, ok := srv.willMessage[client.opts.ClientID]; ok {
				close(m.cancel)
				delete(srv.willMessage, client.opts.ClientID)
			}
		}
	}

	if sessionResume {
		if srv.hooks.OnSessionResumed != nil {
			srv.hooks.OnSessionResumed(context.Background(), client)
		}
		client.queueStore = srv.queueStore[client.opts.ClientID]
		err := client.queueStore.Init(false)
		if err != nil {
			return err
		}
		zaplog.Info("logged in with session reuse",
			zap.String("remote_addr", client.rwc.RemoteAddr().String()),
			zap.String("client_id", client.opts.ClientID))
	} else {
		// create new session
		queueStore, err := srv.persistence.NewQueueStore(srv.config, client)
		if err != nil {
			return err
		}
		client.queueStore = queueStore
		srv.queueStore[client.opts.ClientID] = client.queueStore
		err = client.queueStore.Init(true)
		if err != nil {
			return err
		}

		if srv.hooks.OnSessionCreated != nil {
			srv.hooks.OnSessionCreated(context.Background(), client)
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
	if client.version == packets.Version311 {
		if !client.opts.CleanStart {
			storeSession = true
		}
	}
	if client.version == packets.Version5 {
		if client.opts.SessionExpiry != 0 {
			storeSession = true
		}
	}
	if !client.cleanWillFlag && client.opts.Will.Flag {
		will := &packets.Publish{
			Version:    client.version,
			Dup:        false,
			Qos:        client.opts.Will.Qos,
			Retain:     false,
			TopicName:  client.opts.Will.Topic,
			Payload:    client.opts.Will.Payload,
			Properties: client.opts.Will.Properties,
		}
		msg := gmqtt.MessageFromPublish(will)
		var willDelayInterval uint32
		if client.version == packets.Version5 {
			willDelayInterval = convertUint32(client.opts.Will.Properties.WillDelayInterval, 0)
			if client.opts.SessionExpiry <= willDelayInterval {
				willDelayInterval = client.opts.SessionExpiry
			}
		}
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
					srv.deliverMessageHandler(clientID, "", msg)
					srv.mu.Unlock()
				}
			}(client.opts.ClientID)
		} else {
			srv.deliverMessageHandler(client.opts.ClientID, "", msg)
		}
	}

	if storeSession {
		expiredTime := now.Add(time.Duration(client.opts.SessionExpiry) * time.Second)
		srv.offlineClients[client.opts.ClientID] = expiredTime
		zaplog.Info("logged out and storing session",
			zap.String("remote_addr", client.rwc.RemoteAddr().String()),
			zap.String("client_id", client.opts.ClientID),
			zap.Time("expired_at", expiredTime),
		)
		srv.statsManager.addSessionInactive()
	} else {
		zaplog.Info("logged out and cleaning session",
			zap.String("remote_addr", client.rwc.RemoteAddr().String()),
			zap.String("client_id", client.opts.ClientID),
		)
		_ = srv.sessionTerminated(client, NormalTermination)
		srv.statsManager.messageDequeue(client.statsManager.GetStats().MessageStats.QueuedCurrent)
	}

}

// deliverMessage send msg to matched client, must call under srv.Lock
func (srv *server) deliverMessage(srcClientID string, dstClientID string, msg *gmqtt.Message) (matched bool) {
	// subscriber (client id) list of shared subscriptions, key by share name.
	sharedList := make(map[string][]struct {
		clientID string
		sub      *gmqtt.Subscription
	})
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
		if c := srv.clients[clientID]; c != nil {
			if !c.checkMaxPacketSize(msg) {
				return true
			}
		}
		// shared
		if sub.ShareName != "" {
			sharedList[sub.ShareName] = append(sharedList[sub.ShareName], struct {
				clientID string
				sub      *gmqtt.Subscription
			}{clientID: clientID, sub: sub})
		} else {
			if srv.config.DeliveryMode == Overlap {
				if c, ok := srv.clients[clientID]; ok {
					if msg.QoS > sub.QoS {
						msg.QoS = sub.QoS
					}
					if c.version == packets.Version5 && sub.ID != 0 {
						msg.SubscriptionIdentifier = []uint32{sub.ID}
					}
					msg.Dup = false
					if !sub.RetainAsPublished {
						msg.Retained = false
					}
					var expiry time.Time
					if msg.MessageExpiry != 0 {
						expiry = now.Add(time.Duration(msg.MessageExpiry) * time.Second)
					}
					err := c.queueStore.Add(&queue.Elem{
						At:     now,
						Expiry: expiry,
						MessageWithID: &queue.Publish{
							Message: msg,
						},
					})
					if err != nil {
						c.setError(err)
					}
				}
			} else {
				// OnlyOnce
				if maxQos[clientID] == nil {
					maxQos[clientID] = &struct {
						sub    *gmqtt.Subscription
						subIDs []uint32
					}{sub: sub, subIDs: []uint32{sub.ID}}
				}
				if maxQos[clientID].sub.QoS < sub.QoS {
					maxQos[clientID].sub = sub
				}
				maxQos[clientID].subIDs = append(maxQos[clientID].subIDs, sub.ID)
			}
		}
		return true
	}, subscription.IterationOptions{
		Type:      subscription.TypeAll,
		ClientID:  dstClientID,
		MatchType: subscription.MatchFilter,
		TopicName: msg.Topic,
	})
	if srv.config.DeliveryMode == OnlyOnce {
		for clientID, v := range maxQos {
			if c := srv.clients[clientID]; c != nil {
				if msg.QoS > v.sub.QoS {
					msg.QoS = v.sub.QoS
				}
				if c.version == packets.Version5 {
					for _, id := range v.subIDs {
						if id != 0 {
							msg.SubscriptionIdentifier = append(msg.SubscriptionIdentifier, id)
						}
					}
				}
				msg.Dup = false
				if !v.sub.RetainAsPublished {
					msg.Retained = false
				}
				var expiry time.Time
				if msg.MessageExpiry != 0 {
					expiry = now.Add(time.Duration(msg.MessageExpiry) * time.Second)
				}
				err := c.queueStore.Add(&queue.Elem{
					At:     now,
					Expiry: expiry,
					MessageWithID: &queue.Publish{
						Message: msg,
					},
				})
				if err != nil {
					c.setError(err)
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
		if dstClientID != "" {
			for _, vv := range v {
				if vv.clientID == dstClientID {
					rs = vv
					break
				}
			}
		} else {
			// random
			rs = v[rand.Intn(len(v))]
		}
		if c, ok := srv.clients[rs.clientID]; ok {
			if msg.QoS > rs.sub.QoS {
				msg.QoS = rs.sub.QoS
			}
			msg.SubscriptionIdentifier = []uint32{v[0].sub.ID}
			msg.Dup = false
			if !rs.sub.RetainAsPublished {
				msg.Retained = false
			}
			var expiry time.Time
			if msg.MessageExpiry != 0 {
				expiry = now.Add(time.Duration(msg.MessageExpiry) * time.Second)
			}
			err := c.queueStore.Add(&queue.Elem{
				At:     now,
				Expiry: expiry,
				MessageWithID: &queue.Publish{
					Message: msg,
				},
			})
			if err != nil {
				c.setError(err)
			}
		}
	}
	return
}

func (srv *server) removeSession(client *client) error {
	delete(srv.clients, client.opts.ClientID)
	delete(srv.offlineClients, client.opts.ClientID)
	delete(srv.queueStore, client.opts.ClientID)
	srv.subscriptionsDB.UnsubscribeAll(client.opts.ClientID)
	err := client.queueStore.Clean()
	return err
}

// sessionExpireCheck 判断是否超时
// sessionExpireCheck check and terminate expired sessions
func (srv *server) sessionExpireCheck() {
	now := time.Now()
	srv.mu.Lock()
	for id, expiredTime := range srv.offlineClients {
		if now.After(expiredTime) {
			if client, _ := srv.clients[id]; client != nil {
				_ = srv.sessionTerminated(client, ExpiredTermination)
				srv.statsManager.addSessionExpired()
				srv.statsManager.decSessionInactive()
			}
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
	client.newSession()

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
		onAckedWrappers            []OnAckedWrapper
		onCloseWrappers            []OnCloseWrapper
		onStopWrappers             []OnStopWrapper
		onMsgDroppedWrappers       []OnMsgDroppedWrapper
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
		onSessionTerminated := func(ctx context.Context, client Client, reason SessionTerminatedReason) {}
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
		onMsgDropped := func(ctx context.Context, client Client, msg *gmqtt.Message) {}
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
	persistence, err := persistenceFactories["memory"].New(srv.config, srv.hooks)
	if err != nil {
		return err
	}
	err = persistence.Open()
	if err != nil {
		return err
	}
	srv.persistence = persistence
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
