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
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/DrmagicE/gmqtt/pkg/codes"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/DrmagicE/gmqtt/subscription"
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

	// ClientOptions return a reference of ClientOptions. Do not edit.
	// This is mainly used in callback functions.
	ClientOptions() *ClientOptions
	// Version return the protocol version of the current client.
	Version() packets.Version
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
	Disconnect(disconnect *packets.Disconnect)

	GetSessionStatsManager() SessionStatsManager
}

func (client *client) ClientOptions() *ClientOptions {
	return client.opts
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
	opts          *ClientOptions //OnConnect之前填充,set up before OnConnect()
	cleanWillFlag bool           //收到DISCONNECT报文删除遗嘱标志, whether to remove will msg
	ready         chan struct{}  //close after session prepared

	connectedAt    int64
	disconnectedAt int64

	statsManager SessionStatsManager
	version      packets.Version
	aliasMapper  *aliasMapper

	// gard serverReceiveMaximumQuota
	serverQuotaMu             sync.Mutex
	serverReceiveMaximumQuota uint16

	// gard clientReceiveMaximumQuota
	clientQuotaMu             sync.Mutex
	clientReceiveMaximumQuota uint16
}

func (client *client) Version() packets.Version {
	return client.version
}

func (client *client) Disconnect(disconnect *packets.Disconnect) {
	panic("implement me")
}

type ClientSettings struct {
	SessionExpiry                 uint32
	ReceiveMaximum                uint16
	MaximumQoS                    byte
	RetainAvailable               bool
	MaximumPacketSize             uint32
	AssignedClientID              []byte
	TopicAliasMaximum             uint16
	ReasonString                  string
	UserProperties                []packets.UserProperty
	WildcardSubscriptionAvailable bool
	SubscriptionIDAvailable       bool
	SharedSubscriptionAvailable   bool
	ServerKeepAlive               uint16
	ResponseInformation           string
	ServerReference               string
}

type aliasMapper struct {
	server [][]byte
	client map[string]uint16
	// next alias will be send to client
	nextAlias uint16
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

// ClientOptions will be set after the client connected successfully
type ClientOptions struct {
	ClientID string
	Username string

	KeepAlive     uint16
	CleanStart    bool
	SessionExpiry uint32

	ClientReceiveMax uint16
	ServerReceiveMax uint16

	ClientMaxPacketSize uint32
	ServerMaxPacketSize uint32

	ClientTopicAliasMax uint16
	ServerTopicAliasMax uint16

	RequestResponseInfo bool
	RequestProblemInfo  bool
	UserProperties      []*packets.UserProperty

	RetainAvailable      bool
	WildcardSubAvailable bool
	SubIDAvailable       bool
	SharedSubAvailable   bool

	Will struct {
		Flag          bool
		Retain        bool
		Qos           uint8
		Topic         []byte
		Payload       []byte
		DelayInterval uint32
		PayloadFormat byte
		MessageExpiry uint32
		ContentType   []byte
		ResponseTopic []byte
		Properties    *packets.Properties
	}
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
			switch p := packet.(type) {
			case *packets.Publish:
				client.server.statsManager.messageSent(p.Qos)
				client.statsManager.messageSent(p.Qos)

				if client.version == packets.Version5 {
					if client.opts.ClientTopicAliasMax > 0 {
						// use topic alias if exist
						if id, ok := client.aliasMapper.client[string(p.TopicName)]; ok {
							p.TopicName = []byte{}
							p.Properties.TopicAlias = &id
						} else if len(client.aliasMapper.client) < int(client.opts.ClientTopicAliasMax) {
							// create new alias
							// TODO
							//  增加钩子函数决定是否对当前topic 使用topicAlias
							//  可能的input: packets.Message， 以及当前的alias情况。
							next := client.aliasMapper.nextAlias
							p.Properties.TopicAlias = &next
							client.aliasMapper.nextAlias++
							client.aliasMapper.client[string(p.TopicName)] = next
						}
					}
				}
				// onDeliver hook
				if client.server.hooks.OnDeliver != nil {
					client.server.hooks.OnDeliver(context.Background(), client, messageFromPublish(p))
				}

			case *packets.Puback, *packets.Pubcomp:
				if client.version == packets.Version5 {
					client.addServerQuota()
				}
			case *packets.Pubrel:
				if client.version == packets.Version5 && p.Code >= codes.UnspecifiedError {
					client.addServerQuota()
				}
			}
			err = client.writePacket(packet)
			if err != nil {
				return
			}
			client.server.statsManager.packetSent(packet)
		}

	}
}

