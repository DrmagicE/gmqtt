// Package gmqtt provides an MQTT v3.1.1 server library.
package gmqtt

import (
	"bufio"
	"container/list"
	"crypto/md5"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// Error
var (
	ErrInvalStatus    = errors.New("invalid connection status")
	ErrConnectTimeOut = errors.New("connect time out")
)

// Client status
const (
	Connecting = iota
	Connected
	Switiching
	Disconnected
)
const (
	readBufferSize  = 4096
	writeBufferSize = 4096
	redeliveryTime  = 20
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

// Client represents a MQTT client
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


// UserData returns the user data
func (client *Client) UserData() interface{} {
	client.userMutex.Lock()
	defer client.userMutex.Unlock()
	return client.userData
}

// SetUserData is used to bind user data to the client
func (client *Client) SetUserData(data interface{}) {
	client.userMutex.Lock()
	defer client.userMutex.Unlock()
	client.userData = data
}

//ClientOptions returns the ClientOptions. This is mainly used in callback functions.
//See ./example/hook
func (client *Client) ClientOptions() ClientOptions {
	opts := *client.opts
	opts.WillPayload = make([]byte, len(client.opts.WillPayload))
	copy(opts.WillPayload, client.opts.WillPayload)
	return opts
}

func (client *Client) setConnecting() {
	atomic.StoreInt32(&client.status, Connecting)
}

func (client *Client) setSwitching() {
	atomic.StoreInt32(&client.status, Switiching)
}

func (client *Client) setConnected() {
	atomic.StoreInt32(&client.status, Connected)
}

func (client *Client) setDisConnected() {
	atomic.StoreInt32(&client.status, Disconnected)
}

//Status returns client's status
func (client *Client) Status() int32 {
	return atomic.LoadInt32(&client.status)
}

// IsConnected returns whether the client is connected or not.
func (client *Client) IsConnected() bool {
	return client.Status() == Connected
}

//ClientOptions is mainly used in callback functions.
//See ClientOptions()
type ClientOptions struct {
	ClientID     string
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
			err = client.writePacket(packet)
			if err != nil {
				return
			}

		}

	}
}

func (client *Client) writePacket(packet packets.Packet) error {
	if log != nil {
		log.Printf("%-15s %v: %s ", "sending to", client.rwc.RemoteAddr(), packet)
	}
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
		if client.IsConnected() {
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

// Close 关闭客户端连接，连接关闭完毕会将返回的channel关闭。
//
// Close closes the client connection. The returned channel will be closed after unregister process has been done
func (client *Client) Close() <-chan struct{} {
	client.setError(nil)
	return client.closeComplete
}

var pid = os.Getpid()
var counter uint32
var machineId = readMachineId()

func readMachineId() []byte {
	id := make([]byte, 3, 3)
	hostname, err1 := os.Hostname()
	if err1 != nil {
		_, err2 := io.ReadFull(rand.Reader, id)
		if err2 != nil {
			panic(fmt.Errorf("cannot get hostname: %v; %v", err1, err2))
		}
		return id
	}
	hw := md5.New()
	hw.Write([]byte(hostname))
	copy(id, hw.Sum(nil))
	return id
}

func getRandomUUID() string {
	var b [12]byte
	// Timestamp, 4 bytes, big endian
	binary.BigEndian.PutUint32(b[:], uint32(time.Now().Unix()))
	// Machine, first 3 bytes of md5(hostname)
	b[4] = machineId[0]
	b[5] = machineId[1]
	b[6] = machineId[2]
	// Pid, 2 bytes, specs don't specify endianness, but we use big endian.
	b[7] = byte(pid >> 8)
	b[8] = byte(pid)
	// Increment, 3 bytes, big endian
	i := atomic.AddUint32(&counter, 1)
	b[9] = byte(i >> 16)
	b[10] = byte(i >> 8)
	b[11] = byte(i)
	return fmt.Sprintf(`%x`, string(b[:]))
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
	client.opts.ClientID = string(conn.ClientID)
	if client.opts.ClientID == "" {
		client.opts.ClientID = getRandomUUID()
	}
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
		unackpublish:        make(map[packets.PacketID]bool),
		inflight:            list.New(),
		msgQueue:            list.New(),
		lockedPid:           make(map[packets.PacketID]bool),
		freePid:             1,
		maxInflightMessages: client.server.config.maxInflightMessages,
		maxQueueMessages:    client.server.config.maxQueueMessages,
	}
	client.session = s
}

func (client *Client) internalClose() {
	defer close(client.closeComplete)
	if client.Status() != Switiching {
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
	if client.Status() == Connected { //在线消息
		if publish.Qos >= packets.QOS_1 {
			if publish.Dup == true {
				//redelivery on reconnect,use the original packet id
				client.session.setPacketID(publish.PacketID)
			} else {
				publish.PacketID = client.session.getPacketID()
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
			if srv.subscribe(client.opts.ClientID, topic) {
				isNew = true
			}
			if client.server.Monitor != nil {
				client.server.Monitor.subscribe(SubscriptionsInfo{
					ClientID: client.opts.ClientID,
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
		if _, ok := s.unackpublish[pub.PacketID]; ok {
			dup = true
		} else {
			s.unackpublish[pub.PacketID] = true
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
	delete(client.session.unackpublish, pubrel.PacketID)
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
		srv.unsubscribe(client.opts.ClientID, topicName)
		if client.server.Monitor != nil {
			client.server.Monitor.unSubscribe(client.opts.ClientID, topicName)
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
