package server

import (
	"bufio"
	"container/list"
	"errors"
	"fmt"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrInvalStatus    = errors.New("invalid connection status")
	ErrWriteBufFull   = errors.New("write chan is full")
	ErrConnectTimeOut = errors.New("connect time out")
)

const (
	CONNECTING = iota
	CONNECTED
	DISCONNECTED
)
const READ_BUFFER_SIZE = 4096
const WRITE_BUFFER_SIZE = 4096

const REDELIVER_TIME = 20 //second

var (
	bufioReaderPool sync.Pool
	bufioWriterPool sync.Pool
)

func newBufioReaderSize(r io.Reader, size int) *bufio.Reader {
	if v := bufioReaderPool.Get(); v != nil {
		br := v.(*bufio.Reader)
		br.Reset(r)
		return br
	}
	return bufio.NewReaderSize(r, size)
}

func putBufioReader(br *bufio.Reader) {
	br.Reset(nil)
	bufioReaderPool.Put(br)
}

func newBufioWriterSize(w io.Writer, size int) *bufio.Writer {
	if v := bufioWriterPool.Get(); v != nil {
		bw := v.(*bufio.Writer)
		bw.Reset(w)
		return bw
	}
	return bufio.NewWriterSize(w, size)
}

func putBufioWriter(bw *bufio.Writer) {
	bw.Reset(nil)
	bufioWriterPool.Put(bw)
}

type Client struct {
	mu            sync.Mutex
	server        *Server
	wg            sync.WaitGroup
	rwc           net.Conn //raw tcp connection
	bufr          *bufio.Reader
	bufw          *bufio.Writer
	packetReader  *packets.Reader
	packetWriter  *packets.Writer
	in            chan packets.Packet
	out           chan packets.Packet
	close         chan struct{} //关闭chan
	closeComplete chan struct{} //连接关闭
	status        int32         //client状态
	session       *session
	error         chan error     //错误
	opts          *ClientOptions //OnConnect之前填充,set up before OnConnect()
	cleanWillFlag bool           //收到DISCONNECT报文删除遗嘱标志, whether to remove will msg
	//自定义数据 user data
	userMutex sync.Mutex
	userData  interface{}

	ready chan struct{} //session prepared
}

func (c *Client) UserData() interface{} {
	c.userMutex.Lock()
	defer c.userMutex.Unlock()
	return c.userData
}

func (c *Client) SetUserData(data interface{}) {
	c.userMutex.Lock()
	defer c.userMutex.Unlock()
	c.userData = data
}

//readOnly
func (c *Client) ClientOption() ClientOptions {
	opts := *c.opts
	opts.WillPayload = make([]byte, len(c.opts.WillPayload))
	copy(opts.WillPayload, c.opts.WillPayload)
	return opts
}

func (c *Client) setConnecting() {
	atomic.StoreInt32(&c.status, CONNECTING)
}
func (c *Client) setConnected() {
	atomic.StoreInt32(&c.status, CONNECTED)
}
func (c *Client) setDisConnected() {
	atomic.StoreInt32(&c.status, DISCONNECTED)
}
func (c *Client) Status() int32 {
	return atomic.LoadInt32(&c.status)
}

type ClientOptions struct {
	ClientId     string
	Username     string
	Password     string
	KeepAlive    uint16
	CleanSession bool
	WillFlag     bool
	WillRetain   bool
	WillQos      uint8
	WillTopic    string
	WillPayload  []byte
}

func (client *Client) setError(error error) {
	select {
	case client.error <- error:
	default:
	}
}

func (client *Client) writeLoop() {
	var err error
	defer func() {
		if re := recover(); re != nil {
			err = errors.New(fmt.Sprint(re))
		}
		client.setError(err)
		client.wg.Done()
	}()
	for {
		select {
		case <-client.close: //关闭
			return
		case packet := <-client.out:
			switch packet.(type) {
			case *packets.Publish: //发布publish
				pub := packet.(*packets.Publish)
				if pub.Qos >= packets.QOS_1 && pub.Dup == false {
					inflightElem := &inflightElem{
						at:     time.Now(),
						pid:    pub.PacketId,
						packet: pub,
					}
					client.setInflight(inflightElem)
				}
			case *packets.Pubrel:
				pub := packet.(*packets.Pubrel)
				if pub.Dup == false {
					inflightElem := &inflightElem{
						at:     time.Now(),
						pid:    pub.PacketId,
						packet: pub,
					}
					client.setInflight(inflightElem)
				}
			}
			if log != nil {
				log.Printf("%-15s %v: %s ", "sending to", client.rwc.RemoteAddr(), packet)
			}
			err = client.writePacket(packet)
			if err != nil {
				return
			}
		}

	}
}

