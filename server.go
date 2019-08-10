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

	"fmt"

	"github.com/DrmagicE/gmqtt/logger"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/gorilla/websocket"
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

// Message represent a publish packet
type Message interface {
	Dup() bool
	Qos() uint8
	Retained() bool
	Topic() string
	PacketID() packets.PacketID
	Payload() []byte
}
type msg struct {
	dup      bool
	qos      uint8
	retained bool
	topic    string
	packetID packets.PacketID
	payload  []byte
}

func (m *msg) Dup() bool {
	return m.dup
}

func (m *msg) Qos() uint8 {
	return m.qos
}

func (m *msg) Retained() bool {
	return m.retained
}

func (m *msg) Topic() string {
	return m.topic
}

func (m *msg) PacketID() packets.PacketID {
	return m.packetID
}

func (m *msg) Payload() []byte {
	return m.payload
}

func messageFromPublish(p *packets.Publish) *msg {
	return &msg{
		dup:      p.Dup,
		qos:      p.Qos,
		retained: p.Retain,
		topic:    string(p.TopicName),
		packetID: p.PacketID,
		payload:  p.Payload,
	}
}

// ServerService is mainly used by plugin to interact with Server
type ServerService interface {
	// Publish publishes a message to the broker.
	Publish(topic string, payload []byte, qos uint8, retain bool)
	// Subscribe subscribes topics for the client specified by clientID.
	Subscribe(clientID string, topics []packets.Topic)
	// UnSubscribe unsubscribes topics for the client specified by clientID.
	UnSubscribe(clientID string, topics []string)
	// Client return the client specified by clientID.
	Client(clientID string) Client
	// GetConfig returns the config of the server
	GetConfig() Config
}

// Server represents a mqtt server instance.
// Create a Server by using NewServer() or DefaultServer()
type Server struct {
	wg      sync.WaitGroup
	mu      sync.RWMutex //gard clients & offlineClients map
	status  int32        //server status
	clients map[string]*client
	// offlineClients store the disconnected time of all disconnected clients with valid session(not expired). Key by clientID
	offlineClients  map[string]time.Time
	tcpListener     []net.Listener //tcp listeners
	websocketServer []*WsServer    //websocket server
	exitChan        chan struct{}
	retainedMsgMu   sync.Mutex
	retainedMsg     map[string]*packets.Publish //retained msg, key by topic name

	subscriptionsDB *trieDB //store subscriptions

	msgRouter  chan *msgRouter
	register   chan *register   //register session
	unregister chan *unregister //unregister session

	config Config
	//hooks
	onAccept            OnAccept
	onConnect           OnConnect
	onConnected         OnConnected
	onSessionCreated    OnSessionCreated
	onSessionResumed    OnSessionResumed
	onSessionTerminated OnSessionTerminated
	onSubscribe         OnSubscribe
	onSubscribed        OnSubscribed
	onUnsubscribed      OnUnsubscribed
	onMsgArrived        OnMsgArrived
	onDeliver           OnDeliver
	onAcked             OnAcked
	onMsgDropped        OnMsgDropped
	onClose             OnClose
	onStop              OnStop

	// 所有的插件
	plugins []Plugable
}

func (srv *Server) checkStatus() {
	if srv.Status() != serverStatusInit {
		panic(statusPanic)
	}
}

// RegisterOnAccept registers a onAccept callback.
// A panic will cause if any RegisterOnXXX is called after server.Run()
func (srv *Server) RegisterOnAccept(callback OnAccept) {
	srv.checkStatus()
	srv.onAccept = callback
}

// RegisterOnConnect registers a onConnect callback.
func (srv *Server) RegisterOnConnect(callback OnConnect) {
	srv.checkStatus()
	srv.onConnect = callback
}

// RegisterOnConnect registers a onConnected callback.
func (srv *Server) RegisterOnConnected(callback OnConnected) {
	srv.checkStatus()
	srv.onConnected = callback
}

// RegisterOnSessionCreated registers a OnSessionCreated callback.
func (srv *Server) RegisterOnSessionCreated(callback OnSessionCreated) {
	srv.checkStatus()
	srv.onSessionCreated = callback
}

// RegisterOnSessionResumed registers a OnSessionResumed callback.
func (srv *Server) RegisterOnSessionResumed(callback OnSessionResumed) {
	srv.checkStatus()
	srv.onSessionResumed = callback
}

