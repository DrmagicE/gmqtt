// Package gmqtt provides an MQTT v3.1.1 server library.
package server

import (
	"bufio"
	"bytes"
	"container/list"
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/pkg/codes"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/DrmagicE/gmqtt/subscription"
)

// Error
var (
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
)

var (
	bufioReaderPool sync.Pool
	bufioWriterPool sync.Pool
)

func kvsToProperties(kvs []struct {
	K []byte
	V []byte
}) []packets.UserProperty {
	u := make([]packets.UserProperty, len(kvs))
	for k, v := range kvs {
		u[k].K = v.K
		u[k].V = v.V
	}
	return u
}

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

func (client *client) ClientOptions() *ClientOptions {
	return client.opts
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

	// AuthMethod v5 only
	AuthMethod []byte

	Will struct {
		Flag       bool
		Retain     bool
		Qos        uint8
		Topic      []byte
		Payload    []byte
		Properties *packets.Properties
	}
}

// TopicAliasManager manage the topic alias for V5 client.
// see topicalias/fifo for more details.
type TopicAliasManager interface {
	// Create will be called when the client connects
	Create(client Client)
	// Check return the alias number and whether the alias exist.
	// For examples:
	// If the publish alias exist and the manager decides to use the alias, it return the alias number and true.
	// If the publish alias exist, but the manager decides not to use alias, it return 0 and true.
	// If the publish alias not exist and the manager decides to assign a new alias, it return the new alias and false.
	// If the publish alias not exist, but the manager decides not to assign alias, it return the 0 and false.
	Check(client Client, publish *packets.Publish) (alias uint16, exist bool)
	// Delete will be called when the client disconnects.
	Delete(client Client)
}

// Client represent a MQTT client.
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
	errOnce       sync.Once
	err           error
	opts          *ClientOptions //OnConnect之前填充,set up before OnConnect()
	cleanWillFlag bool           //收到DISCONNECT报文删除遗嘱标志, whether to remove will Msg
	ready         chan struct{}  //close after session prepared

	connectedAt    int64
	disconnectedAt int64

	statsManager SessionStatsManager
	version      packets.Version
	aliasMapper  [][]byte

	// gard serverReceiveMaximumQuota
	serverQuotaMu             sync.Mutex
	serverReceiveMaximumQuota uint16

	// gard clientReceiveMaximumQuota
	clientQuotaMu             sync.Mutex
	clientReceiveMaximumQuota uint16

	config Config
	// for testing
	publishMessageHandler func(publish *packets.Publish)
}

func (client *client) Version() packets.Version {
	return client.version
}

func (client *client) Disconnect(disconnect *packets.Disconnect) {
	client.write(disconnect)
}

type aliasMapper struct {
	server [][]byte
}

func (client *client) GetSessionStatsManager() SessionStatsManager {
	return client.statsManager
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

func (client *client) setConnected(time time.Time) {
	atomic.StoreInt64(&client.connectedAt, time.Unix())
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

func (client *client) setError(err error) {
	client.errOnce.Do(func() {
		if err != nil && err != io.EOF {
			zaplog.Error("connection lost", zap.String("error_msg", err.Error()))
			client.err = err
			if client.version == packets.Version5 {
				if code, ok := err.(*codes.Error); ok {
					if client.IsConnected() {
						// send Disconnect
						client.out <- &packets.Disconnect{
							Version: packets.Version5,
							Code:    code.Code,
							Properties: &packets.Properties{
								ReasonString: code.ReasonString,
								User:         kvsToProperties(code.UserProperties),
							},
						}

					}
				}
			}
		}
	})
}

func (client *client) writeLoop() {
	var err error
	srv := client.server
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
		case packet := <-client.out:
			switch p := packet.(type) {
			case *packets.Publish:
				client.server.statsManager.messageSent(p.Qos)
				client.statsManager.messageSent(p.Qos)

				if client.version == packets.Version5 {
					if client.opts.ClientTopicAliasMax > 0 {
						// use alias if exist
						if alias, ok := srv.topicAliasManager.Check(client, p); ok {
							p.TopicName = []byte{}
							p.Properties.TopicAlias = &alias
						} else {
							// alias not exist
							if alias != 0 {
								p.Properties.TopicAlias = &alias
							}
						}
					}
				}
				// onDeliver hook
				if client.server.hooks.OnDeliver != nil {
					client.server.hooks.OnDeliver(context.Background(), client, gmqtt.MessageFromPublish(p))
				}

			case *packets.Puback, *packets.Pubcomp:
				if client.version == packets.Version5 {
					client.addServerQuota()
				}
			case *packets.Pubrec:
				if client.version == packets.Version5 && p.Code >= codes.UnspecifiedError {
					client.addServerQuota()
				}
			}
			err = client.writePacket(packet)
			if err != nil {
				return
			}
			client.server.statsManager.packetSent(packet)
			if _, ok := packet.(*packets.Disconnect); ok {
				client.rwc.Close()
				return
			}
		}
	}
}

