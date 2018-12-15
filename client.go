// Package gmqtt provides an MQTT v3.1.1 server library.
package gmqtt

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
	ErrConnectTimeOut = errors.New("connect time out")
)

const (
	CONNECTING = iota
	CONNECTED
	SWITCHING
	DISCONNECTED
)
const (
	READ_BUFFER_SIZE  = 4096
	WRITE_BUFFER_SIZE = 4096
	REDELIVER_TIME    = 20 //second
)

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
	server        *Server
	wg            sync.WaitGroup
	rwc           net.Conn //raw tcp connection
	bufr          *bufio.Reader
	bufw          *bufio.Writer
	packetReader  *packets.Reader
	packetWriter  *packets.Writer
	in            chan packets.Packet
	out           chan packets.Packet
	writeMutex    sync.Mutex
	close         chan struct{} //关闭chan
	closeComplete chan struct{} //连接关闭
	status        int32         //client状态
	session       *session
	error         chan error //错误
	err           error
	opts          *ClientOptions //OnConnect之前填充,set up before OnConnect()
	cleanWillFlag bool           //收到DISCONNECT报文删除遗嘱标志, whether to remove will msg
	//自定义数据 user data
	userMutex sync.Mutex
	userData  interface{}

	ready chan struct{} //close after session prepared
}

//Get UserData
func (c *Client) UserData() interface{} {
	c.userMutex.Lock()
	defer c.userMutex.Unlock()
	return c.userData
}

//Set UserData
func (c *Client) SetUserData(data interface{}) {
	c.userMutex.Lock()
	defer c.userMutex.Unlock()
	c.userData = data
}

//ClientOptions returns the ClientOptions. This is mainly used in callback functions.
//See ./example/hook
func (c *Client) ClientOptions() ClientOptions {
	opts := *c.opts
	opts.WillPayload = make([]byte, len(c.opts.WillPayload))
	copy(opts.WillPayload, c.opts.WillPayload)
	return opts
}

func (c *Client) setConnecting() {
	atomic.StoreInt32(&c.status, CONNECTING)
}

func (c *Client) setSwitching() {
	atomic.StoreInt32(&c.status, SWITCHING)
}

func (c *Client) setConnected() {
	atomic.StoreInt32(&c.status, CONNECTED)
}

func (c *Client) setDisConnected() {
	atomic.StoreInt32(&c.status, DISCONNECTED)
}

//Status returns client's status
func (c *Client) Status() int32 {
	return atomic.LoadInt32(&c.status)
}

//ClientOptions
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
	var err error
	err = client.packetWriter.WritePacket(packet)
	if err != nil {
		return err
	}
	return client.packetWriter.Flush()

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
	case err := <-client.error: //有错误关闭
		client.err = err
		client.rwc.Close()
		close(client.close) //退出chanel
		return
	}
}

/*
 关闭客户端连接，连接关闭完毕会将返回的channel关闭。

 Close closes the client connection. The returned channel will be closed after unregister process has been done
*/
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
	register := &register{
		client:  client,
		connect: conn,
	}
	select {
	case client.server.register <- register:
	case <-client.close:
		return
	}
	select {
	case <-client.close:
		return
	case <-client.ready:
	}
	err = register.error
	return

}

func (client *Client) newSession() {
	s := &session{
		unackpublish:        make(map[packets.PacketId]bool),
		inflight:            list.New(),
		msgQueue:            list.New(),
		lockedPid:           make(map[packets.PacketId]bool),
		freePid:             1,
		maxInflightMessages: client.server.config.maxInflightMessages,
		maxQueueMessages:    client.server.config.maxQueueMessages,
	}
	client.session = s
}

func (client *Client) internalClose() {
	defer close(client.closeComplete)
	if client.Status() != SWITCHING {
		unregister := &unregister{client: client, done: make(chan struct{})}
		client.server.unregister <- unregister
		<-unregister.done
	}
	putBufioReader(client.bufr)
	putBufioWriter(client.bufw)
	if client.server.onClose != nil {
		client.server.onClose(client, client.err)
	}
}

func (client *Client) publish(publish *packets.Publish) {
	if client.Status() == CONNECTED { //在线消息
		if publish.Qos >= packets.QOS_1 {
			if publish.Dup == true {
				//redelivery on reconnect,use the original packet id
				client.session.setPacketId(publish.PacketId)
			} else {
				publish.PacketId = client.session.getPacketId()
			}
			if !client.setInflight(publish) {
				return
			}
		}
		select {
		case <-client.close:
			return
		case client.out <- publish:
		}
	} else { //离线消息
		client.msgEnQueue(publish)
	}
}

func (client *Client) write(packets packets.Packet) {
	select {
	case <-client.close:
		return
	case client.out <- packets:
	}

}