func (client *client) writePacket(packet packets.Packet) error {
	zaplog.Debug("sending packet",
		zap.String("packet", packet.String()),
		zap.String("client_id", client.opts.ClientID),
		zap.String("remote_addr", client.rwc.RemoteAddr().String()),
	)
	err := client.packetWriter.WritePacket(packet)
	if err != nil {
		return err
	}
	return client.packetWriter.Flush()
}
func (client *client) addServerQuota() {
	client.serverQuotaMu.Lock()
	if client.serverReceiveMaximumQuota < client.opts.ServerReceiveMax {
		client.serverReceiveMaximumQuota++
	}
	client.serverQuotaMu.Unlock()
}
func (client *client) tryDecServerQuota() error {
	client.serverQuotaMu.Lock()
	defer client.serverQuotaMu.Unlock()
	if client.serverReceiveMaximumQuota == 0 {
		return codes.NewError(codes.RecvMaxExceeded)
	}
	client.serverReceiveMaximumQuota--
	return nil
}

func (client *client) addClientQuota() {

	client.clientQuotaMu.Lock()
	if client.clientReceiveMaximumQuota < client.opts.ClientReceiveMax {
		client.clientReceiveMaximumQuota++
	}
	zaplog.Debug("add client quota",
		zap.String("client_id", client.opts.ClientID),
		zap.Uint16("quota", client.clientReceiveMaximumQuota),
		zap.String("remote_addr", client.rwc.RemoteAddr().String()),
	)
	client.clientQuotaMu.Unlock()
}
func (client *client) tryDecClientQuota() bool {
	client.clientQuotaMu.Lock()
	defer client.clientQuotaMu.Unlock()
	if client.clientReceiveMaximumQuota == 0 {
		return false
	}
	client.clientReceiveMaximumQuota--
	zaplog.Debug("dec client quota",
		zap.String("client_id", client.opts.ClientID),
		zap.Uint16("quota", client.clientReceiveMaximumQuota),
		zap.String("remote_addr", client.rwc.RemoteAddr().String()))
	return true
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
			if keepAlive := client.opts.KeepAlive; keepAlive != 0 { //KeepAlive
				client.rwc.SetReadDeadline(time.Now().Add(time.Duration(keepAlive/2+keepAlive) * time.Second))
			}
		}
		// TODO validate Options
		packet, err = client.packetReader.ReadPacket()
		if err != nil {
			return
		}
		zaplog.Debug("received packet",
			zap.String("packet", packet.String()),
			zap.String("remote", client.rwc.RemoteAddr().String()),
			zap.String("client_id", client.opts.ClientID),
		)
		client.server.statsManager.packetReceived(packet)
		if pub, ok := packet.(*packets.Publish); ok {
			client.server.statsManager.messageReceived(pub.Qos)
			if client.version == packets.Version5 {
				err = client.tryDecServerQuota()
				if err != nil {
					return
				}
			}
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
		//time.Sleep(2 * time.Second)
		client.err = err
		close(client.close) //退出chanel
		if client.version == packets.Version5 {
			if code, ok := client.err.(*codes.Error); ok {
				// 发 Disconnect 或者 connack
				if client.IsConnected() {
					// send Disconnect
					_ = client.writePacket(&packets.Disconnect{
						Version: packets.Version5,
						Code:    code.Code(),
					})

				} else {
					// send connack
					_ = client.writePacket(&packets.Connack{
						Version: client.version,
						Code:    code.Code(),
					})
				}
			}
		}
		client.rwc.Close()
		return
	}
}