func (client *Client) writePacket(packet packets.Packet) error {
	return client.packetWriter.WritePacket(packet)
}

func (client *Client) readLoop() {
	var err error
	defer func() {
		if re := recover(); re != nil {
			err = errors.New(fmt.Sprint(re))
		}
		client.setError(err)
		client.wg.Done()
	}()
	for {
		var packet packets.Packet
		if client.Status() == CONNECTED {
			if keepAlive := client.opts.KeepAlive; keepAlive != 0 { //KeepAlive
				client.rwc.SetReadDeadline(time.Now().Add(time.Duration(keepAlive/2+keepAlive) * time.Second))
			}
		}
		packet, err = client.packetReader.ReadPacket()
		if err != nil {
			return
		}
		if log != nil {
			log.Printf("%-15s %v: %s ", "received from", client.rwc.RemoteAddr(), packet)
		}

		client.in <- packet
	}
}

func (client *Client) errorWatch() {
	defer func() {
		client.wg.Done()
	}()
	select {
	case <-client.close:
		return
	case <-client.error: //有错误关闭
		client.rwc.Close()
		close(client.close) //退出chanel
		return
	}
}

//关闭连接，连接关闭完毕会close(client.closeComplete)
//close client, close(client.closeComplete) when close completely
func (client *Client) Close() <-chan struct{} {
	client.setError(nil)
	return client.closeComplete
}

func (client *Client) connectWithTimeOut() (ok bool) {
	var err error
	defer func() {
		if err != nil {
			client.setError(err)
			ok = false
		} else {
			ok = true
		}
	}()
	var p packets.Packet
	select {
	case <-client.close:
		return
	case p = <-client.in: //first packet
	case <-time.After(5 * time.Second):
		err = ErrConnectTimeOut
		return
	}
	conn, flag := p.(*packets.Connect)
	if !flag {
		err = ErrInvalStatus
		return
	}
	client.opts.ClientId = string(conn.ClientId)
	client.opts.KeepAlive = conn.KeepAlive
	client.opts.CleanSession = conn.CleanSession
	client.opts.Username = string(conn.Username)
	client.opts.Password = string(conn.Password)
	client.opts.WillFlag = conn.WillFlag
	client.opts.WillPayload = make([]byte, len(conn.WillMsg))
	client.opts.WillQos = conn.WillQos
	client.opts.WillTopic = string(conn.WillTopic)
	copy(client.opts.WillPayload, conn.WillMsg)
	client.opts.WillRetain = conn.WillRetain
	if keepAlive := client.opts.KeepAlive; keepAlive != 0 { //KeepAlive
		client.rwc.SetReadDeadline(time.Now().Add(time.Duration(keepAlive/2+keepAlive) * time.Second))
	}
	cc := &clientConnect{
		client:  client,
		connect: conn,
	}

	select {
	case client.server.connect <- cc:
	case <-client.close:
		return
	}
	select {
	case <-client.close:
		return
	case <-client.ready:
	}
	err =  cc.error
	return

}

/*func (client *Client) sessionLogin(connect *packets.Connect) (err error) {
	client.server.connectMu.Lock()
	defer client.server.connectMu.Unlock()
	var sessionReuse bool
	defer func() {
		if err != nil {
			ack := connect.NewConnackPacket(false)
			client.out <- ack
			return
		}
		ack := connect.NewConnackPacket(sessionReuse)
		client.out <- ack
		client.setConnected()
		if sessionReuse {
			//离线队列
			go func() {

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

				for {
					if client.session.offlineQueue.Front() == nil {
						break
					}
					client.out <- client.session.offlineQueue.Remove(client.session.offlineQueue.Front()).(packets.Packet)
				}
				close(client.session.ready)
				if log != nil {
					log.Printf("%-15s %v: logined with session reuse", "", client.rwc.RemoteAddr())
				}
			}()
		} else {
			if log != nil {
				log.Printf("%-15s %v: logined with new session", "", client.rwc.RemoteAddr())
			}
			close(client.session.ready)
		}
	}()

	if connect.AckCode != packets.CODE_ACCEPTED {
		err = errors.New("reject connection, ack code:" + strconv.Itoa(int(connect.AckCode)))
		return
	}
	server := client.server
	if server.OnConnect != nil {
		code := server.OnConnect(client)
		connect.AckCode = code
		if code != packets.CODE_ACCEPTED {
			err = errors.New("reject connection, ack code:" + strconv.Itoa(int(code)))
			return
		}
	}
	clientId := client.opts.ClientId
	oldSession := server.Session(clientId)
	if oldSession != nil {
		if log != nil {
			log.Printf("%-15s %v: logging with duplicate ClientId: %s", "", client.rwc.RemoteAddr(), client.ClientOption().ClientId)
		}
		if client.opts.CleanSession == true {
			oldSession.needStore = false
		}
		<-oldSession.client.Close() //wait for old session to logout

		if client.opts.CleanSession == false && oldSession.client.opts.CleanSession == false {
			//reuse old session
			client.session = oldSession
			oldSession.SetClient(client)
			//oldSession.client = client
			sessionReuse = true
			return
		} else {
			//new session
			client.session = newSession(client)
			server.SetSession(client.session)
			return
		}
	} else {
		// new session
		client.session = newSession(client)
		server.SetSession(client.session)
		return
	}
}*/

