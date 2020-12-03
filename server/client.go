// Package gmqtt provides an MQTT v3.1.1 server library.
package server

import (
	"bufio"
	"bytes"
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
	"github.com/DrmagicE/gmqtt/config"
	"github.com/DrmagicE/gmqtt/persistence/queue"
	"github.com/DrmagicE/gmqtt/persistence/subscription"
	"github.com/DrmagicE/gmqtt/persistence/unack"
	"github.com/DrmagicE/gmqtt/pkg/codes"
	"github.com/DrmagicE/gmqtt/pkg/packets"
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

	// Client 最多能同时接受多少 Server发过去的message。Server发送要限速
	// Inflightmessage
	ClientReceiveMax uint16
	// Server 最多能同时接受多少 Client发过来的message，Client要限速
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

// Client represent a mqtt client.
type Client interface {
	// ClientOptions return a reference of ClientOptions. Do not edit.
	// This is mainly used in callback functions.
	ClientOptions() *ClientOptions
	// Version return the protocol version of the used client.
	Version() packets.Version
	// ConnectedAt returns the connected time
	ConnectedAt() time.Time
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

	error         chan error //错误
	errOnce       sync.Once
	err           error
	opts          *ClientOptions //OnConnect之前填充,set up before OnConnect()
	cleanWillFlag bool           //收到DISCONNECT报文删除遗嘱标志, whether to remove will Msg
	disconnect    *packets.Disconnect

	connectedAt    int64
	disconnectedAt int64

	statsManager      SessionStatsManager
	topicAliasManager TopicAliasManager
	version           packets.Version
	aliasMapper       [][]byte

	// gard serverReceiveMaximumQuota
	serverQuotaMu             sync.Mutex
	serverReceiveMaximumQuota uint16

	config config.Config

	queueStore queue.Store
	unackStore unack.Store
	pl         *packetIDLimiter
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

//Status returns client's status
func (client *client) Status() int32 {
	return atomic.LoadInt32(&client.status)
}

// IsConnected returns whether the client is connected or not.
func (client *client) IsConnected() bool {
	return client.Status() == Connected
}

func (client *client) setError(err error) {
	client.errOnce.Do(func() {
		if client.queueStore != nil {
			qerr := client.queueStore.Close()
			if qerr != nil {
				zaplog.Error("fail to close message queue", zap.String("client_id", client.opts.ClientID), zap.Error(qerr))
			}
		}
		if client.pl != nil {
			client.pl.close()
		}

		if err != nil && err != io.EOF {
			zaplog.Error("connection lost", zap.Error(err))
			client.err = err
			if client.version == packets.Version5 {
				if code, ok := err.(*codes.Error); ok {
					if client.IsConnected() {
						// send Disconnect
						client.write(&packets.Disconnect{
							Version: packets.Version5,
							Code:    code.Code,
							Properties: &packets.Properties{
								ReasonString: code.ReasonString,
								User:         kvsToProperties(code.UserProperties),
							},
						})
					}
				}
			}
		}
	})
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
						if alias, ok := client.topicAliasManager.Check(p); ok {
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
			if client.version == packets.Version5 && pub.Qos > packets.Qos0 {
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
	// if any error occur, this function should set the error to the client and return false
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
	var authOpts *AuthOptions
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
				if !client.config.MQTT.AllowZeroLenClientID && len(conn.ClientID) == 0 {
					err = &codes.Error{
						Code: codes.ClientIdentifierNotValid,
					}
					return
				}

				conn = p.(*packets.Connect)
				client.version = conn.Version
				// default auth options
				authOpts = client.defaultAuthOptions(conn)
				// set default options.
				client.opts.RetainAvailable = authOpts.RetainAvailable
				client.opts.WildcardSubAvailable = authOpts.WildcardSubAvailable
				client.opts.SubIDAvailable = authOpts.SubIDAvailable
				client.opts.SharedSubAvailable = authOpts.SharedSubAvailable
				client.opts.SessionExpiry = authOpts.SessionExpiry

				client.opts.ClientReceiveMax = math.MaxUint16 // unlimited
				client.opts.ServerReceiveMax = authOpts.ReceiveMax

				client.opts.ClientMaxPacketSize = math.MaxUint32 // unlimited
				client.opts.ServerMaxPacketSize = authOpts.MaxPacketSize

				client.opts.ServerTopicAliasMax = authOpts.TopicAliasMax

				if conn.Properties == nil || len(conn.Properties.AuthMethod) == 0 {
					// basic auth
					if srv.hooks.OnBasicAuth != nil {
						err = srv.hooks.OnBasicAuth(context.Background(), client, &ConnectRequest{
							Connect: conn,
							Options: authOpts,
						})
						if err != nil {
							code = codes.Success
						}
					}
				} else {
					// enhanced auth
					if srv.hooks.OnEnhancedAuth != nil {
						var resp *EnhancedAuthResponse
						resp, err = srv.hooks.OnEnhancedAuth(context.Background(), client, &ConnectRequest{
							Connect: conn,
							Options: authOpts,
						})
						if err != nil {
							if resp.Continue {
								code = codes.ContinueAuthentication
							} else {
								code = codes.Success
							}
							authData = resp.AuthData
							onAuth = resp.OnAuth
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
						Auth:    p.(*packets.Auth),
						Options: authOpts,
					})
					if authResp.Continue {
						code = codes.ContinueAuthentication
					}
					authData = authResp.AuthData
				} else {
					err = codes.ErrProtocol
					return
				}
			}
			// authentication faile
			if err != nil {
				codeErr := converError(err)
				client.out <- &packets.Connack{
					Version:    client.version,
					Code:       codeErr.Code,
					Properties: getErrorProperties(client, &codeErr.ErrorDetails),
				}
				return
			}
			// authentication success
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

			client.opts.RetainAvailable = authOpts.RetainAvailable
			client.opts.WildcardSubAvailable = authOpts.WildcardSubAvailable
			client.opts.SubIDAvailable = authOpts.SubIDAvailable
			client.opts.SharedSubAvailable = authOpts.SharedSubAvailable
			client.opts.SessionExpiry = authOpts.SessionExpiry

			var connackPpt *packets.Properties
			if client.version == packets.Version5 {
				client.opts.ClientReceiveMax = convertUint16(conn.Properties.ReceiveMaximum, client.opts.ClientReceiveMax)
				client.opts.ServerReceiveMax = authOpts.ReceiveMax

				client.opts.ClientMaxPacketSize = convertUint32(conn.Properties.MaximumPacketSize, client.opts.ClientMaxPacketSize)
				client.opts.ServerMaxPacketSize = authOpts.MaxPacketSize

				client.opts.ClientTopicAliasMax = convertUint16(conn.Properties.TopicAliasMaximum, client.opts.ClientTopicAliasMax)
				client.opts.ServerTopicAliasMax = authOpts.TopicAliasMax

				client.opts.AuthMethod = conn.Properties.AuthMethod
				client.serverReceiveMaximumQuota = client.opts.ServerReceiveMax
				client.aliasMapper = make([][]byte, client.opts.ServerReceiveMax+1)

				if len(conn.ClientID) == 0 {
					if len(authOpts.AssignedClientID) != 0 {
						client.opts.ClientID = string(authOpts.AssignedClientID)
					} else {
						client.opts.ClientID = getRandomUUID()
						authOpts.AssignedClientID = []byte(client.opts.ClientID)
					}
				} else {
					client.opts.ClientID = string(conn.ClientID)
				}
				client.opts.KeepAlive = authOpts.KeepAlive
				connackPpt = &packets.Properties{
					SessionExpiryInterval: &authOpts.SessionExpiry,
					ReceiveMaximum:        &authOpts.ReceiveMax,
					MaximumQoS:            &authOpts.MaximumQoS,
					RetainAvailable:       bool2Byte(authOpts.RetainAvailable),
					TopicAliasMaximum:     &authOpts.TopicAliasMax,
					WildcardSubAvailable:  bool2Byte(authOpts.WildcardSubAvailable),
					SubIDAvailable:        bool2Byte(authOpts.SubIDAvailable),
					SharedSubAvailable:    bool2Byte(authOpts.SharedSubAvailable),
					MaximumPacketSize:     &authOpts.MaxPacketSize,
					ServerKeepAlive:       &authOpts.KeepAlive,
					AssignedClientID:      authOpts.AssignedClientID,
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

			client.newPacketIDLimiter(client.opts.ClientReceiveMax)

			err = client.server.registerClient(conn, connackPpt, client)
			return
		case <-timeout.C:
			err = ErrConnectTimeOut
			return
		}
	}
}

func getErrorProperties(client *client, errDetails *codes.ErrorDetails) *packets.Properties {
	if client.version == packets.Version5 && client.opts.RequestProblemInfo && errDetails != nil {
		return &packets.Properties{
			ReasonString: errDetails.ReasonString,
			User:         kvsToProperties(errDetails.UserProperties),
		}
	}
	return nil
}

func setErrorProperties(client *client, errDetails *codes.ErrorDetails, ppt *packets.Properties) {
	if client.version == packets.Version5 && client.opts.RequestProblemInfo && errDetails != nil {
		ppt.ReasonString = errDetails.ReasonString
		ppt.User = kvsToProperties(errDetails.UserProperties)
	}
}

func (client *client) defaultAuthOptions(connect *packets.Connect) *AuthOptions {
	opts := &AuthOptions{
		SessionExpiry:        uint32(client.config.MQTT.SessionExpiry.Seconds()),
		ReceiveMax:           client.config.MQTT.ReceiveMax,
		MaximumQoS:           client.config.MQTT.MaximumQoS,
		MaxPacketSize:        client.config.MQTT.MaxPacketSize,
		TopicAliasMax:        client.config.MQTT.TopicAliasMax,
		RetainAvailable:      client.config.MQTT.RetainAvailable,
		WildcardSubAvailable: client.config.MQTT.WildcardAvailable,
		SubIDAvailable:       client.config.MQTT.SubscriptionIDAvailable,
		SharedSubAvailable:   client.config.MQTT.SharedSubAvailable,
		KeepAlive:            client.config.MQTT.MaxKeepAlive,
		AssignedClientID:     make([]byte, 0, len(connect.ClientID)),
	}
	copy(opts.AssignedClientID, connect.ClientID)
	if connect.KeepAlive < opts.KeepAlive {
		opts.KeepAlive = connect.KeepAlive
	}
	if client.version == packets.Version5 {
		if i := connect.Properties.SessionExpiryInterval; i != nil && *i < opts.SessionExpiry {
			opts.SessionExpiry = *i
		}
	}
	return opts
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

func (client *client) checkMaxPacketSize(msg *gmqtt.Message) (valid bool) {
	totalBytes := msg.TotalBytes(packets.Version5)
	if client.opts.ClientMaxPacketSize != 0 && totalBytes > client.opts.ClientMaxPacketSize {
		return false
	}
	return true
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
	now := time.Now()
	if client.version == packets.Version5 {
		if client.opts.SubIDAvailable && len(sub.Properties.SubscriptionIdentifier) != 0 {
			subID = sub.Properties.SubscriptionIdentifier[0]
		}
		if !client.config.MQTT.SubscriptionIDAvailable && subID != 0 {
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

		var subRs subscription.SubscribeResult
		var subErr error
		if code < packets.SubscribeFailure {
			subRs, subErr = srv.subscriptionsDB.Subscribe(client.opts.ClientID, sub)
			if subErr != nil {
				zaplog.Error("failed to subscribe topic",
					zap.String("topic", v.Name),
					zap.Uint8("qos", v.Qos),
					zap.String("client_id", client.opts.ClientID),
					zap.String("remote_addr", client.rwc.RemoteAddr().String()),
					zap.Error(subErr))
				code = packets.SubscribeFailure
			}
		}
		suback.Payload[k] = code
		if code < packets.SubscribeFailure {
			if srv.hooks.OnSubscribed != nil {
				srv.hooks.OnSubscribed(context.Background(), client, sub)
			}

			zaplog.Info("subscribe succeeded",
				zap.String("topic", sub.TopicFilter),
				zap.Uint8("qos", sub.QoS),
				zap.Uint8("retain_handling", sub.RetainHandling),
				zap.Bool("retain_as_published", sub.RetainAsPublished),
				zap.Bool("no_local", sub.NoLocal),
				zap.Uint32("id", sub.ID),
				zap.String("client_id", client.opts.ClientID),
				zap.String("remote_addr", client.rwc.RemoteAddr().String()),
			)
			// The spec does not specify whether the retain message should follow the 'no-local' option rule.
			// Gmqtt follows the mosquitto implementation which will send retain messages to no-local subscriptions.
			// For details: https://github.com/eclipse/mosquitto/issues/1796
			if !isShared && ((!subRs[0].AlreadyExisted && v.RetainHandling != 2) || v.RetainHandling == 0) {
				msgs := srv.retainedDB.GetMatchedMessages(sub.TopicFilter)
				for _, v := range msgs {
					if v.QoS > subRs[0].Subscription.QoS {
						v.QoS = subRs[0].Subscription.QoS
					}
					v.Dup = false
					if !sub.RetainAsPublished {
						v.Retained = false
					}
					var expiry time.Time
					if v.MessageExpiry != 0 {
						expiry = now.Add(time.Second * time.Duration(v.MessageExpiry))
					}
					err := client.queueStore.Add(&queue.Elem{
						At:     now,
						Expiry: expiry,
						MessageWithID: &queue.Publish{
							Message: v,
						},
					})
					if err != nil {
						queue.Drop(srv.hooks.OnMsgDropped, zaplog, client.opts.ClientID, v, &queue.InternalError{Err: err})
						if codesErr, ok := err.(*codes.Error); ok {
							return codesErr
						}
						return &codes.Error{
							Code: codes.UnspecifiedError,
						}
					}
				}
			}
		} else {
			zaplog.Info("subscribe failed",
				zap.String("topic", sub.TopicFilter),
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
	srv := client.server
	var dup bool

	// check retain available
	if !client.opts.RetainAvailable && pub.Retain {
		return &codes.Error{
			Code: codes.RetainNotSupported,
		}
	}
	var msg *gmqtt.Message
	msg = gmqtt.MessageFromPublish(pub)

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
				msg.Topic = string(name)
			} else {
				client.aliasMapper[topicAlias] = pub.TopicName
			}
		}
	}

	if pub.Qos == packets.Qos2 {
		exist, err := client.unackStore.Set(pub.PacketID)
		if err != nil {
			return converError(err)
		}
		if exist {
			dup = true
		}
	}

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
		// 查询订阅，得到结果
		// 根据结果，发送到不同队列
		if msg != nil {
			srv.mu.Lock()
			topicMatched = srv.deliverMessageHandler(client.opts.ClientID, msg)
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
				ppt = getErrorProperties(client, &codeErr.ErrorDetails)
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
			err = client.unackStore.Remove(pub.PacketID)
			if err != nil {
				return converError(err)
			}
		}
	}
	if ack != nil {
		client.write(ack)
	}
	return nil

}

func converError(err error) *codes.Error {
	if err == nil {
		return nil
	}
	if e, ok := err.(*codes.Error); ok {
		return e
	}
	return &codes.Error{
		Code: codes.UnspecifiedError,
		ErrorDetails: codes.ErrorDetails{
			ReasonString: []byte(err.Error()),
		},
	}
}

func (client *client) pubackHandler(puback *packets.Puback) *codes.Error {
	err := client.queueStore.Remove(puback.PacketID)
	if err != nil {
		return converError(err)
	}
	client.pl.release(puback.PacketID)
	client.statsManager.decInflightCurrent(1)
	if ce := zaplog.Check(zapcore.DebugLevel, "unset inflight"); ce != nil {
		ce.Write(zap.String("clientID", client.opts.ClientID),
			zap.Uint16("pid", puback.PacketID),
		)
	}
	return nil
}
func (client *client) pubrelHandler(pubrel *packets.Pubrel) *codes.Error {
	err := client.unackStore.Remove(pubrel.PacketID)
	if err != nil {
		return converError(err)
	}
	pubcomp := pubrel.NewPubcomp()
	client.write(pubcomp)
	return nil
}
func (client *client) pubrecHandler(pubrec *packets.Pubrec) {
	// 从queue中replace
	if client.version == packets.Version5 && pubrec.Code >= codes.UnspecifiedError {
		err := client.queueStore.Remove(pubrec.PacketID)
		client.pl.release(pubrec.PacketID)
		if err != nil {
			client.setError(err)
		}
		return
	}
	pubrel := pubrec.NewPubrel()
	_, err := client.queueStore.Replace(&queue.Elem{
		At: time.Now(),
		MessageWithID: &queue.Pubrel{
			PacketID: pubrel.PacketID,
		}})
	if err != nil {
		client.setError(err)
	}
	client.write(pubrel)
}
func (client *client) pubcompHandler(pubcomp *packets.Pubcomp) {
	// 从queue中delete
	err := client.queueStore.Remove(pubcomp.PacketID)
	client.pl.release(pubcomp.PacketID)
	if err != nil {
		client.setError(err)
	}

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
	}
	cs := make([]codes.Code, len(unSub.Topics))
	topics := unSub.Topics
	if srv.hooks.OnUnSubscribe != nil {
		resp, errDetails := srv.hooks.OnUnSubscribe(context.Background(), client, unSub)
		if client.version == packets.Version5 && client.opts.RequestProblemInfo && errDetails != nil {
			unSuback.Properties = getErrorProperties(client, errDetails)
		}
		cs = resp.Code
	}
	for k, topicName := range topics {
		if cs[k] == codes.Success {
			err := srv.subscriptionsDB.Unsubscribe(client.opts.ClientID, topicName)
			if err != nil {

			}
			if srv.hooks.OnUnsubscribed != nil {
				srv.hooks.OnUnsubscribed(context.Background(), client, topicName)
			}
			zaplog.Info("unsubscribed succeed",
				zap.String("topic", topicName),
				zap.String("client_id", client.opts.ClientID),
				zap.String("remote_addr", client.rwc.RemoteAddr().String()),
			)
		} else {
			zaplog.Info("unsubscribed failed",
				zap.String("topic", topicName),
				zap.String("client_id", client.opts.ClientID),
				zap.String("remote_addr", client.rwc.RemoteAddr().String()),
				zap.Uint8("code", unSuback.Payload[k]))
		}
	}
	if client.version == packets.Version5 {
		unSuback.Payload = cs
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

func (client *client) disconnectHandler(dis *packets.Disconnect) *codes.Error {
	if client.version == packets.Version5 {
		disExpiry := convertUint32(dis.Properties.SessionExpiryInterval, 0)
		sess, err := client.server.sessionStore.Get(client.opts.ClientID)
		if err != nil {
			return &codes.Error{
				Code: codes.UnspecifiedError,
				ErrorDetails: codes.ErrorDetails{
					ReasonString: []byte(err.Error()),
				},
			}
		}
		if sess.ExpiryInterval == 0 && disExpiry != 0 {
			return &codes.Error{
				Code: codes.ProtocolError,
			}
		}
		if disExpiry != 0 {
			err := client.server.sessionStore.SetSessionExpiry(sess.ClientID, disExpiry)
			if err != nil {
				zaplog.Error("fail to set session expiry",
					zap.String("client_id", client.opts.ClientID),
					zap.Error(err))
			}
		}
	}
	client.disconnect = dis
	// 不发送will message
	client.cleanWillFlag = true
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
		var codeErr *codes.Error
		switch packet.(type) {
		case *packets.Subscribe:
			codeErr = client.subscribeHandler(packet.(*packets.Subscribe))
		case *packets.Publish:
			codeErr = client.publishHandler(packet.(*packets.Publish))
		case *packets.Puback:
			codeErr = client.pubackHandler(packet.(*packets.Puback))
		case *packets.Pubrel:
			codeErr = client.pubrelHandler(packet.(*packets.Pubrel))
		case *packets.Pubrec:
			client.pubrecHandler(packet.(*packets.Pubrec))
		case *packets.Pubcomp:
			client.pubcompHandler(packet.(*packets.Pubcomp))
		case *packets.Pingreq:
			client.pingreqHandler(packet.(*packets.Pingreq))
		case *packets.Unsubscribe:
			client.unsubscribeHandler(packet.(*packets.Unsubscribe))
		case *packets.Disconnect:
			codeErr = client.disconnectHandler(packet.(*packets.Disconnect))
			return
		case *packets.Auth:
			auth := packet.(*packets.Auth)
			if client.version != packets.Version5 {
				err = codes.ErrProtocol
				return
			}
			if !bytes.Equal(client.opts.AuthMethod, auth.Properties.AuthData) {
				codeErr = codes.ErrProtocol
				return
			}
			codeErr = client.reAuthHandler(auth)

		default:
			err = codes.ErrProtocol
		}
		if codeErr != nil {
			err = codeErr
			return
		}
	}

}

func (client *client) newPacketIDLimiter(limit uint16) {
	client.pl = &packetIDLimiter{
		cond:      sync.NewCond(&sync.Mutex{}),
		used:      0,
		limit:     limit,
		exit:      false,
		freePid:   1,
		lockedPid: make(map[packets.PacketID]bool),
	}
}

func (client *client) pollInflights() (cont bool, err error) {
	var elems []*queue.Elem
	elems, err = client.queueStore.ReadInflight(uint(client.opts.ServerReceiveMax))
	if err != nil || len(elems) == 0 {
		return false, err
	}
	client.pl.lock()
	defer client.pl.unlock()
	for _, v := range elems {
		id := v.MessageWithID.ID()
		switch m := v.MessageWithID.(type) {
		case *queue.Publish:
			m.Dup = true
			// https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Subscription_Options
			// The Server need not use the same set of Subscription Identifiers in the retransmitted PUBLISH packet.
			m.SubscriptionIdentifier = nil
			client.pl.markUsedLocked(id)
			client.write(gmqtt.MessageToPublish(m.Message, client.version))
		case *queue.Pubrel:
			client.write(&packets.Pubrel{PacketID: id})
		}
	}

	return true, nil
}

func (client *client) pollNewMessages(ids []packets.PacketID) (unused []packets.PacketID, err error) {
	now := time.Now()
	var elems []*queue.Elem
	elems, err = client.queueStore.Read(ids)
	if err != nil {
		return nil, err
	}
	for _, v := range elems {
		switch m := v.MessageWithID.(type) {
		case *queue.Publish:
			if m.QoS != packets.Qos0 {
				ids = ids[1:]
			}
			if client.version == packets.Version5 && m.Message.MessageExpiry != 0 {
				d := uint32(now.Sub(v.At).Seconds())
				m.Message.MessageExpiry = d
			}
			client.write(gmqtt.MessageToPublish(m.Message, client.version))
		case *queue.Pubrel:
		}
	}
	return ids, err
}
func (client *client) pollMessageHandler() {
	var err error
	defer func() {
		if re := recover(); re != nil {
			err = errors.New(fmt.Sprint(re))
		}
		client.setError(err)
		client.wg.Done()
	}()
	// drain all inflight messages
	cont := true
	for cont {
		cont, err = client.pollInflights()
		if err != nil {
			return
		}
	}
	var ids []packets.PacketID
	for {
		//TODO config
		ids = client.pl.pollPacketIDs(100)
		if ids == nil {
			return
		}
		ids, err = client.pollNewMessages(ids)
		if err != nil {
			return
		}
		client.pl.batchRelease(ids)
	}
}

//server goroutine结束的条件:1客户端断开连接 或 2发生错误
func (client *client) serve() {
	defer client.internalClose()
	client.wg.Add(2)
	go client.readLoop()                       //read
	go client.writeLoop()                      //write
	if ok := client.connectWithTimeOut(); ok { //链接成功,建立session
		client.wg.Add(2)
		go client.pollMessageHandler()
		go client.readHandle()
	}
	client.wg.Wait()
	client.rwc.Close()
}