//Subscribe handler
func (client *Client) subscribeHandler(sub *packets.Subscribe) {
	srv := client.server
	if srv.onSubscribe != nil {
		for k, v := range sub.Topics {
			sub.Topics[k].Qos = client.server.onSubscribe(client, v)
		}
	}
	suback := sub.NewSubBack()
	client.write(suback)
	srv.subscriptionsDB.Lock()
	defer srv.subscriptionsDB.Unlock()
	var isNew bool
	for k, v := range sub.Topics {
		if v.Qos != packets.SUBSCRIBE_FAILURE {
			topic := packets.Topic{
				Name: v.Name,
				Qos:  suback.Payload[k],
			}
			if srv.subscribe(client.opts.ClientId, topic) {
				isNew = true
			}
			if client.server.Monitor != nil {
				client.server.Monitor.subscribe(SubscriptionsInfo{
					ClientId: client.opts.ClientId,
					Qos:      suback.Payload[k],
					Name:     v.Name,
					At:       time.Now(),
				})
			}
		}
	}
	if isNew {
		srv.retainedMsgMu.Lock()
		for _, msg := range srv.retainedMsg {
			msg.Retain = true
			msgRouter := &msgRouter{forceBroadcast: false, pub: msg}
			srv.msgRouter <- msgRouter
		}
		srv.retainedMsgMu.Unlock()
	}

}

//Publish handler
func (client *Client) publishHandler(pub *packets.Publish) {
	s := client.session
	srv := client.server
	var dup bool
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
		srv.retainedMsgMu.Lock()
		srv.retainedMsg[string(pub.TopicName)] = pub
		if len(pub.Payload) == 0 {
			delete(srv.retainedMsg, string(pub.TopicName))
		}
		srv.retainedMsgMu.Unlock()
	}
	if !dup {
		var valid bool
		valid = true
		if srv.onPublish != nil {
			valid = client.server.onPublish(client, pub)
		}
		if valid {
			pub.Retain = false
			msgRouter := &msgRouter{forceBroadcast: false, pub: pub}
			select {
			case <-client.close:
				return
			case client.server.msgRouter <- msgRouter:
			}
		}
	}
}
func (client *Client) pubackHandler(puback *packets.Puback) {
	client.unsetInflight(puback)
}
func (client *Client) pubrelHandler(pubrel *packets.Pubrel) {
	delete(client.session.unackpublish, pubrel.PacketId)
	pubcomp := pubrel.NewPubcomp()
	client.write(pubcomp)
}
func (client *Client) pubrecHandler(pubrec *packets.Pubrec) {
	client.unsetInflight(pubrec)
	pubrel := pubrec.NewPubrel()
	client.write(pubrel)
}
func (client *Client) pubcompHandler(pubcomp *packets.Pubcomp) {
	client.unsetInflight(pubcomp)
}
func (client *Client) pingreqHandler(pingreq *packets.Pingreq) {
	resp := pingreq.NewPingresp()
	client.write(resp)
}
func (client *Client) unsubscribeHandler(unSub *packets.Unsubscribe) {
	srv := client.server
	unSuback := unSub.NewUnSubBack()
	client.write(unSuback)
	srv.subscriptionsDB.Lock()
	defer srv.subscriptionsDB.Unlock()

	for _, topicName := range unSub.Topics {
		srv.unsubscribe(client.opts.ClientId, topicName)
		if client.server.Monitor != nil {
			client.server.Monitor.unSubscribe(client.opts.ClientId, topicName)
		}
	}
}

//读处理
func (client *Client) readHandle() {
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
		case <-client.close:
			return
		case packet := <-client.in:
			switch packet.(type) {
			case *packets.Subscribe:
				client.subscribeHandler(packet.(*packets.Subscribe))
			case *packets.Publish:
				client.publishHandler(packet.(*packets.Publish))
			case *packets.Puback:
				client.pubackHandler(packet.(*packets.Puback))
			case *packets.Pubrel:
				client.pubrelHandler(packet.(*packets.Pubrel))
			case *packets.Pubrec:
				client.pubrecHandler(packet.(*packets.Pubrec))
			case *packets.Pubcomp:
				client.pubcompHandler(packet.(*packets.Pubcomp))
			case *packets.Pingreq:
				client.pingreqHandler(packet.(*packets.Pingreq))
			case *packets.Unsubscribe:
				client.unsubscribeHandler(packet.(*packets.Unsubscribe))
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

//重传处理
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
	defer timer.Stop()
	for {
		select {
		case <-client.close: //关闭广播
			return
		case <-timer.C: //重发ticker
			s.inflightMu.Lock()
			for inflight := s.inflight.Front(); inflight != nil; inflight = inflight.Next() {
				if inflight, ok := inflight.Value.(*InflightElem); ok {
					if time.Now().Unix()-inflight.At.Unix() >= int64(retryInterval.Seconds()) {
						pub := inflight.Packet
						p := pub.CopyPublish()
						p.Dup = true
						if inflight.Step == 0 {
							client.write(p)
						}
						if inflight.Step == 1 { //pubrel
							pubrel := pub.NewPubrec().NewPubrel()
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