// Close 关闭客户端连接，连接关闭完毕会将返回的channel关闭。
//
// Close closes the client connection. The returned channel will be closed after unregister process has been done
func (client *client) Close() <-chan struct{} {
	client.rwc.Close()
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

type authRequest struct {
	connectRequest *connectRequest
	auth           *packets.Auth
}

func (a *authRequest) Packet() *packets.Auth {
	return a.auth
}

func (a *authRequest) ConnectRequest() ConnectRequest {
	return a.connectRequest
}

type connectRequest struct {
	connect *packets.Connect
	ppt     *packets.Properties
	config  *Config
}

func (c *connectRequest) Packet() *packets.Connect {
	return c.connect
}

func bool2Byte(bo bool) *byte {
	var b byte
	if bo {
		b = 1
	} else {
		b = 0
	}
	return &b
}

func byte2bool(b *byte) bool {
	if b == nil || *b == 0 {
		return false
	}
	return true
}

func convertUint16(u *uint16, defaultValue uint16) uint16 {
	if u == nil {
		return defaultValue
	}
	return *u
}

func convertUint32(u *uint32, defaultValue uint32) uint32 {
	if u == nil {
		return defaultValue
	}
	return *u

}

func (c *connectRequest) DefaultConnackProperties() *packets.Properties {
	if c.connect.Version != packets.Version5 {
		return nil
	}
	if c.ppt == nil {
		ppt := &packets.Properties{
			SessionExpiryInterval: c.connect.Properties.SessionExpiryInterval,
			ReceiveMaximum:        &c.config.MQTT.ReceiveMax,
			MaximumQOS:            &c.config.MQTT.MaximumQOS,
			RetainAvailable:       bool2Byte(c.config.MQTT.RetainAvailable),
			MaximumPacketSize:     &c.config.MQTT.MaxPacketSize,
			TopicAliasMaximum:     &c.config.MQTT.TopicAliasMax,
			WildcardSubAvailable:  bool2Byte(c.config.MQTT.WildcardAvailable),
			SubIDAvailable:        bool2Byte(c.config.MQTT.SubscriptionIDAvailable),
			SharedSubAvailable:    bool2Byte(c.config.MQTT.SharedSubAvailable),
		}
		if i := ppt.SessionExpiryInterval; i != nil && *i < c.config.SessionExpiryInterval {
			ppt.SessionExpiryInterval = i
		}
		if c.connect.KeepAlive > c.config.MQTT.MaxKeepAlive {
			ppt.ServerKeepAlive = &c.config.MQTT.MaxKeepAlive
		}
		c.ppt = ppt
	}
	return c.ppt
}

func (client *client) connectWithTimeOut() (ok bool) {
	var err error
	srv := client.server
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
	var conn *packets.Connect
	var connRequest *connectRequest
	for {
		select {
		case <-client.close:
			return
		case p := <-client.in:
			var ack *AuthResponse
			switch p.(type) {
			case *packets.Connect:
				if conn != nil {
					err = codes.ErrProtocol
					return
				}

				// set default options.
				client.opts.RetainAvailable = srv.config.MQTT.RetainAvailable
				client.opts.WildcardSubAvailable = srv.config.MQTT.WildcardAvailable
				client.opts.SubIDAvailable = srv.config.MQTT.SubscriptionIDAvailable
				client.opts.SharedSubAvailable = srv.config.MQTT.SharedSubAvailable

				conn = p.(*packets.Connect)
				client.version = conn.Version
				connRequest = &connectRequest{
					config:  &client.server.config,
					connect: conn,
				}
				if srv.hooks.OnConnect != nil {
					ack = client.server.hooks.OnConnect(context.Background(), connRequest, client)
				} else {
					ack = &AuthResponse{
						Code:       codes.Success,
						Properties: connRequest.DefaultConnackProperties(),
					}
				}
			case *packets.Auth:
				if conn == nil {
					err = codes.ErrProtocol
					return
				}
				if srv.hooks.OnAuth != nil {
					ack = client.server.hooks.OnAuth(context.Background(), &authRequest{
						connectRequest: connRequest,
						auth:           p.(*packets.Auth),
					}, client)
				}
			}
			if ack.Code == codes.ContinueAuthentication {
				client.out <- &packets.Auth{
					Code:       ack.Code,
					Properties: ack.Properties,
				}
				continue
			}
			if ack.Code == codes.Success {
				if client.version == packets.Version5 {
					ppt := ack.Properties
					client.opts.RetainAvailable = byte2bool(ppt.RetainAvailable)
					client.opts.WildcardSubAvailable = byte2bool(ppt.WildcardSubAvailable)
					client.opts.SubIDAvailable = byte2bool(ppt.SubIDAvailable)
					client.opts.SharedSubAvailable = byte2bool(ppt.SharedSubAvailable)

					client.opts.SessionExpiry = convertUint32(ppt.SessionExpiryInterval, 0)

					client.opts.ClientReceiveMax = convertUint16(conn.Properties.ReceiveMaximum, 65535)
					client.opts.ServerReceiveMax = convertUint16(ack.Properties.ReceiveMaximum, 65535)

					client.opts.ClientMaxPacketSize = convertUint32(conn.Properties.MaximumPacketSize, 0)
					client.opts.ServerMaxPacketSize = convertUint32(ack.Properties.MaximumPacketSize, 0)

					client.opts.ClientTopicAliasMax = convertUint16(conn.Properties.TopicAliasMaximum, 0)
					client.opts.ServerTopicAliasMax = convertUint16(ack.Properties.TopicAliasMaximum, 0)

					client.clientReceiveMaximumQuota = client.opts.ClientReceiveMax
					client.serverReceiveMaximumQuota = client.opts.ServerReceiveMax

					client.aliasMapper.client = make(map[string]uint16)
					client.aliasMapper.server = make([][]byte, client.opts.ServerReceiveMax+1)

					if len(conn.ClientID) == 0 {
						if len(ppt.AssignedClientID) != 0 {
							client.opts.ClientID = string(ppt.AssignedClientID)
						} else {
							client.opts.ClientID = getRandomUUID()
							ppt.AssignedClientID = []byte(client.opts.ClientID)
						}
					} else {
						client.opts.ClientID = string(conn.ClientID)
					}
					client.opts.KeepAlive = convertUint16(ppt.ServerKeepAlive, conn.KeepAlive)

				} else {
					if len(conn.ClientID) == 0 {
						client.opts.ClientID = getRandomUUID()
					} else {
						client.opts.ClientID = string(conn.ClientID)
					}
					client.opts.KeepAlive = conn.KeepAlive
				}
				if keepAlive := client.opts.KeepAlive; keepAlive != 0 { //KeepAlive
					client.rwc.SetReadDeadline(time.Now().Add(time.Duration(keepAlive/2+keepAlive) * time.Second))
				}

				client.opts.CleanStart = conn.CleanStart

				client.opts.Will.Flag = conn.WillFlag
				client.opts.Will.Payload = conn.WillMsg
				client.opts.Will.Qos = conn.WillQos
				client.opts.Will.Retain = conn.WillRetain
				client.opts.Will.Topic = conn.WillTopic
				client.opts.Will.Properties = conn.WillProperties

				client.server.registerClient(conn, ack, client)

				return
			}
			err = codes.NewError(ack.Code)
			return
		case <-timeout.C:
			err = ErrConnectTimeOut
			return
		}
	}

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
		client.server.unregisterClient(client)
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
// goroutine safe
func (client *client) onlinePublish(publish *packets.Publish) {
	if publish.Qos >= packets.Qos1 {
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

	select {
	case <-client.close:
	case client.out <- publish:
	}
}

func (client *client) publish(publish *packets.Publish) {
	if client.IsConnected() { //在线消息
		// set topic alias
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
func (client *client) subscribeHandler(sub *packets.Subscribe) *codes.Error {
	srv := client.server
	if srv.hooks.OnSubscribe != nil {
		for k, v := range sub.Topics {
			qos := srv.hooks.OnSubscribe(context.Background(), client, v)
			sub.Topics[k].Qos = qos
		}
	}
	var msgs []packets.Message

	suback := sub.NewSuback()
	var subID uint32

	if client.version == packets.Version5 {
		if client.opts.SubIDAvailable && len(sub.Properties.SubscriptionIdentifier) != 0 {
			subID = sub.Properties.SubscriptionIdentifier[0]
		}
		if !srv.config.MQTT.SubscriptionIDAvailable && subID != 0 {
			return codes.NewError(codes.SubIDNotSupported)
		}
	}

	for k, v := range sub.Topics {
		var isShared bool
		if client.version == packets.Version5 {

			if strings.HasPrefix(v.Name, "$share/") {
				isShared = true
				if !client.opts.SharedSubAvailable {
					v.Qos = codes.SharedSubNotSupported
				}
			}
			if !client.opts.SubIDAvailable && subID != 0 {
				v.Qos = codes.SubIDNotSupported
			}

			if !client.opts.WildcardSubAvailable {
				for _, c := range v.Name {
					if c == '+' || c == '#' {
						v.Qos = codes.WildcardSubNotSupported
						break
					}
				}
			}
		}
		if v.Qos < packets.SubscribeFailure {
			topic := packets.Topic{
				SubOptions: packets.SubOptions{
					Qos:               v.Qos,
					RetainAsPublished: v.RetainAsPublished,
					NoLocal:           v.NoLocal,
					RetainHandling:    v.RetainHandling,
				},
				Name: v.Name,
			}

			subRs := srv.subscriptionsDB.Subscribe(client.opts.ClientID, subscription.FromTopic(topic, subID))
			if srv.hooks.OnSubscribed != nil {
				srv.hooks.OnSubscribed(context.Background(), client, topic)
			}
			zaplog.Info("subscribe succeeded",
				zap.String("topic", v.Name),
				zap.Uint8("qos", suback.Payload[k]),
				zap.String("client_id", client.opts.ClientID),
				zap.String("remote_addr", client.rwc.RemoteAddr().String()),
			)
			// TODO no local
			if !isShared && ((!subRs[0].AlreadyExisted && v.RetainHandling != 2) || v.RetainHandling == 0) {
				msgs = srv.retainedDB.GetMatchedMessages(topic.Name)
				for _, v := range msgs {
					publish := messageToPublish(v, client.version)
					if publish.Qos > subRs[0].Subscription.QoS() {
						publish.Qos = subRs[0].Subscription.QoS()
					}
					publish.Dup = false
					if !topic.RetainAsPublished {
						publish.Retain = false
					}
					// TODO 放到msgRouter里面
					client.publish(publish)
				}
			}
		} else {
			zaplog.Info("subscribe failed",
				zap.String("topic", v.Name),
				zap.Uint8("qos", suback.Payload[k]),
				zap.String("client_id", client.opts.ClientID),
				zap.String("remote_addr", client.rwc.RemoteAddr().String()),
			)
		}
	}
	client.write(suback)
	return nil
}

//Publish handler
//The PUBLISH packet sent to a Client by the Server MUST contain a
// Message Expiry Interval set to the received value minus the time that the Application Message has been waiting in the Server
func (client *client) publishHandler(pub *packets.Publish) *codes.Error {
	s := client.session
	srv := client.server
	var dup bool

	// check retain available
	if !client.opts.RetainAvailable && pub.Retain {
		return codes.NewError(codes.RetainNotSupported)
	}

	if client.version == packets.Version5 {
		if pub.Properties.TopicAlias != nil {
			if *pub.Properties.TopicAlias >= client.opts.ServerTopicAliasMax {
				return codes.NewError(codes.TopicAliasInvalid)
			} else {
				topicAlias := *pub.Properties.TopicAlias
				name := client.aliasMapper.server[int(topicAlias)]
				if len(pub.TopicName) == 0 {
					if len(name) == 0 {
						return codes.NewError(codes.TopicAliasInvalid)
					}
					pub.TopicName = name
				} else {
					client.aliasMapper.server[topicAlias] = pub.TopicName
				}
			}
		}
	}

	if pub.Qos == packets.Qos1 {
		puback := pub.NewPuback()
		client.write(puback)
	}
	if pub.Qos == packets.Qos2 {
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
			// TODO 支持返回 code
			valid = srv.hooks.OnMsgArrived(context.Background(), client, msg)
		}
		if valid {
			msgRouter := &msgRouter{msg: msg, srcClientID: client.opts.ClientID}
			select {
			case <-client.close:
			case client.server.msgRouter <- msgRouter:
			}
		}
	}
	return nil

}
func (client *client) pubackHandler(puback *packets.Puback) {
	if client.version == packets.Version5 {
		client.addClientQuota()
	}
	client.unsetInflight(puback)
}
func (client *client) pubrelHandler(pubrel *packets.Pubrel) {
	if client.version == packets.Version5 {
		if pubrel.Code >= codes.UnspecifiedError {
			client.addClientQuota()
		}
	}
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
	if client.version == packets.Version5 {
		client.addClientQuota()
	}
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
		srv.subscriptionsDB.Unsubscribe(client.opts.ClientID, topicName)
		if srv.hooks.OnUnsubscribed != nil {
			srv.hooks.OnUnsubscribed(context.Background(), client, topicName)
		}
		zaplog.Info("unsubscribed",
			zap.String("topic", topicName),
			zap.String("client_id", client.opts.ClientID),
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
			var err *codes.Error
			switch packet.(type) {
			case *packets.Subscribe:
				err = client.subscribeHandler(packet.(*packets.Subscribe))
			case *packets.Publish:
				err = client.publishHandler(packet.(*packets.Publish))
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
				err = codes.ErrProtocol
			}
			if err != nil {
				client.setError(err)
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
								Flags:        packets.FlagPubrel,
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
