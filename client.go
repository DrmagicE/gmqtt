// Package gmqtt provides an MQTT v3.1.1 server library.
package gmqtt

import (
	"bufio"
	"container/list"
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/DrmagicE/gmqtt/pkg/packets"
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

// Client represent
type Client interface {
	// OptionsReader returns ClientOptionsReader for reading options data.
	OptionsReader() ClientOptionsReader
	// IsConnected returns whether the client is connected.
	IsConnected() bool
	// ConnectedAt returns the connected time
	ConnectedAt() time.Time
	// DisconnectedAt return the disconnected time
	DisconnectedAt() time.Time
	// Connection returns the raw net.Conn
	Connection() net.Conn
	// Close closes the client connection. The returned channel will be closed after unregister process has been done
	Close() <-chan struct{}

	GetSessionStatsManager() SessionStatsManager
}

// Client represents a MQTT client and implements the Client interface
type client struct {
	server        *server
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
	error         chan error //错误
	err           error
	opts          *options //OnConnect之前填充,set up before OnConnect()
	cleanWillFlag bool     //收到DISCONNECT报文删除遗嘱标志, whether to remove will msg
	//自定义数据
	keys  map[string]interface{}
	ready chan struct{} //close after session prepared

	connectedAt    int64
	disconnectedAt int64

	statsManager SessionStatsManager
}

func (client *client) GetSessionStatsManager() SessionStatsManager {
	return client.statsManager
}

func (client *client) setConnectedAt(time time.Time) {
	atomic.StoreInt64(&client.connectedAt, time.Unix())
}
func (client *client) setDisconnectedAt(time time.Time) {
	atomic.StoreInt64(&client.disconnectedAt, time.Unix())
}

// ConnectedAt
func (client *client) ConnectedAt() time.Time {
	return time.Unix(atomic.LoadInt64(&client.connectedAt), 0)
}

// DisconnectedAt
func (client *client) DisconnectedAt() time.Time {
	return time.Unix(atomic.LoadInt64(&client.disconnectedAt), 0)
}

// Connection returns the raw net.Conn
func (client *client) Connection() net.Conn {
	return client.rwc
}

//OptionsReader returns the ClientOptionsReader. This is mainly used in callback functions.
//See ./example/hook
func (client *client) OptionsReader() ClientOptionsReader {
	return client.opts
	/*opts.WillPayload = make([]byte, len(client.opts.WillPayload))
	copy(opts.WillPayload, client.opts.WillPayload)
	return opts*/
}

func (client *client) setConnecting() {
	atomic.StoreInt32(&client.status, Connecting)
}

func (client *client) setSwitching() {
	atomic.StoreInt32(&client.status, Switiching)
}

func (client *client) setConnected() {
	atomic.StoreInt32(&client.status, Connected)
}

func (client *client) setDisConnected() {
	atomic.StoreInt32(&client.status, Disconnected)
}

//Status returns client's status
func (client *client) Status() int32 {
	return atomic.LoadInt32(&client.status)
}

// IsConnected returns whether the client is connected or not.
func (client *client) IsConnected() bool {
	return client.Status() == Connected
}

// IsDisConnected returns whether the client is connected or not.
func (client *client) IsDisConnected() bool {
	return client.Status() == Disconnected
}

//ClientOptionsReader is mainly used in callback functions.
type ClientOptionsReader interface {
	ClientID() string
	Username() string
	Password() string
	KeepAlive() uint16
	CleanSession() bool
	WillFlag() bool
	WillRetain() bool
	WillQos() uint8
	WillTopic() string
	WillPayload() []byte
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
}

// options client options
type options struct {
	clientID     string
	username     string
	password     string
	keepAlive    uint16
	cleanSession bool
	willFlag     bool
	willRetain   bool
	willQos      uint8
	willTopic    string
	willPayload  []byte
	localAddr    net.Addr
	remoteAddr   net.Addr
}

// ClientID return clientID
func (o *options) ClientID() string {
	return o.clientID
}

// Username return username
func (o *options) Username() string {
	return o.username
}

// Password return Password
func (o *options) Password() string {
	return o.password
}

// KeepAlive return keepalive
func (o *options) KeepAlive() uint16 {
	return o.keepAlive
}

// CleanSession return cleanSession
func (o *options) CleanSession() bool {
	return o.cleanSession
}

// WillFlag return willflag
func (o *options) WillFlag() bool {
	return o.willFlag
}

// WillRetain return willRetain
func (o *options) WillRetain() bool {
	return o.willRetain
}
func (o *options) WillQos() uint8 {
	return o.willQos
}
func (o *options) WillTopic() string {
	return o.willTopic
}
func (o *options) WillPayload() []byte {
	return o.willPayload
}
func (o *options) LocalAddr() net.Addr {
	return o.localAddr
}
func (o *options) RemoteAddr() net.Addr {
	return o.remoteAddr
}

func (client *client) setError(err error) {
	select {
	case client.error <- err:
		if err != nil && err != io.EOF {
			zaplog.Error("connection lost", zap.String("error_msg", err.Error()))
		}
	default:
	}
}

func (client *client) writeLoop() {
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
			zaplog.Debug("sending packet",
				zap.String("packet", packet.String()),
				zap.String("client_id", client.opts.clientID),
				zap.String("remote_addr", client.rwc.RemoteAddr().String()),
			)
			err = client.writePacket(packet)
			if err != nil {
				return
			}
			client.server.statsManager.packetSent(packet)
			if pub, ok := packet.(*packets.Publish); ok {
				client.server.statsManager.messageSent(pub.Qos)
				client.statsManager.messageSent(pub.Qos)
			}
		}

	}
}