func (client *Client) reuseSession(oldSession *session) {
	client.session = oldSession
	oldSession.needStore = true
}

func (client *Client) newSession() {
	s := &session{
		subTopics:     make(map[string]packets.Topic),
		unackpublish:  make(map[packets.PacketId]bool),
		inflight:      list.New(),
		inflightToken: make(chan struct{}),
		pid:           make(map[packets.PacketId]bool),
		offlineQueue:  list.New(),
		needStore:     !client.opts.CleanSession,
	}
	client.session = s
}

//session logout,called after tcp conn  is closed
func (client *Client) sessionLogout() {
	if client.session == nil {
		return
	}
	<-client.ready
	s := client.session
	server := client.server
	server.mu.Lock() //在这里阻塞了
	defer server.mu.Unlock()
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
	if !client.cleanWillFlag && client.opts.WillFlag {
		willMsg := &packets.Publish{
			Dup:       false,
			Qos:       client.opts.WillQos,
			Retain:    client.opts.WillRetain,
			TopicName: []byte(client.opts.WillTopic),
			Payload:   client.opts.WillPayload,
		}
		go func() {
			client.server.incoming <- willMsg
		}()
	}

	client.session.Lock()
	needStore := client.session.needStore
	client.session.Unlock()
	if needStore == false {
		if log != nil {
			log.Printf("%-15s %v: logout & cleaning session", "", client.rwc.RemoteAddr())
		}
		delete(server.clients, client.opts.ClientId)
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
					client.write(p)
				}
			default:
				break clearOut
			}
		}
		s.offlineAt = time.Now()
	}
}

func (client *Client) internalClose() {
	client.sessionLogout()
	putBufioReader(client.bufr)
	putBufioWriter(client.bufw)
	if client.server.OnClose != nil {
		client.server.OnClose(client)
	}
	close(client.closeComplete)
}

func (client *Client) write(packet packets.Packet) {
	if client.Status() == CONNECTED { //在线消息
		<-client.ready
		client.out <- packet
	} else { //离线消息
		if pub, ok := packet.(*packets.Publish); ok {
			if pub.Qos == packets.QOS_0 && client.server.config.queueQos0Messages == false {
				return
			}
		}
		log.Printf("%-15s[%s] %s ", "queueing offline msg cid", client.ClientOption().ClientId, packet)
		client.session.offlineQueueMu.Lock()
		client.session.offlineQueue.PushBack(packet)
		client.session.offlineQueueMu.Unlock()
	}
}