// RegisterOnConnect registers a OnSessionTerminated callback.
func (srv *Server) RegisterOnSessionTerminated(callback OnSessionTerminated) {
	srv.checkStatus()
	srv.onSessionTerminated = callback
}

// RegisterOnSubscribe registers a onSubscribe callback.
func (srv *Server) RegisterOnSubscribe(callback OnSubscribe) {
	srv.checkStatus()
	srv.onSubscribe = callback
}

// RegisterOnSubscribe registers a onSubscribed callback.
func (srv *Server) RegisterOnSubscribed(callback OnSubscribed) {
	srv.checkStatus()
	srv.onSubscribed = callback
}

// RegisterOnUnsubscribed registers a onUnsubscribed callback.
func (srv *Server) RegisterOnUnsubscribed(callback OnUnsubscribed) {
	srv.checkStatus()
	srv.onUnsubscribed = callback
}

// RegisterOnMsgArrived registers a onMsgArrived callback.
func (srv *Server) RegisterOnMsgArrived(callback OnMsgArrived) {
	srv.checkStatus()
	srv.onMsgArrived = callback
}

// RegisterOnMsgDropped registers a onAcked callback.
func (srv *Server) RegisterOnMsgDropped(callback OnMsgDropped) {
	srv.checkStatus()
	srv.onMsgDropped = callback
}

// RegisterOnDeliver registers a onDeliver callback.
func (srv *Server) RegisterOnDeliver(callback OnDeliver) {
	srv.checkStatus()
	srv.onDeliver = callback
}

// RegisterOnAcked registers a onAcked callback.
func (srv *Server) RegisterOnAcked(callback OnAcked) {
	srv.checkStatus()
	srv.onAcked = callback
}

// RegisterOnClose registers a onClose callback.
func (srv *Server) RegisterOnClose(callback OnClose) {
	srv.checkStatus()
	srv.onClose = callback
}

// RegisterOnStop registers a onStop callback.
func (srv *Server) RegisterOnStop(callback OnStop) {
	srv.checkStatus()
	srv.onStop = callback
}

var log *logger.Logger

// SetLogger sets the logger. It is used in DEBUG mode.
func SetLogger(l *logger.Logger) {
	log = l
}

type DeliverMode int

const (
	Overlap  DeliverMode = 0
	OnlyOnce DeliverMode = 1
)

type Config struct {
	RetryInterval              time.Duration
	RetryCheckInterval         time.Duration
	SessionExpiryInterval      time.Duration
	SessionExpireCheckInterval time.Duration
	QueueQos0Messages          bool
	MaxInflight                int
	MaxAwaitRel                int
	MaxMsgQueue                int
	DeliverMode                DeliverMode
}

// DefaultConfig default config used by NewServer()
var DefaultConfig = Config{
	RetryInterval:              20 * time.Second,
	RetryCheckInterval:         20 * time.Second,
	SessionExpiryInterval:      0,
	SessionExpireCheckInterval: 0,
	QueueQos0Messages:          true,
	MaxInflight:                32,
	MaxAwaitRel:                100,
	MaxMsgQueue:                1000,
	DeliverMode:                OnlyOnce,
}

// GetConfig returns the config of the server
func (srv *Server) GetConfig() Config {
	return srv.config
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
	pub *packets.Publish
}

// Status returns the server status
func (srv *Server) Status() int32 {
	return atomic.LoadInt32(&srv.status)
}

