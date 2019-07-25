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
	DefaultDeliveryRetryInterval = 20 * time.Second
	DefaultQueueQos0Messages     = true
	DefaultMaxInflightMessages   = 20
	DefaultMaxQueueMessages      = 2048
	DefaultMsgRouterLen          = 4096
	DefaultRegisterLen           = 2048
	DefaultUnRegisterLen         = 2048
)

// Server status
const (
	serverStatusInit = iota
	serverStatusStarted
)

type subscriptionsDB interface {
	subscribe(clientID string, topic packets.Topic)
	unsubscribe(clientID string, topicName string)
	deleteAll(clientID string)
	// 判断消息路由的时候使用
	getMatchedTopicFilter(topicName string) map[string][]packets.Topic
	// 管理api使用
	getClientTopicFilter(clientID string) []packets.Topic
}

// Server represents a mqtt server instance.
// Create an instance of Server, by using NewServer()
type Server struct {
	mu              sync.RWMutex //gard clients map
	status          int32        //server status
	clients         map[string]*Client
	tcpListener     []net.Listener //tcp listeners
	websocketServer []*WsServer    //websocket server
	exitChan        chan struct{}
	retainedMsgMu   sync.Mutex
	retainedMsg     map[string]*packets.Publish //retained msg, key by topic name

	subscriptionsDB subscriptionsDB //store subscriptions

	msgRouter  chan *msgRouter
	register   chan *register   //register session
	unregister chan *unregister //unregister session

	config *Config
	//hooks
	onAccept       OnAccept
	onConnect      OnConnect
	onSubscribe    OnSubscribe
	onUnsubscribed OnUnsubscribed
	onPublish      OnPublish
	onDeliver      OnDeliver
	onAcked        OnAcked
	onClose        OnClose
	onStop         OnStop
	//Monitor
	Monitor *Monitor
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

// RegisterOnSubscribe registers a onSubscribe callback.
func (srv *Server) RegisterOnSubscribe(callback OnSubscribe) {
	srv.checkStatus()
	srv.onSubscribe = callback
}

// RegisterOnUnsubscribed registers a onUnsubscribed callback.
func (srv *Server) RegisterOnUnsubscribed(callback OnUnsubscribed) {
	srv.checkStatus()
	srv.onUnsubscribed = callback
}

// RegisterOnPublish registers a onPublish callback.
func (srv *Server) RegisterOnPublish(callback OnPublish) {
	srv.checkStatus()
	srv.onPublish = callback
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
	DeliveryRetryInterval time.Duration
	QueueQos0Messages     bool
	MaxInflightMessages   int
	MaxQueueMessages      int
	DeliverMode           DeliverMode
}

//session register
type register struct {
	client  *Client
	connect *packets.Connect
	error   error
}

// session unregister
type unregister struct {
	client *Client
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
		code := srv.onConnect(client)
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
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	var oldSession *session
	oldClient, oldExist := srv.clients[client.opts.ClientID]
	srv.clients[client.opts.ClientID] = client
	if oldExist {
		oldSession = oldClient.session
		if oldClient.Status() == Connected {
			if log != nil {
				log.Printf("%-15s %v: logging with duplicate ClientID: %s", "", client.rwc.RemoteAddr(), client.ClientOptions().ClientID)
			}
			oldClient.setSwitching()
			<-oldClient.Close()
			if oldClient.opts.WillFlag {
				willMsg := &packets.Publish{
					Dup:       false,
					Qos:       oldClient.opts.WillQos,
					Retain:    oldClient.opts.WillRetain,
					TopicName: []byte(oldClient.opts.WillTopic),
					Payload:   oldClient.opts.WillPayload,
				}
				go func() {
					msgRouter := &msgRouter{pub: willMsg}
					srv.msgRouter <- msgRouter
				}()
			}
			if !client.opts.CleanSession && !oldClient.opts.CleanSession { //reuse old session
				sessionReuse = true
			clearOut:
				for {
					select {
					case p := <-oldClient.out:
						if p, ok := p.(*packets.Publish); ok {
							oldClient.msgEnQueue(p)
						}
					default:
						break clearOut
					}
				}
			}
		} else if oldClient.Status() == Disconnected {
			if !client.opts.CleanSession {
				sessionReuse = true
			}
		}
	}
	ack := connect.NewConnackPacket(sessionReuse)
	client.out <- ack
	client.setConnected()
	if sessionReuse { //发送还未确认的消息和离线消息队列 inflight & msgQueue
		client.session.unackpublish = oldSession.unackpublish
		oldSession.inflightMu.Lock()
		for e := oldSession.inflight.Front(); e != nil; e = e.Next() { //wriute nacknowledged publish & pubrel
			if inflight, ok := e.Value.(*InflightElem); ok {
				pub := inflight.Packet
				pub.Dup = true
				if inflight.Step == 0 {
					client.publish(pub)
				}
				if inflight.Step == 1 { //pubrel
					pubrel := pub.NewPubrec().NewPubrel()
					client.session.inflight.PushBack(inflight)
					client.session.setPacketID(pub.PacketID)
					client.out <- pubrel
				}
			}
		}
		oldSession.inflightMu.Unlock()
		oldSession.msgQueueMu.Lock()
		for e := oldSession.msgQueue.Front(); e != nil; e = e.Next() { //write offline msg
			if publish, ok := e.Value.(*packets.Publish); ok {
				client.publish(publish)
			}
		}
		oldSession.msgQueueMu.Unlock()
		if log != nil {
			log.Printf("%-15s %v: logined with session reuse", "", client.rwc.RemoteAddr())
		}
	} else {
		if oldExist {
			//srv.subscriptionsDB.Lock()
			srv.subscriptionsDB.deleteAll(client.opts.ClientID)
			//srv.subscriptionsDB.Unlock()
		}
		if log != nil {
			log.Printf("%-15s %v: logined with new session", "", client.rwc.RemoteAddr())
		}
	}
	if srv.Monitor != nil {
		srv.Monitor.register(client, sessionReuse)
	}
}

func (srv *Server) unregisterHandler(unregister *unregister) {
	defer close(unregister.done)
	client := unregister.client
	client.setDisConnected()
	if client.session == nil {
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

	if !client.cleanWillFlag && client.opts.WillFlag {
		willMsg := &packets.Publish{
			Dup:       false,
			Qos:       client.opts.WillQos,
			Retain:    false,
			TopicName: []byte(client.opts.WillTopic),
			Payload:   client.opts.WillPayload,
		}
		go func() {
			msgRouter := &msgRouter{pub: willMsg}
			client.server.msgRouter <- msgRouter
		}()
	}
	if client.opts.CleanSession {
		if log != nil {
			log.Printf("%-15s %v: logout & cleaning session", "", client.rwc.RemoteAddr())
		}
		srv.mu.Lock()
		delete(srv.clients, client.opts.ClientID)
		//	srv.subscriptionsDB.Lock()
		srv.subscriptionsDB.deleteAll(client.opts.ClientID)
		//	srv.subscriptionsDB.Unlock()
		srv.mu.Unlock()
	} else { //store session 保持session
		if log != nil {
			log.Printf("%-15s %v: logout & storing session", "", client.rwc.RemoteAddr())
		}
		//clear  out
	clearOut:
		for {
			select {
			case p := <-client.out:
				if p, ok := p.(*packets.Publish); ok {
					client.publish(p)
				}
			default:
				break clearOut
			}
		}
	}
	if srv.Monitor != nil {
		srv.Monitor.unRegister(client.opts.ClientID, client.opts.CleanSession)
	}
}

func (srv *Server) msgRouterHandler(msg *msgRouter) {
	srv.mu.RLock()
	defer srv.mu.RUnlock()
	//	srv.subscriptionsDB.RLock()
	//	defer srv.subscriptionsDB.RUnlock()
	pub := msg.pub
	rs := srv.subscriptionsDB.getMatchedTopicFilter(string(pub.TopicName))
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

// server event loop
func (srv *Server) eventLoop() {
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

// WsServer is used to build websocket server
type WsServer struct {
	Server   *http.Server
	CertFile string //TLS configration
	KeyFile  string //TLS configration
}

// OnAccept 会在新连接建立的时候调用，只在TCP server中有效。如果返回false，则会直接关闭连接
//
// OnAccept will be called after a new connection established in TCP server. If returns false, the connection will be close directly.
type OnAccept func(conn net.Conn) bool

// OnStop will be called on server.Stop()
type OnStop func()

/*
OnSubscribe 返回topic允许订阅的最高QoS等级

OnSubscribe returns the maximum available QoS for the topic:
 0x00 - Success - Maximum QoS 0
 0x01 - Success - Maximum QoS 1
 0x02 - Success - Maximum QoS 2
 0x80 - Failure
*/
type OnSubscribe func(client *Client, topic packets.Topic) uint8

// OnUnsubscribed will be called after the topic has been unsubscribed
type OnUnsubscribed func(client *Client, topicName string)

// OnPublish 返回接收到的publish报文是否允许转发，返回false则该报文不会被继续转发
//
// OnPublish returns whether the publish packet will be delivered or not.
// If returns false, the packet will not be delivered to any clients.
type OnPublish func(client *Client, publish *packets.Publish) bool

// OnClose tcp连接关闭之后触发
//
// OnClose will be called after the tcp connection of the client has been closed
type OnClose func(client *Client, err error)

// OnConnect 当合法的connect报文到达的时候触发，返回connack中响应码
//
// OnConnect will be called when a valid connect packet is received.
// It returns the code of the connack packet
type OnConnect func(client *Client) (code uint8)

// OnDeliver
type OnDeliver func(client *Client, publish *packets.Publish)

// OnAcked
type OnAcked func(client *Client, publish *packets.Publish)

// NewServer returns a default gmqtt server instance
func NewServer() *Server {
	return &Server{
		status:          serverStatusInit,
		exitChan:        make(chan struct{}),
		clients:         make(map[string]*Client),
		msgRouter:       make(chan *msgRouter, DefaultMsgRouterLen),
		register:        make(chan *register, DefaultRegisterLen),
		unregister:      make(chan *unregister, DefaultUnRegisterLen),
		retainedMsg:     make(map[string]*packets.Publish),
		subscriptionsDB: newTrieDB(),
		config: &Config{
			DeliveryRetryInterval: DefaultDeliveryRetryInterval,
			QueueQos0Messages:     DefaultQueueQos0Messages,
			MaxInflightMessages:   DefaultMaxInflightMessages,
			MaxQueueMessages:      DefaultMaxQueueMessages,
			DeliverMode:           Overlap,
		},
		Monitor: &Monitor{
			Repository: &MonitorStore{
				clients:       make(map[string]ClientInfo),
				sessions:      make(map[string]SessionInfo),
				subscriptions: make(map[string]map[string]SubscriptionsInfo),
			},
		},
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

// SetDeliveryRetryInterval sets the delivery retry interval.
func (srv *Server) SetDeliveryRetryInterval(duration time.Duration) {
	srv.checkStatus()
	srv.config.DeliveryRetryInterval = duration
}

// SetMaxQueueMessages sets the maximum queue messages.
func (srv *Server) SetMaxQueueMessages(nums int) {
	srv.checkStatus()
	srv.config.MaxQueueMessages = nums
}

// SetQueueQos0Messages sets whether to queue QoS 0 messages. Default to true.
func (srv *Server) SetQueueQos0Messages(b bool) {
	srv.checkStatus()
	srv.config.QueueQos0Messages = b
}

// SetMaxInflightMessages sets the maximum inflight messages.
func (srv *Server) SetMaxInflightMessages(i int) {
	srv.checkStatus()
	if i > maxInflightMessages {
		srv.config.MaxInflightMessages = maxInflightMessages
		return
	}
	srv.config.MaxInflightMessages = i
}

// Publish 主动发布一个主题
//
// Publish publishs a message to the broker.
// 	Notice: This method will not trigger the onPublish callback
func (srv *Server) Publish(publish *packets.Publish) {
	srv.msgRouter <- &msgRouter{publish}
}

// Subscribe 为某一个客户端订阅主题
//
// Subscribe subscribes topics for the client specified by clientID.
// 	Notice: This method will not trigger the onSubscribe callback
func (srv *Server) Subscribe(clientID string, topics []packets.Topic) {
	/*	client := srv.Client(clientID)
		if client == nil {
			return
		}*/
	//	srv.subscriptionsDB.Lock()
	//	defer srv.subscriptionsDB.Unlock()
	for _, v := range topics {
		srv.subscriptionsDB.subscribe(clientID, v)
		if srv.Monitor != nil {
			srv.Monitor.subscribe(SubscriptionsInfo{
				ClientID: clientID,
				Qos:      v.Qos,
				Name:     string(v.Name),
				At:       time.Now(),
			})
		}
	}
}

// UnSubscribe 为某一个客户端取消订阅某个主题
//
// UnSubscribe unsubscribes topics for the client specified by clientID.
func (srv *Server) UnSubscribe(clientID string, topics []string) {
	client := srv.Client(clientID)
	if client == nil {
		return
	}
	//	srv.subscriptionsDB.Lock()
	//	defer srv.subscriptionsDB.Unlock()
	for _, v := range topics {
		srv.subscriptionsDB.unsubscribe(clientID, v)
		if srv.Monitor != nil {
			srv.Monitor.unSubscribe(clientID, v)
		}
	}
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
		if srv.onAccept != nil {
			if !srv.onAccept(rw) {
				rw.Close()
				continue
			}
		}
		client := srv.newClient(rw)
		go client.serve()
	}
}

// Client returns all the connected clients
func (srv *Server) Client(clientID string) *Client {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	return srv.clients[clientID]
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

func (srv *Server) newClient(c net.Conn) *Client {
	client := &Client{
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
	}
	client.packetReader = packets.NewReader(client.bufr)
	client.packetWriter = packets.NewWriter(client.bufw)
	client.setConnecting()
	client.newSession()
	return client
}

// Run starts the mqtt server. This method is non-blocking
func (srv *Server) Run() {
	if srv.Monitor != nil {
		srv.Monitor.Repository.Open()
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
		if srv.Monitor != nil {
			srv.Monitor.Repository.Close()
		}
		if srv.onStop != nil {
			srv.onStop()
		}
		return nil
	}

}