//处理读到的包
//goroutine 退出条件，1.session逻辑错误,2链接关闭
func (client *Client) readHandle() {
	var err error
	s := client.session
	defer func() {
		if re := recover(); re != nil {
			err = errors.New(fmt.Sprint(re))
		}
		client.setError(err)
		client.wg.Done()
	}()

	for {
		select {
		case <-client.close:
			return
		case packet := <-client.in:
			switch packet.(type) {
			case *packets.Subscribe:
				sub := packet.(*packets.Subscribe)
				if client.server.OnSubscribe != nil {
					for k, v := range sub.Topics {
						sub.Topics[k].Qos = client.server.OnSubscribe(client, v)
					}
				}
				suback := sub.NewSubBack()
				client.write(suback)
				s.topicsMu.Lock()
				var isNew bool
				for k, v := range sub.Topics {
					if v.Qos != packets.SUBSCRIBE_FAILURE {
						topic := packets.Topic{
							Name: v.Name,
							Qos:  suback.Payload[k],
						}
						if _, ok := s.subTopics[string(v.Name)]; !ok {
							isNew = true
						}
						s.subTopics[string(v.Name)] = topic
					}
				}
				s.topicsMu.Unlock()
				if isNew {
					client.server.retainedMsgMu.Lock()
					for _, msg := range client.server.retainedMsg {
						client.deliver(msg, true) //retain msg
					}
					client.server.retainedMsgMu.Unlock()
				}
			case *packets.Publish:
				var dup bool
				pub := packet.(*packets.Publish)
				if pub.Qos == packets.QOS_1 {
					puback := pub.NewPuback()
					client.write(puback)
				}
				if pub.Qos == packets.QOS_2 {
					pubrec := pub.NewPubrec()
					client.write(pubrec)
					if _, ok := s.unackpublish[pub.PacketId]; ok {
						dup = true
					} else {
						s.unackpublish[pub.PacketId] = true
					}
				}
				if pub.Retain {
					//保留消息，处理保留
					client.server.retainedMsgMu.Lock()
					client.server.retainedMsg[string(pub.TopicName)] = pub
					if len(pub.Payload) == 0 {
						delete(client.server.retainedMsg, string(pub.TopicName))
					}
					client.server.retainedMsgMu.Unlock()
				}
				if !dup {
					var valid bool
					valid = true
					if client.server.OnPublish != nil {
						valid = client.server.OnPublish(client, pub)
					}
					if valid {
						select {
						case client.server.incoming <- pub:
						case <-client.close:
							return
						}
					}
				}
			case *packets.Puback:
				pub := packet.(*packets.Puback)
				inflightElem := &inflightElem{
					pid:    pub.PacketId,
					packet: pub,
				}
				client.unsetInflight(inflightElem)
			case *packets.Pubrel:
				pub := packet.(*packets.Pubrel)
				delete(client.session.unackpublish, pub.PacketId)
				pubcomp := pub.NewPubcomp()
				client.write(pubcomp)
			case *packets.Pubrec:
				pub := packet.(*packets.Pubrec)
				inflightElem := &inflightElem{
					pid:    pub.PacketId,
					packet: pub,
				}
				client.unsetInflight(inflightElem)
				pubrel := pub.NewPubrel()
				client.write(pubrel)
			case *packets.Pubcomp:
				pub := packet.(*packets.Pubcomp)
				inflightElem := &inflightElem{
					pid:    pub.PacketId,
					packet: pub,
				}
				client.unsetInflight(inflightElem)
			case *packets.Pingreq:
				ping := packet.(*packets.Pingreq)
				resp := ping.NewPingresp()
				client.write(resp)
			case *packets.Unsubscribe:
				unSub := packet.(*packets.Unsubscribe)
				unSuback := unSub.NewUnSubBack()
				client.write(unSuback)
				//删除client的订阅列表
				s.topicsMu.Lock()
				for _, topicName := range unSub.Topics {
					delete(client.session.subTopics, topicName)
				}
				s.topicsMu.Unlock()
			case *packets.Disconnect:
				//正常关闭
				client.cleanWillFlag = true
				return
			default:
				err = errors.New("invalid packet")
				return
			}

		}
	}
}

//session重发,退出条件，client连接关闭
func (client *Client) redeliver() {

	var err error
	s := client.session
	defer func() {
		if re := recover(); re != nil {
			err = errors.New(fmt.Sprint(re))
		}
		client.setError(err)
		client.wg.Done()
	}()
	retryInterval := client.server.config.deliveryRetryInterval
	timer := time.NewTicker(retryInterval)
	for {
		select {
		case <-client.close: //关闭广播
			return
		case <-timer.C: //重发ticker
			s.inflightMu.Lock()
			for inflight := s.inflight.Front(); inflight != nil; inflight = inflight.Next() {
				if inflight, ok := inflight.Value.(*inflightElem); ok {
					if time.Now().Unix()-inflight.at.Unix() >= int64(retryInterval.Seconds()) {
						switch inflight.packet.(type) { //publish 和 pubrel要重发
						case *packets.Publish:
							publish := inflight.packet.(*packets.Publish)
							pub := publish.CopyPublish()
							pub.Dup = true
							pub.PacketId = publish.PacketId
							if log != nil {
								log.Printf("%-15s %v: %s", "redelivering:", client.rwc.RemoteAddr(), publish)
							}
							client.write(pub)
						case *packets.Pubrel:
							pubrel := inflight.packet.(*packets.Pubrel)
							if log != nil {
								log.Printf("%-15s %v: %s", "redelivering:", client.rwc.RemoteAddr(), pubrel)
							}
							client.write(pubrel)
						}
					}
				}
			}
			s.inflightMu.Unlock()
		}
	}
}

//server goroutine结束的条件:1客户端断开连接 或 2发生错误
func (client *Client) serve() {
	defer client.internalClose()
	client.wg.Add(3)
	go client.errorWatch()
	go client.readLoop()                       //read packet
	go client.writeLoop()                      //write packet
	if ok := client.connectWithTimeOut(); ok { //链接成功,建立session
		client.wg.Add(2)
		go client.readHandle()
		go client.redeliver()
	}
	client.wg.Wait()
}
