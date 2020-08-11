package gmqtt

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"

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
)

// Default configration
const (
	DefaultMsgRouterLen = 4096
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

	config  Config
	hooks   Hooks
	plugins []Plugable

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
	SessionExpiryInterval      uint32
	SessionExpiryCheckInterval uint32
	QueueQos0Messages          bool
	MaxInflight                int
	MaxAwaitRel                int
	MaxMsgQueue                int
	DeliveryMode               DeliveryMode

	MQTT struct {
		SharedSubAvailable      bool
		WildcardAvailable       bool
		SubscriptionIDAvailable bool
		ReceiveMax              uint16
		RetainAvailable         bool
		MaxPacketSize           uint32
		TopicAliasMax           uint16
		MaximumQOS              byte
		MaxKeepAlive            uint16
	}
}

// DefaultConfig default config used by NewServer()
var DefaultConfig = Config{
	RetryInterval:              20 * time.Second,
	RetryCheckInterval:         20 * time.Second,
	SessionExpiryInterval:      65535,
	SessionExpiryCheckInterval: 100,
	QueueQos0Messages:          true,
	MaxInflight:                32,
	MaxAwaitRel:                100,
	MaxMsgQueue:                1000,
	DeliveryMode:               OnlyOnce,
	MQTT: struct {
		SharedSubAvailable      bool
		WildcardAvailable       bool
		SubscriptionIDAvailable bool
		ReceiveMax              uint16
		RetainAvailable         bool
		MaxPacketSize           uint32
		TopicAliasMax           uint16
		MaximumQOS              byte
		MaxKeepAlive            uint16
	}{
		SharedSubAvailable:      true,
		WildcardAvailable:       true,
		SubscriptionIDAvailable: true,
		ReceiveMax:              2,
		RetainAvailable:         true,
		MaxPacketSize:           0,
		TopicAliasMax:           10,
		MaximumQOS:              2,
		MaxKeepAlive:            60,
	},
}

// GetConfig returns the config of the server
func (srv *server) GetConfig() Config {
	return srv.config
}

// GetStatsManager returns StatsManager
func (srv *server) GetStatsManager() StatsManager {
	return srv.statsManager
}

type msgRouter struct {
	msg         packets.Message
	clientID    string
	srcClientID string
}

// Status returns the server status
func (srv *server) Status() int32 {
	return atomic.LoadInt32(&srv.status)
}

// 已经判断是成功了，注册
func (srv *server) registerClient(connect *packets.Connect, ack *AuthResponse, client *client) {
	defer close(client.ready)
	srv.mu.Lock()
	defer srv.mu.Unlock()
	srv.statsManager.addClientConnected()
	srv.statsManager.addSessionActive()
	client.setConnectedAt(time.Now())
	client.setConnected()
	if srv.hooks.OnConnected != nil {
		srv.hooks.OnConnected(context.Background(), client)
	}
	var sessionResume bool

	var oldSession *session
	oldClient, oldExist := srv.clients[client.opts.ClientID]
	srv.clients[client.opts.ClientID] = client
	if oldExist {
		oldSession = oldClient.session
		if oldClient.IsConnected() {
			zaplog.Info("logging with duplicate ClientID",
				zap.String("remote", client.rwc.RemoteAddr().String()),
				zap.String("client_id", client.opts.ClientID),
			)
			oldClient.setSwitching()
			oldClient.setError(codes.NewError(codes.SessionTakenOver))
			<-oldClient.Close()
			if oldClient.opts.Will.Flag {
				willMsg := &packets.Publish{
					Dup:       false,
					Qos:       oldClient.opts.Will.Qos,
					Retain:    oldClient.opts.Will.Retain,
					TopicName: oldClient.opts.Will.Topic,
					Payload:   oldClient.opts.Will.Payload,
				}

				srv.deliverMessage(oldClient, messageFromPublish(willMsg))

			}
			if !client.opts.CleanStart && !oldClient.opts.CleanStart { //reuse old session
				sessionResume = true
			}
		} else if oldClient.IsDisConnected() {
			if !client.opts.CleanStart {
				sessionResume = true
			} else if srv.hooks.OnSessionTerminated != nil {
				srv.hooks.OnSessionTerminated(context.Background(), oldClient, ConflictTermination)
			}
		} else if srv.hooks.OnSessionTerminated != nil {
			srv.hooks.OnSessionTerminated(context.Background(), oldClient, ConflictTermination)
		}
	}

	connack := connect.NewConnackPacket(codes.Success, sessionResume)
	connack.Properties = ack.Properties
	client.out <- connack

	if sessionResume { //发送还未确认的消息和离线消息队列 sending inflight messages & offline message
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
			zap.String("client_id", client.opts.ClientID))
	} else {
		if oldExist {
			srv.subscriptionsDB.UnsubscribeAll(client.opts.ClientID)
		}
		zaplog.Info("logged in with new session",
			zap.String("remote_addr", client.rwc.RemoteAddr().String()),
			zap.String("client_id", client.opts.ClientID),
		)
	}
	if sessionResume {
		if srv.hooks.OnSessionResumed != nil {
			srv.hooks.OnSessionResumed(context.Background(), client)
		}
	} else {
		if srv.hooks.OnSessionCreated != nil {
			srv.hooks.OnSessionCreated(context.Background(), client)
		}
	}
	delete(srv.offlineClients, client.opts.ClientID)
	return
}