func (client *client) writePacket(packet packets.Packet) error {

	if ce := zaplog.Check(zapcore.DebugLevel, "sending packet"); ce != nil {
		ce.Write(
			zap.String("packet", packet.String()),
			zap.String("remote_addr", client.rwc.RemoteAddr().String()),
			zap.String("client_id", client.opts.ClientID),
		)
	}
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
		close(client.in)
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

		if ce := zaplog.Check(zapcore.DebugLevel, "received packet"); ce != nil {
			ce.Write(
				zap.String("packet", packet.String()),
				zap.String("remote_addr", client.rwc.RemoteAddr().String()),
				zap.String("client_id", client.opts.ClientID),
			)
		}

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

// Close closes the client connection. The returned channel will be closed after unregisterClient process has been done
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
	// connack Properties
	var ppt *packets.Properties
	// for enhanced auth
	var onAuth OnAuth
	for {
		select {
		case p := <-client.in:
			if p == nil {
				return
			}
			code := codes.Success
			var authData []byte
			switch p.(type) {
			case *packets.Connect:
				if conn != nil {
					err = codes.ErrProtocol
					return
				}
				if !client.config.AllowZeroLenClientID && len(conn.ClientID) == 0 {
					err = &codes.Error{
						Code: codes.ClientIdentifierNotValid,
					}
					return
				}

				// set default options.
				client.opts.RetainAvailable = client.config.RetainAvailable
				client.opts.WildcardSubAvailable = client.config.WildcardAvailable
				client.opts.SubIDAvailable = client.config.SubscriptionIDAvailable
				client.opts.SharedSubAvailable = client.config.SharedSubAvailable
				client.opts.ServerMaxPacketSize = client.config.MaxPacketSize
				client.opts.SessionExpiry = uint32(client.config.SessionExpiry.Seconds())

				conn = p.(*packets.Connect)
				client.version = conn.Version
				// default connack properties
				ppt = client.defaultConnackProperties(conn)
				if client.version == packets.Version311 || (client.version == packets.Version5 && len(conn.Properties.AuthMethod) != 0) {
					// basic auth
					if srv.hooks.OnBasicAuth != nil {
						resp := srv.hooks.OnBasicAuth(context.Background(), client, &ConnectRequest{
							Connect:                  conn,
							DefaultConnackProperties: ppt,
						})
						code = resp.Code
						if resp.ConnackProperties != nil {
							// override connack properties
							ppt = resp.ConnackProperties
						}
					}
				} else {
					// enhanced auth
					if srv.hooks.OnEnhancedAuth != nil {
						resp := srv.hooks.OnEnhancedAuth(context.Background(), client, &ConnectRequest{
							Connect:                  conn,
							DefaultConnackProperties: ppt,
						})
						code = resp.Code
						authData = resp.AuthData
						onAuth = resp.OnAuth
						if resp.ConnackProperties != nil {
							// override connack properties
							ppt = resp.ConnackProperties
						}
					}
				}
			case *packets.Auth:
				if conn == nil || client.version == packets.Version311 {
					err = codes.ErrProtocol
					return
				}
				if onAuth != nil {
					authResp := onAuth(context.Background(), client, &AuthRequest{
						Auth:                     p.(*packets.Auth),
						DefaultConnackProperties: ppt,
					})
					code = authResp.codes
					authData = authResp.AuthData
					if authResp.ConnackProperties != nil {
						// override connack properties
						ppt = authResp.ConnackProperties
					}
				} else {
					err = codes.ErrProtocol
					return
				}
			}
			// authentication faile
			if code != codes.Success && code != codes.ContinueAuthentication {
				client.out <- &packets.Connack{
					Code:       code,
					Properties: ppt,
				}
				return
			}
			if code == codes.ContinueAuthentication && authData != nil {
				client.out <- &packets.Auth{
					Code: code,
					Properties: &packets.Properties{
						AuthMethod: conn.Properties.AuthMethod,
						AuthData:   authData,
					},
				}
				continue
			}
			// code = codes.Success
			if client.version == packets.Version5 {
				client.opts.RetainAvailable = byte2bool(ppt.RetainAvailable)
				client.opts.WildcardSubAvailable = byte2bool(ppt.WildcardSubAvailable)
				client.opts.SubIDAvailable = byte2bool(ppt.SubIDAvailable)
				client.opts.SharedSubAvailable = byte2bool(ppt.SharedSubAvailable)
				client.opts.SessionExpiry = convertUint32(ppt.SessionExpiryInterval, 0)

				client.opts.ClientReceiveMax = convertUint16(conn.Properties.ReceiveMaximum, 65535)
				client.opts.ServerReceiveMax = convertUint16(ppt.ReceiveMaximum, 65535)

				client.opts.ClientMaxPacketSize = convertUint32(conn.Properties.MaximumPacketSize, 0)
				client.opts.ServerMaxPacketSize = convertUint32(ppt.MaximumPacketSize, 0)

				client.opts.ClientTopicAliasMax = convertUint16(conn.Properties.TopicAliasMaximum, 0)
				client.opts.ServerTopicAliasMax = convertUint16(ppt.TopicAliasMaximum, 0)

				client.opts.AuthMethod = conn.Properties.AuthMethod

				client.clientReceiveMaximumQuota = client.opts.ClientReceiveMax
				client.serverReceiveMaximumQuota = client.opts.ServerReceiveMax

				client.aliasMapper = make([][]byte, client.opts.ServerReceiveMax+1)

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

				recvMax := int(convertUint16(ppt.ReceiveMaximum, math.MaxUint16))
				if recvMax >= client.config.MaxInflight {
					client.config.MaxInflight = recvMax
				}
				if recvMax >= client.config.MaxAwaitRel {
					client.config.MaxAwaitRel = recvMax
				}

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
			client.opts.Username = string(conn.Username)
			client.opts.Will.Flag = conn.WillFlag
			client.opts.Will.Payload = conn.WillMsg
			client.opts.Will.Qos = conn.WillQos
			client.opts.Will.Retain = conn.WillRetain
			client.opts.Will.Topic = conn.WillTopic
			client.opts.Will.Properties = conn.WillProperties
			client.server.registerClient(conn, ppt, client)
			return
		case <-timeout.C:
			err = ErrConnectTimeOut
			return
		}
	}
}

func errDetailsToProperties(errDetails *codes.ErrorDetails) *packets.Properties {
	return &packets.Properties{
		ReasonString: errDetails.ReasonString,
		User:         kvsToProperties(errDetails.UserProperties),
	}
}
func (client *client) defaultConnackProperties(connect *packets.Connect) *packets.Properties {
	if connect.Version != packets.Version5 {
		return nil
	}
	ppt := &packets.Properties{
		SessionExpiryInterval: connect.Properties.SessionExpiryInterval,
		ReceiveMaximum:        &client.config.ReceiveMax,
		MaximumQoS:            &client.config.MaximumQoS,
		RetainAvailable:       bool2Byte(client.config.RetainAvailable),
		TopicAliasMaximum:     &client.config.TopicAliasMax,
		WildcardSubAvailable:  bool2Byte(client.config.WildcardAvailable),
		SubIDAvailable:        bool2Byte(client.config.SubscriptionIDAvailable),
		SharedSubAvailable:    bool2Byte(client.config.SharedSubAvailable),
	}
	if client.config.MaxPacketSize != 0 {
		ppt.MaximumPacketSize = &client.config.MaxPacketSize
	}

	if i := ppt.SessionExpiryInterval; i != nil && *i < uint32(client.config.SessionExpiry.Seconds()) {
		ppt.SessionExpiryInterval = i
	}
	if connect.KeepAlive > client.config.MaxKeepAlive {
		ppt.ServerKeepAlive = &client.config.MaxKeepAlive
	}

	return ppt
}

func (client *client) newSession() {
	s := &session{
		unackpublish: make(map[packets.PacketID]bool),
		inflight:     list.New(),
		awaitRel:     list.New(),
		msgQueue:     list.New(),
		lockedPid:    make(map[packets.PacketID]bool),
		freePid:      1,
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
	case client.out <- publish:
	default:
		// in case client.out has been closed
		return
	}
}

func (client *client) publish(publish *packets.Publish) {
	if client.IsConnected() {
		client.onlinePublish(publish)
	} else {
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
// test case:
// 1. 有request problem info的话，可以返回string和User，否则不返回
func (client *client) subscribeHandler(sub *packets.Subscribe) *codes.Error {
	srv := client.server
	suback := &packets.Suback{
		Version:    sub.Version,
		PacketID:   sub.PacketID,
		Properties: nil,
		Payload:    make([]codes.Code, len(sub.Topics)),
	}
	var topics []packets.Topic
	var subID uint32
	if client.version == packets.Version5 {
		if client.opts.SubIDAvailable && len(sub.Properties.SubscriptionIdentifier) != 0 {
			subID = sub.Properties.SubscriptionIdentifier[0]
		}
		if !client.config.SubscriptionIDAvailable && subID != 0 {
			return &codes.Error{
				Code: codes.SubIDNotSupported,
			}
		}
	}

	if srv.hooks.OnSubscribe != nil {
		resp, errDetails := srv.hooks.OnSubscribe(context.Background(), client, sub)
		topics = resp.Topics
		if resp.ID != nil {
			subID = *resp.ID
		}
		if errDetails != nil && client.version == packets.Version5 {
			if client.opts.RequestProblemInfo {
				suback.Properties = &packets.Properties{
					ReasonString: errDetails.ReasonString,
					User:         kvsToProperties(errDetails.UserProperties),
				}
			}
		}
	} else {
		topics = sub.Topics
	}

	for k, v := range topics {
		var isShared bool
		var sub *gmqtt.Subscription
		code := v.Qos
		sub = subscription.FromTopic(v, subID)
		if client.version == packets.Version5 {
			if sub.ShareName != "" {
				isShared = true
				if !client.opts.SharedSubAvailable {
					code = codes.SharedSubNotSupported
				}
			}
			if !client.opts.SubIDAvailable && subID != 0 {
				code = codes.SubIDNotSupported
			}
			if !client.opts.WildcardSubAvailable {
				for _, c := range sub.TopicFilter {
					if c == '+' || c == '#' {
						code = codes.WildcardSubNotSupported
						break
					}
				}
			}
		}
		suback.Payload[k] = code
		if code < packets.SubscribeFailure {
			subRs := srv.subscriptionsDB.Subscribe(client.opts.ClientID, sub)
			if srv.hooks.OnSubscribed != nil {
				srv.hooks.OnSubscribed(context.Background(), client, sub)
			}
			zaplog.Info("subscribe succeeded",
				zap.String("topic", v.Name),
				zap.Uint8("qos", v.Qos),
				zap.String("client_id", client.opts.ClientID),
				zap.String("remote_addr", client.rwc.RemoteAddr().String()),
			)
			// The spec does not specify whether the retain message should follow the 'no-local' option rule.
			// Gmqtt follows the mosquitto implementation which will send retain messages to no-local subscriptions.
			// For details: https://github.com/eclipse/mosquitto/issues/1796
			if !isShared && ((!subRs[0].AlreadyExisted && v.RetainHandling != 2) || v.RetainHandling == 0) {
				msgs := srv.retainedDB.GetMatchedMessages(sub.TopicFilter)
				for _, v := range msgs {
					publish := gmqtt.MessageToPublish(v, client.version)
					if publish.Qos > subRs[0].Subscription.QoS {
						publish.Qos = subRs[0].Subscription.QoS
					}
					publish.Dup = false
					if !sub.RetainAsPublished {
						publish.Retain = false
					}
					client.publishMessageHandler(publish)
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

func (client *client) publishHandler(pub *packets.Publish) *codes.Error {
	s := client.session
	srv := client.server
	var dup bool

	// check retain available
	if !client.opts.RetainAvailable && pub.Retain {
		return &codes.Error{
			Code: codes.RetainNotSupported,
		}
	}
	if client.version == packets.Version5 && pub.Properties.TopicAlias != nil {
		if *pub.Properties.TopicAlias >= client.opts.ServerTopicAliasMax {
			return &codes.Error{
				Code: codes.TopicAliasInvalid,
			}
		} else {
			topicAlias := *pub.Properties.TopicAlias
			name := client.aliasMapper[int(topicAlias)]
			if len(pub.TopicName) == 0 {
				if len(name) == 0 {
					return &codes.Error{
						Code: codes.TopicAliasInvalid,
					}
				}
				pub.TopicName = name
			} else {
				client.aliasMapper[topicAlias] = pub.TopicName
			}
		}
	}

	if pub.Qos == packets.Qos2 {
		if _, ok := s.unackpublish[pub.PacketID]; ok {
			dup = true
		} else {
			s.unackpublish[pub.PacketID] = true
		}
	}

	var msg *gmqtt.Message
	msg = gmqtt.MessageFromPublish(pub)
	if pub.Retain {
		if len(pub.Payload) == 0 {
			srv.retainedDB.Remove(string(pub.TopicName))
		} else {
			srv.retainedDB.AddOrReplace(msg)
		}
	}

	var err error
	var topicMatched bool
	if !dup {
		if srv.hooks.OnMsgArrived != nil {
			msg, err = srv.hooks.OnMsgArrived(context.Background(), client, pub)
		}
		if msg != nil {
			srv.mu.Lock()
			topicMatched = srv.deliverMessageHandler(client.opts.ClientID, "", msg)
			srv.mu.Unlock()
		}
	}

	var ack packets.Packet
	// ack properties
	var ppt *packets.Properties
	code := codes.Success
	if client.version == packets.Version5 {
		if !topicMatched {
			code = codes.NotMatchingSubscribers
		}
		if err != nil {
			if codeErr, ok := err.(*codes.Error); ok {
				ppt = errDetailsToProperties(&codeErr.ErrorDetails)
				code = codeErr.Code
			} else {
				ppt = &packets.Properties{
					ReasonString: []byte(err.Error()),
				}
				code = codes.UnspecifiedError
			}
		}
	}
	if pub.Qos == packets.Qos1 {
		ack = pub.NewPuback(code, ppt)
	}
	if pub.Qos == packets.Qos2 {
		ack = pub.NewPubrec(code, ppt)
		if code >= codes.UnspecifiedError {
			delete(s.unackpublish, pub.PacketID)
		}
	}
	if ack != nil {
		client.write(ack)
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
	delete(client.session.unackpublish, pubrel.PacketID)
	pubcomp := pubrel.NewPubcomp()
	client.write(pubcomp)
}
func (client *client) pubrecHandler(pubrec *packets.Pubrec) {
	client.unsetInflight(pubrec)
	if client.version == packets.Version5 && pubrec.Code >= codes.UnspecifiedError {
		client.addClientQuota()
		return
	}
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
	unSuback := &packets.Unsuback{
		Version:    unSub.Version,
		PacketID:   unSub.PacketID,
		Properties: nil,
		Payload:    make([]codes.Code, len(unSub.Topics)),
	}
	topics := unSub.Topics
	if srv.hooks.OnUnSubscribe != nil {
		var errDetails *codes.ErrorDetails
		topics, errDetails = srv.hooks.OnUnSubscribe(context.Background(), client, unSub)
		if client.version == packets.Version5 && client.opts.RequestProblemInfo && errDetails != nil {
			unSuback.Properties = errDetailsToProperties(errDetails)
		}
	}
	for k, topicName := range topics {
		srv.subscriptionsDB.Unsubscribe(client.opts.ClientID, topicName)
		if srv.hooks.OnUnsubscribed != nil {
			srv.hooks.OnUnsubscribed(context.Background(), client, topicName)
		}
		zaplog.Info("unsubscribed",
			zap.String("topic", topicName),
			zap.String("client_id", client.opts.ClientID),
			zap.String("remote_addr", client.rwc.RemoteAddr().String()),
		)
		if client.version == packets.Version5 {
			unSuback.Payload[k] = codes.Success
		}
	}
	client.write(unSuback)

}

func (client *client) reAuthHandler(auth *packets.Auth) *codes.Error {
	srv := client.server
	// default code
	code := codes.Success
	var resp *ReAuthResponse
	var err error
	if srv.hooks.OnReAuth != nil {
		resp, err = srv.hooks.OnReAuth(context.Background(), client, auth.Properties.AuthData)
		if err != nil {
			if c, ok := err.(*codes.Error); ok {
				return c
			} else {
				return codes.NewError(codes.UnspecifiedError)
			}
		}
	} else {
		return codes.ErrProtocol
	}
	if resp.Continue {
		code = codes.ContinueAuthentication
	}
	client.write(&packets.Auth{
		Code: code,
		Properties: &packets.Properties{
			AuthMethod: client.opts.AuthMethod,
			AuthData:   resp.AuthData,
		},
	})
	return nil

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
		close(client.close)
	}()
	for packet := range client.in {
		if client.version == packets.Version5 {
			if client.opts.ServerMaxPacketSize != 0 && packets.TotalBytes(packet) > client.opts.ServerMaxPacketSize {
				err = codes.NewError(codes.PacketTooLarge)
				return
			}
		}
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
			dis := packet.(*packets.Disconnect)
			if client.version == packets.Version5 {
				disExpiry := convertUint32(dis.Properties.SessionExpiryInterval, 0)
				if client.opts.SessionExpiry == 0 && disExpiry != 0 {
					err = codes.NewError(codes.ProtocolError)
					return
				}
				if disExpiry != 0 {
					client.opts.SessionExpiry = disExpiry
				}
			}
			//正常关闭
			client.cleanWillFlag = true
			return
		case *packets.Auth:
			auth := packet.(*packets.Auth)
			if client.version != packets.Version5 {
				err = codes.ErrProtocol
				return
			}
			if !bytes.Equal(client.opts.AuthMethod, auth.Properties.AuthData) {
				err = codes.ErrProtocol
				return
			}
			err = client.reAuthHandler(auth)

		default:
			err = codes.ErrProtocol
		}
		if err != nil {
			client.setError(err)
			return
		}
	}

}

//server goroutine结束的条件:1客户端断开连接 或 2发生错误
func (client *client) serve() {
	defer client.internalClose()
	client.wg.Add(2)
	go client.readLoop()                       //read
	go client.writeLoop()                      //write
	if ok := client.connectWithTimeOut(); ok { //链接成功,建立session
		client.wg.Add(1)
		go client.readHandle()
	}
	client.wg.Wait()
	client.rwc.Close()
}

func (client *client) setCleanWillFlag(disconnect *packets.Disconnect) error {
	if client.version == packets.Version5 {
		disExpiry := convertUint32(disconnect.Properties.SessionExpiryInterval, 0)
		if client.opts.SessionExpiry == 0 && disExpiry != 0 {
			return codes.NewError(codes.ProtocolError)
		}
		if disExpiry != 0 {
			client.opts.SessionExpiry = disExpiry
		}
	}
	// 不发送will message关闭
	client.cleanWillFlag = true
	return nil
}