package gmqtt

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"

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
)

// Default configration
const (
	DefaultMsgRouterLen  = 4096
	DefaultRegisterLen   = 2048
	DefaultUnRegisterLen = 2048
)

// Server status
const (
	serverStatusInit = iota
	serverStatusStarted
)

var zaplog *zap.Logger

func init() {
	zaplog = zap.NewNop()
}

// LoggerWithField add fields to a new logger.
// Plugins can use this method to add plugin name field.
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
}

// server represents a mqtt server instance.
// Create a server by using NewServer()
type server struct {
	wg      sync.WaitGroup
	mu      sync.RWMutex //gard clients & offlineClients map
	status  int32        //server status
	clients map[string]*client
	// offlineClients store the disconnected time of all disconnected clients
	// with valid session(not expired). Key by clientID
	offlineClients  map[string]time.Time
	tcpListener     []net.Listener //tcp listeners
	websocketServer []*WsServer    //websocket serverStop
	exitChan        chan struct{}

	retainedDB      retained.Store
	subscriptionsDB subscription.Store //store subscriptions

	msgRouter  chan *msgRouter
	register   chan *register   //register session
	unregister chan *unregister //unregister session
	config     Config
	hooks      Hooks
	plugins    []Plugable

	statsManager   StatsManager
	publishService PublishService
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

type DeliveryMode int

const (
	Overlap  DeliveryMode = 0
	OnlyOnce DeliveryMode = 1
)

type Config struct {
	RetryInterval              time.Duration
	RetryCheckInterval         time.Duration
	SessionExpiryInterval      time.Duration
	SessionExpiryCheckInterval time.Duration
	QueueQos0Messages          bool
	MaxInflight                int
	MaxAwaitRel                int
	MaxMsgQueue                int
	DeliveryMode               DeliveryMode
	MsgRouterLen               int
	RegisterLen                int
	UnregisterLen              int
}

// DefaultConfig default config used by NewServer()
var DefaultConfig = Config{
	RetryInterval:              20 * time.Second,
	RetryCheckInterval:         20 * time.Second,
	SessionExpiryInterval:      0 * time.Second,
	SessionExpiryCheckInterval: 0 * time.Second,
	QueueQos0Messages:          true,
	MaxInflight:                32,
	MaxAwaitRel:                100,
	MaxMsgQueue:                1000,
	DeliveryMode:               OnlyOnce,
	MsgRouterLen:               DefaultMsgRouterLen,
	RegisterLen:                DefaultRegisterLen,
	UnregisterLen:              DefaultUnRegisterLen,
}

// GetConfig returns the config of the server
func (srv *server) GetConfig() Config {
	return srv.config
}

// GetStatsManager returns StatsManager
func (srv *server) GetStatsManager() StatsManager {
	return srv.statsManager
}

//session register
type register struct {
	client  *client
	connect *packets.Connect
	error   error
}

// session unregister
type unregister struct {
	client *client
	done   chan struct{}
}

type msgRouter struct {
	msg      packets.Message
	clientID string
	// if set to false, must set clientID to specify the client to send
	match bool
}

// Status returns the server status
func (srv *server) Status() int32 {
	return atomic.LoadInt32(&srv.status)
}

func (srv *server) registerHandler(register *register) {
	// ack code set in Connack Packet
	var code uint8
	client := register.client
	defer close(client.ready)
	connect := register.connect
	var sessionReuse bool
	if connect.AckCode != packets.CodeAccepted {
		err := errors.New("reject connection, ack code:" + strconv.Itoa(int(connect.AckCode)))
		ack := connect.NewConnackPacket(false)
		client.writePacket(ack)
		register.error = err
		return
	}
	if srv.hooks.OnConnect != nil {
		code = srv.hooks.OnConnect(context.Background(), client)
	}
	connect.AckCode = code
	if code != packets.CodeAccepted {
		err := errors.New("reject connection, ack code:" + strconv.Itoa(int(code)))
		ack := connect.NewConnackPacket(false)
		client.writePacket(ack)
		client.setError(err)
		register.error = err
		return
	}
	if srv.hooks.OnConnected != nil {
		srv.hooks.OnConnected(context.Background(), client)
	}
	srv.statsManager.addClientConnected()
	srv.statsManager.addSessionActive()

	client.setConnectedAt(time.Now())
	srv.mu.Lock()
	defer srv.mu.Unlock()
	var oldSession *session
	oldClient, oldExist := srv.clients[client.opts.clientID]
	srv.clients[client.opts.clientID] = client
	if oldExist {
		oldSession = oldClient.session
		if oldClient.IsConnected() {
			zaplog.Info("logging with duplicate ClientID",
				zap.String("remote", client.rwc.RemoteAddr().String()),
				zap.String("client_id", client.OptionsReader().ClientID()),
			)
			oldClient.setSwitching()
			<-oldClient.Close()
			if oldClient.opts.willFlag {
				willMsg := &packets.Publish{
					Dup:       false,
					Qos:       oldClient.opts.willQos,
					Retain:    oldClient.opts.willRetain,
					TopicName: []byte(oldClient.opts.willTopic),
					Payload:   oldClient.opts.willPayload,
				}
				go func() {
					msgRouter := &msgRouter{msg: messageFromPublish(willMsg), match: true}
					srv.msgRouter <- msgRouter
				}()
			}
			if !client.opts.cleanSession && !oldClient.opts.cleanSession { //reuse old session
				sessionReuse = true
			}
		} else if oldClient.IsDisConnected() {
			if !client.opts.cleanSession {
				sessionReuse = true
			} else if srv.hooks.OnSessionTerminated != nil {
				srv.hooks.OnSessionTerminated(context.Background(), oldClient, ConflictTermination)
			}
		} else if srv.hooks.OnSessionTerminated != nil {
			srv.hooks.OnSessionTerminated(context.Background(), oldClient, ConflictTermination)
		}
	}
	ack := connect.NewConnackPacket(sessionReuse)
	client.out <- ack
	client.setConnected()
	if sessionReuse { //发送还未确认的消息和离线消息队列 sending inflight messages & offline message
		client.session.unackpublish = oldSession.unackpublish
		client.statsManager = oldClient.statsManager
		//send unacknowledged publish
		oldSession.inflightMu.Lock()
		for e := oldSession.inflight.Front(); e != nil; e = e.Next() {
			if inflight, ok := e.Value.(*inflightElem); ok {
				pub := inflight.packet
				pub.Dup = true
				client.statsManager.decInflightCurrent(1)
				client.onlinePublish(pub)
			}
		}
		oldSession.inflightMu.Unlock()
		//send unacknowledged pubrel
		oldSession.awaitRelMu.Lock()
		for e := oldSession.awaitRel.Front(); e != nil; e = e.Next() {
			if await, ok := e.Value.(*awaitRelElem); ok {
				pid := await.pid
				pubrel := &packets.Pubrel{
					FixHeader: &packets.FixHeader{
						PacketType:   packets.PUBREL,
						Flags:        packets.FLAG_PUBREL,
						RemainLength: 2,
					},
					PacketID: pid,
				}
				client.setAwaitRel(pid)
				client.session.setPacketID(pid)
				client.statsManager.decAwaitCurrent(1)
				client.out <- pubrel
			}
		}
		oldSession.awaitRelMu.Unlock()

		//send offline msg
		oldSession.msgQueueMu.Lock()
		for e := oldSession.msgQueue.Front(); e != nil; e = e.Next() {
			if publish, ok := e.Value.(*packets.Publish); ok {
				client.statsManager.messageDequeue(1)
				client.onlinePublish(publish)
			}
		}
		oldSession.msgQueueMu.Unlock()

		zaplog.Info("logged in with session reuse",
			zap.String("remote_addr", client.rwc.RemoteAddr().String()),
			zap.String("client_id", client.opts.clientID))
	} else {
		if oldExist {
			srv.subscriptionsDB.UnsubscribeAll(client.opts.clientID)
		}
		zaplog.Info("logged in with new session",
			zap.String("remote_addr", client.rwc.RemoteAddr().String()),
			zap.String("client_id", client.opts.clientID),
		)
	}
	if sessionReuse {
		if srv.hooks.OnSessionResumed != nil {
			srv.hooks.OnSessionResumed(context.Background(), client)
		}
	} else {
		if srv.hooks.OnSessionCreated != nil {
			srv.hooks.OnSessionCreated(context.Background(), client)
		}
	}
	delete(srv.offlineClients, client.opts.clientID)
}
func (srv *server) unregisterHandler(unregister *unregister) {
	defer close(unregister.done)
	client := unregister.client
	client.setDisConnected()
	select {
	case <-client.ready:
	default:
		// default means the client is closed before srv.registerHandler(),
		// session is not created, so there is no need to unregister.
		return
	}
clearIn:
	for {
		select {
		case p := <-client.in:
			if _, ok := p.(*packets.Disconnect); ok {
				client.cleanWillFlag = true
			}
		default:
			break clearIn
		}
	}

	if !client.cleanWillFlag && client.opts.willFlag {
		willMsg := &packets.Publish{
			Dup:       false,
			Qos:       client.opts.willQos,
			Retain:    false,
			TopicName: []byte(client.opts.willTopic),
			Payload:   client.opts.willPayload,
		}
		msg := messageFromPublish(willMsg)
		go func() {
			msgRouter := &msgRouter{msg: msg, match: true}
			client.server.msgRouter <- msgRouter
		}()
	}
	if client.opts.cleanSession {
		zaplog.Info("logged out and cleaning session",
			zap.String("remote_addr", client.rwc.RemoteAddr().String()),
			zap.String("client_id", client.OptionsReader().ClientID()),
		)
		srv.mu.Lock()
		srv.removeSession(client.opts.clientID)
		srv.mu.Unlock()
		if srv.hooks.OnSessionTerminated != nil {
			srv.hooks.OnSessionTerminated(context.Background(), client, NormalTermination)
		}
		srv.statsManager.messageDequeue(client.statsManager.GetStats().MessageStats.QueuedCurrent)
	} else { //store session 保持session
		srv.mu.Lock()
		srv.offlineClients[client.opts.clientID] = time.Now()
		srv.mu.Unlock()
		zaplog.Info("logged out and storing session",
			zap.String("remote_addr", client.rwc.RemoteAddr().String()),
			zap.String("client_id", client.OptionsReader().ClientID()),
		)
		//clear  out
	clearOut:
		for {
			select {
			case p := <-client.out:
				if p, ok := p.(*packets.Publish); ok {
					client.msgEnQueue(p)
				}
			default:
				break clearOut
			}
		}
		srv.statsManager.addSessionInactive()
	}
}

// 所有进来的 msg都会分配pid，指定pid重传的不在这里处理
func (srv *server) msgRouterHandler(m *msgRouter) {
	msg := m.msg
	var matched subscription.ClientTopics
	if m.match {
		matched = srv.subscriptionsDB.GetTopicMatched(msg.Topic())
		if m.clientID != "" {
			tmp, ok := matched[m.clientID]
			matched = make(subscription.ClientTopics)
			if ok {
				matched[m.clientID] = tmp
			}
		}
	} else {
		// no need to search in subscriptionsDB.
		matched = make(subscription.ClientTopics)
		matched[m.clientID] = append(matched[m.clientID], packets.Topic{
			Qos:  msg.Qos(),
			Name: msg.Topic(),
		})
	}
	srv.mu.RLock()
	defer srv.mu.RUnlock()
	for cid, topics := range matched {
		if srv.config.DeliveryMode == Overlap {
			for _, t := range topics {
				if c, ok := srv.clients[cid]; ok {
					publish := messageToPublish(msg)
					if publish.Qos > t.Qos {
						publish.Qos = t.Qos
					}
					publish.Dup = false
					c.publish(publish)
				}
			}
		} else {
			// deliver once
			var maxQos uint8
			for _, t := range topics {
				if t.Qos > maxQos {
					maxQos = t.Qos
				}
				if maxQos == packets.QOS_2 {
					break
				}
			}
			if c, ok := srv.clients[cid]; ok {
				publish := messageToPublish(msg)
				if publish.Qos > maxQos {
					publish.Qos = maxQos
				}
				publish.Dup = false
				c.publish(publish)
			}
		}
	}
}
func (srv *server) removeSession(clientID string) {
	delete(srv.clients, clientID)
	delete(srv.offlineClients, clientID)
	srv.subscriptionsDB.UnsubscribeAll(clientID)
}

// sessionExpireCheck 判断是否超时
// sessionExpireCheck check and terminate expired sessions
func (srv *server) sessionExpireCheck() {
	expire := srv.config.SessionExpiryCheckInterval
	if expire == 0 {
		return
	}
	now := time.Now()
	srv.mu.Lock()
	for id, disconnectedAt := range srv.offlineClients {
		if now.Sub(disconnectedAt) >= expire {
			if client, _ := srv.clients[id]; client != nil {
				srv.removeSession(id)
				if srv.hooks.OnSessionTerminated != nil {
					srv.hooks.OnSessionTerminated(context.Background(), client, ExpiredTermination)
				}
				srv.statsManager.addSessionExpired()
				srv.statsManager.decSessionInactive()
			}
		}
	}
	srv.mu.Unlock()

}

// server event loop
func (srv *server) eventLoop() {
	if srv.config.SessionExpiryInterval != 0 {
		sessionExpireTimer := time.NewTicker(srv.config.SessionExpiryCheckInterval)
		defer sessionExpireTimer.Stop()
		for {
			select {
			case register := <-srv.register:
				srv.registerHandler(register)
			case unregister := <-srv.unregister:
				srv.unregisterHandler(unregister)
			case msg := <-srv.msgRouter:
				srv.msgRouterHandler(msg)
			case <-sessionExpireTimer.C:
				srv.sessionExpireCheck()
			}
		}
	} else {
		for {
			select {
			case register := <-srv.register:
				srv.registerHandler(register)
			case unregister := <-srv.unregister:
				srv.unregisterHandler(unregister)
			case msg := <-srv.msgRouter:
				srv.msgRouterHandler(msg)
			}
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

// NewServer returns a gmqtt server instance with the given options
func NewServer(opts ...Options) *server {
	// statistics
	subStore := subscription_trie.NewStore()
	statsMgr := newStatsManager(subStore)
	srv := &server{
		status:          serverStatusInit,
		exitChan:        make(chan struct{}),
		clients:         make(map[string]*client),
		offlineClients:  make(map[string]time.Time),
		retainedDB:      retained_trie.NewStore(),
		subscriptionsDB: subStore,
		config:          DefaultConfig,
		statsManager:    statsMgr,
	}
	srv.publishService = &publishService{server: srv}
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

		// onAccept hooks
		if srv.hooks.OnAccept != nil {
			if !srv.hooks.OnAccept(context.Background(), rw) {
				rw.Close()
				continue
			}
		}

		client := srv.newClient(rw)
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
// wsConn implements the io.ReadWriter
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

func (srv *server) newClient(c net.Conn) *client {
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
		opts:          &options{},
		cleanWillFlag: false,
		ready:         make(chan struct{}),
		statsManager:  newSessionStatsManager(),
	}
	client.packetReader = packets.NewReader(client.bufr)
	client.packetWriter = packets.NewWriter(client.bufw)
	client.setConnecting()
	client.newSession()
	return client
}

func (srv *server) loadPlugins() error {
	var (
		onAcceptWrappers           []OnAcceptWrapper
		onConnectWrappers          []OnConnectWrapper
		onConnectedWrappers        []OnConnectedWrapper
		onSessionCreatedWrapper    []OnSessionCreatedWrapper
		onSessionResumedWrapper    []OnSessionResumedWrapper
		onSessionTerminatedWrapper []OnSessionTerminatedWrapper
		onSubscribeWrappers        []OnSubscribeWrapper
		onSubscribedWrappers       []OnSubscribedWrapper
		onUnsubscribeWrappers      []OnUnsubscribeWrapper
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
		if hooks.OnConnectWrapper != nil {
			onConnectWrappers = append(onConnectWrappers, hooks.OnConnectWrapper)
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

	// onAccept
	if onAcceptWrappers != nil {
		onAccept := func(ctx context.Context, conn net.Conn) bool {
			return true
		}
		for i := len(onAcceptWrappers); i > 0; i-- {
			onAccept = onAcceptWrappers[i-1](onAccept)
		}
		srv.hooks.OnAccept = onAccept
	}

	// onConnect
	if onConnectWrappers != nil {
		onConnect := func(ctx context.Context, client Client) (code uint8) {
			return packets.CodeAccepted
		}
		for i := len(onConnectWrappers); i > 0; i-- {
			onConnect = onConnectWrappers[i-1](onConnect)
		}
		srv.hooks.OnConnect = onConnect
	}

	// onConnected
	if onConnectedWrappers != nil {
		onConnected := func(ctx context.Context, client Client) {}
		for i := len(onConnectedWrappers); i > 0; i-- {
			onConnected = onConnectedWrappers[i-1](onConnected)
		}
		srv.hooks.OnConnected = onConnected
	}

	// onSessionCreated
	if onSessionCreatedWrapper != nil {
		onSessionCreated := func(ctx context.Context, client Client) {}
		for i := len(onSessionCreatedWrapper); i > 0; i-- {
			onSessionCreated = onSessionCreatedWrapper[i-1](onSessionCreated)
		}
		srv.hooks.OnSessionCreated = onSessionCreated
	}

	// onSessionResumed
	if onSessionResumedWrapper != nil {
		onSessionResumed := func(ctx context.Context, client Client) {}
		for i := len(onSessionResumedWrapper); i > 0; i-- {
			onSessionResumed = onSessionResumedWrapper[i-1](onSessionResumed)
		}
		srv.hooks.OnSessionResumed = onSessionResumed
	}

	// onSessionTerminated
	if onSessionTerminatedWrapper != nil {
		onSessionTerminated := func(ctx context.Context, client Client, reason SessionTerminatedReason) {}
		for i := len(onSessionTerminatedWrapper); i > 0; i-- {
			onSessionTerminated = onSessionTerminatedWrapper[i-1](onSessionTerminated)
		}
		srv.hooks.OnSessionTerminated = onSessionTerminated
	}

	// onSubscribe
	if onSubscribeWrappers != nil {
		onSubscribe := func(ctx context.Context, client Client, topic packets.Topic) (qos uint8) {
			return topic.Qos
		}
		for i := len(onSubscribeWrappers); i > 0; i-- {
			onSubscribe = onSubscribeWrappers[i-1](onSubscribe)
		}
		srv.hooks.OnSubscribe = onSubscribe
	}

	// onSubscribed
	if onSubscribedWrappers != nil {
		onSubscribed := func(ctx context.Context, client Client, topic packets.Topic) {}
		for i := len(onSubscribedWrappers); i > 0; i-- {
			onSubscribed = onSubscribedWrappers[i-1](onSubscribed)
		}
		srv.hooks.OnSubscribed = onSubscribed
	}

	//onUnsubscribe
	if onUnsubscribeWrappers != nil {
		onUnsubscribe := func(ctx context.Context, client Client, topicName string) {}
		for i := len(onUnsubscribeWrappers); i > 0; i-- {
			onUnsubscribe = onUnsubscribeWrappers[i-1](onUnsubscribe)
		}
		srv.hooks.OnUnsubscribe = onUnsubscribe
	}

	//onUnsubscribed
	if onUnsubscribedWrappers != nil {
		onUnsubscribed := func(ctx context.Context, client Client, topicName string) {}
		for i := len(onUnsubscribedWrappers); i > 0; i-- {
			onUnsubscribed = onUnsubscribedWrappers[i-1](onUnsubscribed)
		}
		srv.hooks.OnUnsubscribed = onUnsubscribed
	}

	// onMsgArrived
	if onMsgArrivedWrappers != nil {
		onMsgArrived := func(ctx context.Context, client Client, msg packets.Message) (valid bool) {
			return true
		}
		for i := len(onMsgArrivedWrappers); i > 0; i-- {
			onMsgArrived = onMsgArrivedWrappers[i-1](onMsgArrived)
		}
		srv.hooks.OnMsgArrived = onMsgArrived
	}

	// onDeliver
	if onDeliverWrappers != nil {
		onDeliver := func(ctx context.Context, client Client, msg packets.Message) {}
		for i := len(onDeliverWrappers); i > 0; i-- {
			onDeliver = onDeliverWrappers[i-1](onDeliver)
		}
		srv.hooks.OnDeliver = onDeliver
	}

	// onAcked
	if onAckedWrappers != nil {
		onAcked := func(ctx context.Context, client Client, msg packets.Message) {}
		for i := len(onAckedWrappers); i > 0; i-- {
			onAcked = onAckedWrappers[i-1](onAcked)
		}
		srv.hooks.OnAcked = onAcked
	}

	// onClose hooks
	if onCloseWrappers != nil {
		onClose := func(ctx context.Context, client Client, err error) {}
		for i := len(onCloseWrappers); i > 0; i-- {
			onClose = onCloseWrappers[i-1](onClose)
		}
		srv.hooks.OnClose = onClose
	}

	// onStop
	if onStopWrappers != nil {
		onStop := func(ctx context.Context) {}
		for i := len(onStopWrappers); i > 0; i-- {
			onStop = onStopWrappers[i-1](onStop)
		}
		srv.hooks.OnStop = onStop
	}

	// onMsgDropped
	if onMsgDroppedWrappers != nil {
		onMsgDropped := func(ctx context.Context, client Client, msg packets.Message) {}
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
			zaplog.Warn("websocket upgrade error", zap.String("msg", err.Error()))
			return
		}
		defer c.Close()
		conn := &wsConn{c.UnderlyingConn(), c}
		client := srv.newClient(conn)
		client.serve()
	}
}

// Run starts the mqtt server. This method is non-blocking
func (srv *server) Run() {
	srv.msgRouter = make(chan *msgRouter, srv.config.MsgRouterLen)
	srv.register = make(chan *register, srv.config.RegisterLen)
	srv.unregister = make(chan *unregister, srv.config.UnregisterLen)

	var tcps []string
	var ws []string
	for _, v := range srv.tcpListener {
		tcps = append(tcps, v.Addr().String())
	}
	for _, v := range srv.websocketServer {
		ws = append(ws, v.Server.Addr)
	}
	zaplog.Info("starting gmqtt server", zap.Strings("tcp server listen on", tcps), zap.Strings("websocket server listen on", ws))

	err := srv.loadPlugins()
	if err != nil {
		panic(err)
	}
	srv.status = serverStatusStarted
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
}

// Stop gracefully stops the mqtt server by the following steps:
//  1. Closing all open TCP listeners and shutting down all open websocket servers
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