func (srv *server) unregisterClient(client *client) {
	if !client.IsConnected() {
		return
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	client.setDisConnected()
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
	if !client.cleanWillFlag && client.opts.Will.Flag {
		willMsg := &packets.Publish{
			Version:    client.version,
			Dup:        false,
			Qos:        client.opts.Will.Qos,
			Retain:     false,
			TopicName:  client.opts.Will.Topic,
			Payload:    client.opts.Will.Payload,
			Properties: client.opts.Will.Properties,
		}
		msg := messageFromPublish(willMsg)

		srv.deliverMessage(client, msg)
	}
	if client.opts.CleanStart && client.opts.SessionExpiry == 0 {
		zaplog.Info("logged out and cleaning session",
			zap.String("remote_addr", client.rwc.RemoteAddr().String()),
			zap.String("client_id", client.opts.ClientID),
		)
		srv.removeSession(client.opts.ClientID)
		if srv.hooks.OnSessionTerminated != nil {
			srv.hooks.OnSessionTerminated(context.Background(), client, NormalTermination)
		}
		srv.statsManager.messageDequeue(client.statsManager.GetStats().MessageStats.QueuedCurrent)
	} else { //store session 保持session
		srv.offlineClients[client.opts.ClientID] = time.Now()
		zaplog.Info("logged out and storing session",
			zap.String("remote_addr", client.rwc.RemoteAddr().String()),
			zap.String("client_id", client.opts.ClientID),
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

func (srv *server) deliverMessage(src *client, msg packets.Message) {
	// TODO no local 等option
	// TODO 计算报文大小。太大的就不要发了

	// subscriber (client id) list of shared subscriptions
	sharedList := make(map[string][]struct {
		clientID string
		sub      subscription.Subscription
	})

	maxQos := make(map[string]*struct {
		sub    subscription.Subscription
		subIDs []uint32
	})
	srcClientID := src.opts.ClientID
	// 先取的所有的share 和unshare
	srv.subscriptionsDB.Iterate(func(clientID string, sub subscription.Subscription) bool {
		if sub.NoLocal() && clientID == srcClientID {
			return true
		}
		// shared
		if sub.ShareName() != "" {
			sharedList[sub.TopicFilter()] = append(sharedList[sub.TopicFilter()], struct {
				clientID string
				sub      subscription.Subscription
			}{clientID: clientID, sub: sub})
		} else {
			if srv.config.DeliveryMode == Overlap {
				if c, ok := srv.clients[clientID]; ok {
					publish := messageToPublish(msg, c.version)
					if publish.Qos > sub.QoS() {
						publish.Qos = sub.QoS()
					}
					if c.version == packets.Version5 && sub.ID() != 0 {
						publish.Properties.SubscriptionIdentifier = []uint32{sub.ID()}
					}
					publish.Dup = false
					if !sub.RetainAsPublished() {
						publish.Retain = false
					}
					c.publish(publish)
				}
			} else {
				if maxQos[clientID] == nil {
					maxQos[clientID] = &struct {
						sub    subscription.Subscription
						subIDs []uint32
					}{sub: sub, subIDs: []uint32{sub.ID()}}
				}
				if maxQos[clientID].sub.QoS() < sub.QoS() {
					maxQos[clientID].sub = sub
				}
				maxQos[clientID].subIDs = append(maxQos[clientID].subIDs, sub.ID())
			}
		}
		return true
	}, subscription.IterationOptions{
		Type:      subscription.TypeAll,
		MatchType: subscription.MatchFilter,
		TopicName: msg.Topic(),
	})

	if srv.config.DeliveryMode == OnlyOnce {
		for clientID, v := range maxQos {
			if c := srv.clients[clientID]; c != nil {
				publish := messageToPublish(msg, c.version)
				if publish.Qos > v.sub.QoS() {
					publish.Qos = v.sub.QoS()
				}
				if c.version == packets.Version5 {
					for _, id := range v.subIDs {
						if id != 0 {
							publish.Properties.SubscriptionIdentifier = append(publish.Properties.SubscriptionIdentifier, id)
						}
					}
				}
				publish.Dup = false
				if !v.sub.RetainAsPublished() {
					publish.Retain = false
				}
				c.publish(publish)
			}
		}
	}

	// unshare
	// TODO 实现一个钩子函数，自定义随机逻辑。 在shared里面挑一个
	for _, v := range sharedList {
		if c, ok := srv.clients[v[0].clientID]; ok {
			publish := messageToPublish(msg, c.version)
			if publish.Qos > v[0].sub.QoS() {
				publish.Qos = v[0].sub.QoS()
			}
			publish.Properties.SubscriptionIdentifier = []uint32{v[0].sub.ID()}
			publish.Dup = false
			if !v[0].sub.RetainAsPublished() {
				publish.Retain = false
			}
			c.publish(publish)
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
	expire := time.Second * time.Duration(srv.config.SessionExpiryCheckInterval)
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
		sessionExpireTimer := time.NewTicker(time.Second * time.Duration(srv.config.SessionExpiryCheckInterval))
		defer sessionExpireTimer.Stop()
		for {
			select {
			case <-sessionExpireTimer.C:
				srv.sessionExpireCheck()
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
		opts:          &ClientOptions{},
		cleanWillFlag: false,
		ready:         make(chan struct{}),
		statsManager:  newSessionStatsManager(),
		aliasMapper: &aliasMapper{
			client:    nil,
			server:    nil,
			nextAlias: 1,
		},
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
		onConnect := func(ctx context.Context, req ConnectRequest, client Client) *AuthResponse {
			return &AuthResponse{
				Code:       codes.Success,
				Properties: req.DefaultConnackProperties(),
			}
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
