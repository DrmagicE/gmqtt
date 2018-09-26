package server

import (
	"context"
	"errors"
	"github.com/DrmagicE/gmqtt/logger"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/gorilla/websocket"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

var (
	ErrInvalWsMsgType = errors.New("invalid websocket message type") // [MQTT-6.0.0-1]
)

//Default configration
const (
	DefaultDeliveryRetryInterval = 20 * time.Second
	DefaultQueueQos0Messages     = true
	DefaultMaxInflightMessages   = 20
)

var log = &logger.Logger{}

func SetLogger(l *logger.Logger) {
	log = l
}

type config struct {
	deliveryRetryInterval time.Duration
	queueQos0Messages     bool
	maxInflightMessages   int
}

//for session login
type clientConnect struct {
	client  *Client
	connect *packets.Connect
	error error
}

type Server struct {
	sync.WaitGroup
	connectMu       sync.Mutex
	mu              sync.RWMutex //gard clients map
	clients         map[string]*Client
	connect         chan *clientConnect //to build session
	tcpListener     []net.Listener      //tcp listeners
	websocketServer []*WsServer         //websocket server
	exitChan        chan struct{}
	retainedMsgMu   sync.Mutex
	retainedMsg     map[string]*packets.Publish //retained msg, key by topic name
	incoming        chan *packets.Publish       //packet to be distributed
	config          *config
	//hooks
	OnAccept    OnAccept
	OnConnect   OnConnect
	OnSubscribe OnSubscribe
	OnPublish   OnPublish
	OnClose     OnClose
	OnStop      OnStop
}

type WsServer struct {
	Server   *http.Server
	CertFile string
	KeyFile  string
}

type OnAccept func(conn net.Conn) bool

type OnStop func()

//返回qos等级，或者是不允许订阅
//Allowed return codes:
//0x00 - Success - Maximum QoS 0
//0x01 - Success - Maximum QoS 1
//0x02 - Success - Maximum QoS 2
//0x80 - Failure
type OnSubscribe func(client *Client, topic packets.Topic) uint8

//返回qos等级，或者是不允许订阅
//Whether the publish packet will be delivered or not.
type OnPublish func(client *Client, publish *packets.Publish) bool

//tcp连接关闭之后触发
//called after tcp connection closed
type OnClose func(client *Client)

//返回connack中响应码
//return the code of connack packet
type OnConnect func(client *Client) (code uint8)

func NewServer() *Server {
	return &Server{
		exitChan:    make(chan struct{}),
		clients:     make(map[string]*Client),
		incoming:    make(chan *packets.Publish, 8192),
		retainedMsg: make(map[string]*packets.Publish),
		connect:     make(chan *clientConnect),
		config: &config{
			deliveryRetryInterval: DefaultDeliveryRetryInterval,
			queueQos0Messages:     DefaultQueueQos0Messages,
			maxInflightMessages:   DefaultMaxInflightMessages,
		},
	}
}

func (srv *Server) SetDeliveryRetryInterval(duration time.Duration) {
	srv.config.deliveryRetryInterval = duration
}

func (srv *Server) SetQueueQos0Messages(b bool) {
	srv.config.queueQos0Messages = b
}

func (srv *Server) SetMaxInflightMessages(i int) {
	srv.config.maxInflightMessages = i
}

func (srv *Server) AddTCPListenner(ln ...net.Listener) {
	for _, v := range ln {
		srv.tcpListener = append(srv.tcpListener, v)
	}
}

func (srv *Server) AddWebSocketServer(Server ...*WsServer) {
	for _, v := range Server {
		srv.websocketServer = append(srv.websocketServer, v)
	}
}

func (srv *Server) routing() {
	for {
		select {
		case <-srv.exitChan:
			return
		case packet := <-srv.incoming:
			srv.mu.RLock() //阻塞在这里
			for _, c := range srv.clients {
				c.deliver(packet, false)
			}
			srv.mu.RUnlock()
		}
	}
}

//分发publish报文
func (client *Client) deliver(incoming *packets.Publish, isRetain bool) {
	s := client.session
	var matchTopic packets.Topic
	var isMatch bool
	once := sync.Once{}
	s.topicsMu.Lock()
	for _, topic := range s.subTopics {
		if packets.TopicMatch(incoming.TopicName, []byte(topic.Name)) {
			once.Do(func() {
				matchTopic = topic
				isMatch = true
			})
			if topic.Qos > matchTopic.Qos { //[MQTT-3.3.5-1]
				matchTopic = topic
			}
		}
	}
	s.topicsMu.Unlock()
	if isMatch { //匹配
		publish := incoming.CopyPublish()
		if publish.Qos > matchTopic.Qos {
			publish.Qos = matchTopic.Qos
		}
		if publish.Qos > 0 {
			publish.PacketId = s.getPacketId()
		}
		publish.Dup = false
		publish.Retain = isRetain
		client.write(publish)
	}
}

func (srv *Server) serveTcp(l net.Listener) {
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
		if srv.OnAccept != nil {
			if !srv.OnAccept(rw) {
				continue
			}
		}
		client := srv.newClient(rw)
		go client.serve()
	}
}