func (srv *Server) registerHandler(register *register) {
	// ack code set in Connack Packet
	var code uint8
	client := register.client
	defer close(client.ready)
	connect := register.connect
	var sessionReuse bool
	if connect.AckCode != packets.CodeAccepted {
		err := errors.New("reject connection, ack code:" + strconv.Itoa(int(connect.AckCode)))
		ack := connect.NewConnackPacket(false)
		//client.out <- ack
		client.writePacket(ack)
		register.error = err
		return
	}
	if srv.onConnect != nil {
		code = srv.onConnect(&chainStore{}, client)
	}
	connect.AckCode = code
	if code != packets.CodeAccepted {
		err := errors.New("reject connection, ack code:" + strconv.Itoa(int(code)))
		ack := connect.NewConnackPacket(false)
		//client.out <- ack
		client.writePacket(ack)
		client.setError(err)
		register.error = err
		return
	}
	if srv.onConnected != nil {
		srv.onConnected(&chainStore{}, client)
	}
	client.setConnectedAt(time.Now())
	srv.mu.Lock()
	defer srv.mu.Unlock()
	var oldSession *session
	oldClient, oldExist := srv.clients[client.opts.clientID]
	srv.clients[client.opts.clientID] = client
	if oldExist {
		oldSession = oldClient.session
		if oldClient.IsConnected() {
			if log != nil {
				log.Printf("%-15s %v: logging with duplicate ClientID: %s", "", client.rwc.RemoteAddr(), client.OptionsReader().ClientID())
			}
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
					msgRouter := &msgRouter{pub: willMsg}
					srv.msgRouter <- msgRouter
				}()
			}
			if !client.opts.cleanSession && !oldClient.opts.cleanSession { //reuse old session
				sessionReuse = true
				/*			clearOut:
							for {
								select {
								case p := <-oldClient.out:
									if p, ok := p.(*packets.Publish); ok {
										oldClient.msgEnQueue(p)
									}
								default:
									break clearOut
								}
							}*/
			}
		} else if oldClient.Status() == Disconnected {
			if !client.opts.cleanSession {
				sessionReuse = true
			} else if srv.onSessionTerminated != nil {
				srv.onSessionTerminated(&chainStore{}, oldClient, ConflictTermination)
			}
		} else if srv.onSessionTerminated != nil {
			srv.onSessionTerminated(&chainStore{}, oldClient, ConflictTermination)
		}
	}
	ack := connect.NewConnackPacket(sessionReuse)
	client.out <- ack
	client.setConnected()
	if sessionReuse { //发送还未确认的消息和离线消息队列 inflight & msgQueue
		client.session.unackpublish = oldSession.unackpublish
		client.addMsgDeliveredTotal(oldClient.MsgDeliveredTotal())
		client.addMsgDroppedTotal(oldClient.MsgDroppedTotal())
		client.addSubscriptionsCount(oldClient.SubscriptionsCount())
		//send unacknowledged publish
		oldSession.inflightMu.Lock()
		for e := oldSession.inflight.Front(); e != nil; e = e.Next() {
			if inflight, ok := e.Value.(*inflightElem); ok {
				pub := inflight.packet
				pub.Dup = true
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
				client.out <- pubrel
			}
		}
		oldSession.awaitRelMu.Unlock()

		//send offline msg
		oldSession.msgQueueMu.Lock()
		for e := oldSession.msgQueue.Front(); e != nil; e = e.Next() {
			if publish, ok := e.Value.(*packets.Publish); ok {
				client.onlinePublish(publish)
			}
		}
		oldSession.msgQueueMu.Unlock()
		if log != nil {
			log.Printf("%-15s %v: logined with session reuse", "", client.rwc.RemoteAddr())
		}
	} else {
		if oldExist {
			srv.subscriptionsDB.deleteAll(client.opts.clientID)
		}
		if log != nil {
			log.Printf("%-15s %v: logined with new session", "", client.rwc.RemoteAddr())
		}
	}
	if sessionReuse {
		if srv.onSessionResumed != nil {
			srv.onSessionResumed(&chainStore{}, client)
		}
	} else {
		if srv.onSessionCreated != nil {
			srv.onSessionCreated(&chainStore{}, client)
		}
	}
	delete(srv.offlineClients, client.opts.clientID)

}
func (srv *Server) unregisterHandler(unregister *unregister) {
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
		go func() {
			msgRouter := &msgRouter{pub: willMsg}
			client.server.msgRouter <- msgRouter
		}()
	}
	if client.opts.cleanSession {
		if log != nil {
			log.Printf("%-15s %v: logout & cleaning session", "", client.rwc.RemoteAddr())
		}
		srv.mu.Lock()
		srv.removeSession(client.opts.clientID)
		srv.mu.Unlock()
		if srv.onSessionTerminated != nil {
			srv.onSessionTerminated(&chainStore{}, client, NormalTermination)
		}

	} else { //store session 保持session
		srv.mu.Lock()
		srv.offlineClients[client.opts.clientID] = time.Now()
		srv.mu.Unlock()
		if log != nil {
			log.Printf("%-15s %v: logout & storing session", "", client.rwc.RemoteAddr())
		}
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
	}
}
func (srv *Server) msgRouterHandler(msg *msgRouter) {
	pub := msg.pub
	rs := srv.subscriptionsDB.getMatchedTopicFilter(string(pub.TopicName))
	srv.mu.RLock()
	defer srv.mu.RUnlock()
	for cid, topics := range rs {
		if srv.config.DeliverMode == Overlap {
			for _, t := range topics {
				publish := pub.CopyPublish()
				if publish.Qos > t.Qos {
					publish.Qos = t.Qos
				}
				publish.Dup = false
				if c, ok := srv.clients[cid]; ok {
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
			publish := pub.CopyPublish()
			if publish.Qos > maxQos {
				publish.Qos = maxQos
			}
			publish.Dup = false
			if c, ok := srv.clients[cid]; ok {
				c.publish(publish)
			}
		}
	}
}
func (srv *Server) removeSession(clientID string) {
	delete(srv.clients, clientID)
	delete(srv.offlineClients, clientID)
	srv.subscriptionsDB.deleteAll(clientID)
}

// sessionExpireCheck 判断是否超时
// sessionExpireCheck check and terminate expired sessions
func (srv *Server) sessionExpireCheck() {
	expire := srv.config.SessionExpireCheckInterval
	if expire == 0 {
		return
	}
	now := time.Now()
	srv.mu.Lock()
	for id, disconnectedAt := range srv.offlineClients {
		if now.Sub(disconnectedAt) >= expire {
			if client, _ := srv.clients[id]; client != nil {
				srv.removeSession(id)
				if srv.onSessionTerminated != nil {
					srv.onSessionTerminated(&chainStore{}, client, ExpiredTermination)
				}
			}
		}
	}
	srv.mu.Unlock()

}

// server event loop
func (srv *Server) eventLoop() {
	if srv.config.SessionExpiryInterval != 0 {
		sessionExpireTimer := time.NewTicker(srv.config.SessionExpireCheckInterval)
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
	CertFile string //TLS configration
	KeyFile  string //TLS configration
}

// DefaultServer returns a default gmqtt server instance
func DefaultServer() *Server {
	return &Server{
		status:          serverStatusInit,
		exitChan:        make(chan struct{}),
		clients:         make(map[string]*client),
		offlineClients:  make(map[string]time.Time),
		msgRouter:       make(chan *msgRouter, DefaultMsgRouterLen),
		register:        make(chan *register, DefaultRegisterLen),
		unregister:      make(chan *unregister, DefaultUnRegisterLen),
		retainedMsg:     make(map[string]*packets.Publish),
		subscriptionsDB: newTrieDB(),
		config:          DefaultConfig,
	}
}

// NewServer returns a gmqtt server instance with the given config
func NewServer(c Config) *Server {
	return &Server{
		status:          serverStatusInit,
		exitChan:        make(chan struct{}),
		clients:         make(map[string]*client),
		offlineClients:  make(map[string]time.Time),
		msgRouter:       make(chan *msgRouter, DefaultMsgRouterLen),
		register:        make(chan *register, DefaultRegisterLen),
		unregister:      make(chan *unregister, DefaultUnRegisterLen),
		retainedMsg:     make(map[string]*packets.Publish),
		subscriptionsDB: newTrieDB(),
		config:          c,
	}
}

// SetMsgRouterLen sets the length of msgRouter channel.
func (srv *Server) SetMsgRouterLen(i int) {
	srv.checkStatus()
	srv.msgRouter = make(chan *msgRouter, i)
}

// SetRegisterLen sets the length of register channel.
func (srv *Server) SetRegisterLen(i int) {
	srv.checkStatus()
	srv.register = make(chan *register, i)
}

// SetUnregisterLen sets the length of unregister channel.
func (srv *Server) SetUnregisterLen(i int) {
	srv.checkStatus()
	srv.unregister = make(chan *unregister, i)
}

// Publish 主动发布一个主题
//
// Publish publishs a message to the broker.
// 	Notice: This method will not trigger the onPublish callback
func (srv *Server) Publish(topic string, payload []byte, qos uint8, retain bool) {
	pub := &packets.Publish{
		Qos:       qos,
		TopicName: []byte(topic),
		Payload:   payload,
		Retain:    retain,
	}
	srv.msgRouter <- &msgRouter{pub}
}

// Subscribe 为某一个客户端订阅主题
//
// Subscribe subscribes topics for the client specified by clientID.
// 	Notice: This method will not trigger the onSubscribe callback
func (srv *Server) Subscribe(clientID string, topics []packets.Topic) {
	for _, v := range topics {
		srv.subscriptionsDB.subscribe(clientID, v)
	}
}

// UnSubscribe 为某一个客户端取消订阅某个主题
//
// UnSubscribe unsubscribes topics for the client specified by clientID.
func (srv *Server) UnSubscribe(clientID string, topics []string) {
	//client := srv.Client(clientID)
	//if client == nil {
	//	return
	//}
	for _, v := range topics {
		srv.subscriptionsDB.unsubscribe(clientID, v)
	}
}

// Client returns the client for given clientID
func (srv *Server) Client(clientID string) Client {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	return srv.clients[clientID]
}

// AddTCPListenner adds tcp listeners to mqtt server.
// This method enables the mqtt server to serve on multiple ports.
func (srv *Server) AddTCPListenner(ln ...net.Listener) {
	srv.checkStatus()
	srv.tcpListener = append(srv.tcpListener, ln...)
}

// AddWebSocketServer adds websocket server to mqtt server.
func (srv *Server) AddWebSocketServer(Server ...*WsServer) {
	srv.checkStatus()
	srv.websocketServer = append(srv.websocketServer, Server...)
}

func (srv *Server) serveTCP(l net.Listener) {
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
		if srv.onAccept != nil {
			if !srv.onAccept(&chainStore{}, rw) {
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

func (srv *Server) serveWebSocket(ws *WsServer) {
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

func (srv *Server) newClient(c net.Conn) *client {
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
	}
	client.packetReader = packets.NewReader(client.bufr)
	client.packetWriter = packets.NewWriter(client.bufw)
	client.setConnecting()
	client.newSession()
	return client
}

// AddPlugins 添加插件
func (srv *Server) AddPlugins(plugin ...Plugable) {
	srv.plugins = append(srv.plugins, plugin...)
}

func (srv *Server) loadPlugins() error {
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
	if onAckedWrappers != nil {
		onAccept := func(cs ChainStore, conn net.Conn) bool {
			return true
		}
		for i := len(onAcceptWrappers); i > 0; i-- {
			onAccept = onAcceptWrappers[i-1](onAccept)
		}
		srv.onAccept = onAccept
	}
	// onConnect
	if onConnectWrappers != nil {
		onConnect := func(cs ChainStore, client Client) (code uint8) {
			return packets.CodeAccepted
		}
		for i := len(onConnectWrappers); i > 0; i-- {
			onConnect = onConnectWrappers[i-1](onConnect)
		}
		srv.onConnect = onConnect
	}
	// onConnected
	if onConnectedWrappers != nil {
		onConnected := func(cs ChainStore, client Client) {}
		for i := len(onConnectedWrappers); i > 0; i-- {
			onConnected = onConnectedWrappers[i-1](onConnected)
		}
		srv.onConnected = onConnected
	}
	// onSessionCreated
	if onSessionCreatedWrapper != nil {
		onSessionCreated := func(cs ChainStore, client Client) {}
		for i := len(onSessionCreatedWrapper); i > 0; i-- {
			onSessionCreated = onSessionCreatedWrapper[i-1](onSessionCreated)
		}
		srv.onSessionCreated = onSessionCreated
	}

	// onSessionResumed
	if onSessionResumedWrapper != nil {
		onSessionResumed := func(cs ChainStore, client Client) {}
		for i := len(onSessionResumedWrapper); i > 0; i-- {
			onSessionResumed = onSessionResumedWrapper[i-1](onSessionResumed)
		}
		srv.onSessionResumed = onSessionResumed
	}

	// onSessionTerminated
	if onSessionTerminatedWrapper != nil {
		onSessionTerminated := func(cs ChainStore, client Client, reason SessionTerminatedReason) {}
		for i := len(onSessionTerminatedWrapper); i > 0; i-- {
			onSessionTerminated = onSessionTerminatedWrapper[i-1](onSessionTerminated)
		}
		srv.onSessionTerminated = onSessionTerminated
	}

	// onSubscribe
	if onSubscribeWrappers != nil {
		onSubscribe := func(cs ChainStore, client Client, topic packets.Topic) (qos uint8) {
			return topic.Qos
		}
		for i := len(onSubscribeWrappers); i > 0; i-- {
			onSubscribe = onSubscribeWrappers[i-1](onSubscribe)
		}
		srv.onSubscribe = onSubscribe
	}
	// onSubscribed
	if onSubscribedWrappers != nil {
		onSubscribed := func(cs ChainStore, client Client, topic packets.Topic) {}
		for i := len(onSubscribedWrappers); i > 0; i-- {
			onSubscribed = onSubscribedWrappers[i-1](onSubscribed)
		}
		srv.onSubscribed = onSubscribed
	}
	//onUnsubscribed
	if onUnsubscribedWrappers != nil {
		onUnsubscribed := func(cs ChainStore, client Client, topicName string) {}
		for i := len(onUnsubscribedWrappers); i > 0; i-- {
			onUnsubscribed = onUnsubscribedWrappers[i-1](onUnsubscribed)
		}
		srv.onUnsubscribed = onUnsubscribed
	}
	// onMsgArrived
	if onMsgArrivedWrappers != nil {
		onMsgArrived := func(cs ChainStore, client Client, msg Message) (valid bool) {
			return true
		}
		for i := len(onMsgArrivedWrappers); i > 0; i-- {
			onMsgArrived = onMsgArrivedWrappers[i-1](onMsgArrived)
		}
		srv.onMsgArrived = onMsgArrived
	}
	// onDeliver
	if onDeliverWrappers != nil {
		onDeliver := func(cs ChainStore, client Client, msg Message) {}
		for i := len(onDeliverWrappers); i > 0; i-- {
			onDeliver = onDeliverWrappers[i-1](onDeliver)
		}
		srv.onDeliver = onDeliver
	}
	// onAcked
	if onAckedWrappers != nil {
		onAcked := func(cs ChainStore, client Client, msg Message) {}
		for i := len(onAckedWrappers); i > 0; i-- {
			onAcked = onAckedWrappers[i-1](onAcked)
		}
		srv.onAcked = onAcked
	}
	// onClose hooks
	if onCloseWrappers != nil {
		onClose := func(cs ChainStore, client Client, err error) {}
		for i := len(onCloseWrappers); i > 0; i-- {
			onClose = onCloseWrappers[i-1](onClose)
		}
		srv.onClose = onClose
	}
	// onStop
	if onStopWrappers != nil {
		onStop := func(cs ChainStore) {}
		for i := len(onStopWrappers); i > 0; i-- {
			onStop = onStopWrappers[i-1](onStop)
		}
		srv.onStop = onStop
	}

	// onMsgDropped
	if onMsgDroppedWrappers != nil {
		onMsgDropped := func(cs ChainStore, client Client, msg Message) {}
		for i := len(onMsgDroppedWrappers); i > 0; i-- {
			onMsgDropped = onMsgDroppedWrappers[i-1](onMsgDropped)
		}
		srv.onMsgDropped = onMsgDropped
	}

	return nil
}

// Run starts the mqtt server. This method is non-blocking
func (srv *Server) Run() {
	err := srv.loadPlugins()
	if err != nil {
		panic(err)
	}

	srv.status = serverStatusStarted
	go srv.eventLoop()
	for _, ln := range srv.tcpListener {
		go srv.serveTCP(ln)
	}
	if len(srv.websocketServer) != 0 {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			c, err := defaultUpgrader.Upgrade(w, r, nil)
			if err != nil {
				log.Println("upgrade:", err)
				return
			}
			defer c.Close()
			conn := &wsConn{c.UnderlyingConn(), c}
			client := srv.newClient(conn)
			client.serve()
		})
	}
	for _, server := range srv.websocketServer {
		go srv.serveWebSocket(server)
	}
}

// Stop gracefully stops the mqtt server by the following steps:
//  1. Closing all open TCP listeners and shutting down all open websocket servers
//  2. Closing all idle connections
//  3. Waiting for all connections have been closed
//  4. Triggering OnStop()
func (srv *Server) Stop(ctx context.Context) error {
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
		return ctx.Err()
	case <-done:
		for _, v := range srv.plugins {
			fmt.Println("unload", v.Unload())
		}
		if srv.onStop != nil {
			srv.onStop(&chainStore{})
		}
		return nil
	}

}