func (client *client) writePacket(packet packets.Packet) error {
	err := client.packetWriter.WritePacket(packet)
	if err != nil {
		return err
	}
	return client.packetWriter.Flush()
}

func (client *client) readLoop() {
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
			if keepAlive := client.opts.keepAlive; keepAlive != 0 { //KeepAlive
				client.rwc.SetReadDeadline(time.Now().Add(time.Duration(keepAlive/2+keepAlive) * time.Second))
			}
		}
		packet, err = client.packetReader.ReadPacket()
		if err != nil {
			return
		}
		zaplog.Debug("received packet",
			zap.String("packet", packet.String()),
			zap.String("remote", client.rwc.RemoteAddr().String()),
			zap.String("client_id", client.opts.clientID),
		)
		client.server.statsManager.packetReceived(packet)
		if pub, ok := packet.(*packets.Publish); ok {
			client.server.statsManager.messageReceived(pub.Qos)
		}
		client.in <- packet
	}
}

func (client *client) errorWatch() {
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
func (client *client) Close() <-chan struct{} {
	client.setError(nil)
	return client.closeComplete
}

var pid = os.Getpid()
var counter uint32
var machineId = readMachineId()

func readMachineId() []byte {
	id := make([]byte, 3)
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

func (client *client) connectWithTimeOut() (ok bool) {
	var err error
	defer func() {
		if err != nil {
			client.setError(err)
			ok = false
		} else {
			ok = true
		}
	}()
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	var p packets.Packet
	select {
	case <-client.close:
		return
	case p = <-client.in: //first packet
	case <-timeout.C:
		err = ErrConnectTimeOut
		return
	}
	conn, flag := p.(*packets.Connect)
	if !flag {
		err = ErrInvalStatus
		return
	}
	client.opts.clientID = string(conn.ClientID)
	if client.opts.clientID == "" {
		client.opts.clientID = getRandomUUID()
	}
	client.opts.keepAlive = conn.KeepAlive
	client.opts.cleanSession = conn.CleanSession
	client.opts.username = string(conn.Username)
	client.opts.password = string(conn.Password)
	client.opts.willFlag = conn.WillFlag
	client.opts.willPayload = conn.WillMsg
	client.opts.willQos = conn.WillQos
	client.opts.willTopic = string(conn.WillTopic)
	client.opts.willRetain = conn.WillRetain
	client.opts.remoteAddr = client.rwc.RemoteAddr()
	client.opts.localAddr = client.rwc.LocalAddr()
	if keepAlive := client.opts.keepAlive; keepAlive != 0 { //KeepAlive
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

func (client *client) newSession() {
	s := &session{
		unackpublish: make(map[packets.PacketID]bool),
		inflight:     list.New(),
		awaitRel:     list.New(),
		msgQueue:     list.New(),
		lockedPid:    make(map[packets.PacketID]bool),
		freePid:      1,
		config:       &client.server.config,
	}
	client.session = s
}

func (client *client) internalClose() {
	defer close(client.closeComplete)
	if client.Status() != Switiching {
		unregister := &unregister{client: client, done: make(chan struct{})}
		client.server.unregister <- unregister
		<-unregister.done
	}
	putBufioReader(client.bufr)
	putBufioWriter(client.bufw)

	// onClose hooks
	if client.server.hooks.OnClose != nil {
		client.server.hooks.OnClose(context.Background(), client, client.err)
	}
	client.setDisconnectedAt(time.Now())
	client.server.statsManager.addClientDisconnected()
	client.server.statsManager.decSessionActive()
}

// 这里的publish都是已经copy后的publish了
// 从msgRouter过来的publish 的dup不可能是true
func (client *client) onlinePublish(publish *packets.Publish) {
	if publish.Qos >= packets.QOS_1 {
		if publish.Dup {
			//redelivery on reconnect,use the original packet id
			client.session.setPacketID(publish.PacketID)
		} else {
			publish.PacketID = client.session.getPacketID()
		}
		if !client.setInflight(publish) {
			return
		}
	}
	client.sendMsg(publish)
}

// sendMsg wrap the hook function and session stats
func (client *client) sendMsg(publish *packets.Publish) {
	select {
	case <-client.close:
		return
	case client.out <- publish:
		// onDeliver hook
		if client.server.hooks.OnDeliver != nil {
			client.server.hooks.OnDeliver(context.Background(), client, messageFromPublish(publish))
		}
	}
}

func (client *client) publish(publish *packets.Publish) {
	if client.IsConnected() { //在线消息
		client.onlinePublish(publish)
	} else { //离线消息
		client.msgEnQueue(publish)
	}
}

func (client *client) write(packets packets.Packet) {
	select {
	case <-client.close:
		return
	case client.out <- packets:
	}
}

//Subscribe handler
func (client *client) subscribeHandler(sub *packets.Subscribe) {
	srv := client.server
	if srv.hooks.OnSubscribe != nil {
		for k, v := range sub.Topics {
			qos := srv.hooks.OnSubscribe(context.Background(), client, v)
			sub.Topics[k].Qos = qos
		}
	}
	var msgs []packets.Message
	suback := sub.NewSubBack()
	for k, v := range sub.Topics {
		if v.Qos != packets.SUBSCRIBE_FAILURE {
			topic := packets.Topic{
				Name: v.Name,
				Qos:  suback.Payload[k],
			}
			srv.subscriptionsDB.Subscribe(client.opts.clientID, topic)
			if srv.hooks.OnSubscribed != nil {
				srv.hooks.OnSubscribed(context.Background(), client, topic)
			}
			zaplog.Info("subscribe succeeded",
				zap.String("topic", v.Name),
				zap.Uint8("qos", suback.Payload[k]),
				zap.String("client_id", client.opts.clientID),
				zap.String("remote_addr", client.rwc.RemoteAddr().String()),
			)
			// matched retained messages
			msgs = srv.retainedDB.GetMatchedMessages(topic.Name)
		} else {
			zaplog.Info("subscribe failed",
				zap.String("topic", v.Name),
				zap.Uint8("qos", suback.Payload[k]),
				zap.String("client_id", client.opts.clientID),
				zap.String("remote_addr", client.rwc.RemoteAddr().String()),
			)
		}
	}
	client.write(suback)
	for _, msg := range msgs {
		srv.msgRouter <- &msgRouter{msg: msg, match: false, clientID: client.opts.clientID}
	}
}

//Publish handler
func (client *client) publishHandler(pub *packets.Publish) {
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
	msg := messageFromPublish(pub)
	if pub.Retain {
		if len(pub.Payload) == 0 {
			srv.retainedDB.Remove(string(pub.TopicName))
		} else {
			srv.retainedDB.AddOrReplace(msg)
		}
	}
	if !dup {
		var valid = true
		if srv.hooks.OnMsgArrived != nil {
			valid = srv.hooks.OnMsgArrived(context.Background(), client, msg)
		}
		if valid {
			pub.Retain = false
			msgRouter := &msgRouter{msg: messageFromPublish(pub), match: true}
			select {
			case <-client.close:
				return
			case client.server.msgRouter <- msgRouter:
			}
		}
	}
}
func (client *client) pubackHandler(puback *packets.Puback) {
	client.unsetInflight(puback)
}
func (client *client) pubrelHandler(pubrel *packets.Pubrel) {
	delete(client.session.unackpublish, pubrel.PacketID)
	pubcomp := pubrel.NewPubcomp()
	client.write(pubcomp)
}
func (client *client) pubrecHandler(pubrec *packets.Pubrec) {
	client.unsetInflight(pubrec)
	client.setAwaitRel(pubrec.PacketID)
	pubrel := pubrec.NewPubrel()
	client.write(pubrel)
}
func (client *client) pubcompHandler(pubcomp *packets.Pubcomp) {
	client.unsetAwaitRel(pubcomp.PacketID)
}
func (client *client) pingreqHandler(pingreq *packets.Pingreq) {
	resp := pingreq.NewPingresp()
	client.write(resp)
}
func (client *client) unsubscribeHandler(unSub *packets.Unsubscribe) {
	srv := client.server
	unSuback := unSub.NewUnSubBack()
	client.write(unSuback)
	for _, topicName := range unSub.Topics {
		if srv.hooks.OnUnsubscribe != nil {
			srv.hooks.OnUnsubscribe(context.Background(), client, topicName)
		}
		srv.subscriptionsDB.Unsubscribe(client.opts.clientID, topicName)
		if srv.hooks.OnUnsubscribed != nil {
			srv.hooks.OnUnsubscribed(context.Background(), client, topicName)
		}
		zaplog.Info("unsubscribed",
			zap.String("topic", topicName),
			zap.String("client_id", client.opts.clientID),
			zap.String("remote_addr", client.rwc.RemoteAddr().String()),
		)
	}

}

//读处理
func (client *client) readHandle() {
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

//重传处理, 除了重传递publish之外，pubrel也要重传
func (client *client) redeliver() {
	var err error
	s := client.session
	defer func() {
		if re := recover(); re != nil {
			err = errors.New(fmt.Sprint(re))
		}
		client.setError(err)
		client.wg.Done()
	}()
	retryCheckInterval := client.server.config.RetryCheckInterval
	retryInterval := client.server.config.RetryInterval
	timer := time.NewTicker(retryCheckInterval)
	defer timer.Stop()
	for {
		select {
		case <-client.close: //关闭广播
			return
		case <-timer.C: //重发ticker
			now := time.Now()
			s.inflightMu.Lock()
			for inflight := s.inflight.Front(); inflight != nil; inflight = inflight.Next() {
				if inflight, ok := inflight.Value.(*inflightElem); ok {
					if now.Sub(inflight.at) >= retryInterval {
						pub := inflight.packet
						p := pub.CopyPublish()
						p.Dup = true
						client.write(p)
					}
				}
			}
			s.inflightMu.Unlock()

			s.awaitRelMu.Lock()
			for awaitRel := s.awaitRel.Front(); awaitRel != nil; awaitRel = awaitRel.Next() {
				if awaitRel, ok := awaitRel.Value.(*awaitRelElem); ok {
					if now.Sub(awaitRel.at) >= retryInterval {
						pubrel := &packets.Pubrel{
							FixHeader: &packets.FixHeader{
								PacketType:   packets.PUBREL,
								Flags:        packets.FLAG_PUBREL,
								RemainLength: 2,
							},
							PacketID: awaitRel.pid,
						}
						client.write(pubrel)
					}
				}
			}
			s.awaitRelMu.Unlock()
		}
	}
}

//server goroutine结束的条件:1客户端断开连接 或 2发生错误
func (client *client) serve() {
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