func (srv *Server) Clients(clientId string) *Client {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	return srv.clients[clientId]
}

func (srv *Server) AddClients(client *Client) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	srv.clients[client.opts.ClientId] = client
}

var defaultUpgrader = &websocket.Upgrader{
	ReadBufferSize:  READ_BUFFER_SIZE,
	WriteBufferSize: WRITE_BUFFER_SIZE,
}

//实现io.ReadWriter接口
//implement the io.ReadWriter
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
		bufr:          newBufioReaderSize(c, READ_BUFFER_SIZE),
		bufw:          newBufioWriterSize(c, WRITE_BUFFER_SIZE),
		close:         make(chan struct{}),
		closeComplete: make(chan struct{}),
		error:         make(chan error, 1),
		in:            make(chan packets.Packet, READ_BUFFER_SIZE),
		out:           make(chan packets.Packet, READ_BUFFER_SIZE),
		status:        CONNECTING,
		opts:          &ClientOptions{},
		cleanWillFlag: false,
		ready:         make(chan struct{}),
	}
	client.packetReader = packets.NewReader(client.bufr)
	client.packetWriter = packets.NewWriter(client.bufw)
	client.setConnecting()
	return client
}

func (srv *Server) startSession() {
	for {
		select {
		case <-srv.exitChan:
			return
		case cc := <-srv.connect:
			client := cc.client
			connect := cc.connect
			var sessionReuse bool

			if connect.AckCode != packets.CODE_ACCEPTED {
				cc.error = errors.New("reject connection, ack code:" + strconv.Itoa(int(connect.AckCode)))
				if cc.error != nil {
					ack := connect.NewConnackPacket(false)
					client.out <- ack
					continue
				}
			}
			server := client.server
			if server.OnConnect != nil {
				code := server.OnConnect(client)
				connect.AckCode = code
				if code != packets.CODE_ACCEPTED {
					cc.error = errors.New("reject connection, ack code:" + strconv.Itoa(int(code)))
					if cc.error != nil {
						ack := connect.NewConnackPacket(false)
						client.out <- ack
						continue
					}
				}
			}
			clientId := client.opts.ClientId
			oldClient := server.Clients(clientId)
			var oldSession *session
			if oldClient != nil {
				oldSession = oldClient.session
				if oldClient.Status() == CONNECTED {
					if log != nil {
						log.Printf("%-15s %v: logging with duplicate ClientId: %s", "", client.rwc.RemoteAddr(), client.ClientOption().ClientId)
					}
					if client.opts.CleanSession == true {
						oldSession.Lock()
						oldSession.needStore = false
						oldSession.Unlock()
					}
					<-oldClient.Close()
				}
				if client.opts.CleanSession == false && oldClient.opts.CleanSession == false {
					//reuse session
					sessionReuse = true
				}
			}
			if sessionReuse {
				client.reuseSession(oldSession)
			} else {
				client.newSession()
			}
			server.AddClients(client)
			ack := connect.NewConnackPacket(sessionReuse)
			client.out <- ack
			client.setConnected()
			if sessionReuse {
				//发送还未确认的消息和离线消息队列
				go func() {
					client.session.inflightMu.Lock()
					//write unacknowledged publish & pubrel
					for e := client.session.inflight.Front(); e != nil; e = e.Next() {
						if inflight, ok := e.Value.(*inflightElem); ok {
							switch inflight.packet.(type) {
							case *packets.Publish:
								publish := inflight.packet.(*packets.Publish)
								publish.Dup = true
								client.out <- publish

							case *packets.Pubrel:
								pubrel := inflight.packet.(*packets.Pubrel)
								pubrel.Dup = true
								client.out <- pubrel
							}
						}
					}
					client.session.inflight.Init()
					client.session.inflightMu.Unlock()
					//offline msg
					client.session.offlineQueueMu.Lock()
					for {
						if client.session.offlineQueue.Front() == nil {
							break
						}
						client.out <- client.session.offlineQueue.Remove(client.session.offlineQueue.Front()).(packets.Packet)
					}
					client.session.offlineQueueMu.Unlock()
					if log != nil {
						log.Printf("%-15s %v: logined with session reuse", "", client.rwc.RemoteAddr())
					}
				}()
			} else {
				if log != nil {
					log.Printf("%-15s %v: logined with new session", "", client.rwc.RemoteAddr())
				}
			}
			close(client.ready)
		}

	}
}

func (srv *Server) Run() {
	go srv.routing()
	go srv.startSession()
	for _, ln := range srv.tcpListener {
		go srv.serveTcp(ln)
	}
	if len(srv.websocketServer) != 0 {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			defaultUpgrader.CheckOrigin = func(r *http.Request) bool {
				return true
			}
			defaultUpgrader.Subprotocols = []string{"mqtt"}
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

func (srv *Server) Stop(ctx context.Context) error {
	srv.connectMu.Lock()
	defer srv.connectMu.Unlock()
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
	//closing all client

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
			//wait for all sessions to logout
			<-v
		}
		close(done)
	}()
	<-done
	if srv.OnStop != nil {
		srv.OnStop()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}

}
