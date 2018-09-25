package server

import (
	"context"
	"errors"
	"github.com/DrmagicE/gmqtt/logger"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/gorilla/websocket"
	"net"
	"net/http"
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

type Server struct {
	sync.WaitGroup
	connectMu       sync.Mutex
	mu              sync.RWMutex //gard session map

	sessions        map[string]*session
	tcpListener     []net.Listener //tcp listeners
	websocketServer []*WsServer    //websocket server
	exitChan        chan struct{}
	retainedMsgMu   sync.Mutex
	retainedMsg     map[string]*packets.Publish

	incoming chan *packets.Publish

	config *config

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
		sessions:    make(map[string]*session),
		incoming:    make(chan *packets.Publish, 8192),
		retainedMsg: make(map[string]*packets.Publish),
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
			srv.mu.RLock()
			for _, s := range srv.sessions {
				s.deliver(packet, false)
			}
			srv.mu.RUnlock()
		}
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

func (srv *Server) Session(clientId string) *session {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	return srv.sessions[clientId]
}

func (srv *Server) SetSession(s *session) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	srv.sessions[s.client.opts.ClientId] = s
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
	}
	client.packetReader = packets.NewReader(client.bufr)
	client.packetWriter = packets.NewWriter(client.bufw)
	client.setConnecting()
	return client
}



func (srv *Server) Run() {
	go srv.routing()
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
	closeCompleteSet := make([]<-chan struct{}, len(srv.sessions))
	i := 0
	for _, v := range srv.sessions {
		closeCompleteSet[i] = v.client.Close()
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
