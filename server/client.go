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
	"reflect"
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
	"github.com/DrmagicE/gmqtt/pkg/bitmap"
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
)

const (
	readBufferSize  = 1024
	writeBufferSize = 1024
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

// ClientOptions is the options which controls how the server interacts with the client.
// It will be set after the client has connected.
type ClientOptions struct {
	// ClientID is the client id for the client.
	ClientID string
	// Username is the username for the client.
	Username string
	// KeepAlive is the keep alive time in seconds for the client.
	// The server will close the client if no there is no packet has been received for 1.5 times the KeepAlive time.
	KeepAlive uint16
	// SessionExpiry is the session expiry interval in seconds.
	// If the client version is v5, this value will be set into CONNACK Session Expiry Interval property.
	// See: https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901082
	SessionExpiry uint32
	// MaxInflight limits the number of QoS 1 and QoS 2 publications that the client is willing to process concurrently.
	// For v3 client, it is default to config.MQTT.MaxInflight.
	// For v5 client, it is the minimum of config.MQTT.MaxInflight and Receive Maximum property in CONNECT packet.
	MaxInflight uint16
	// ReceiveMax limits the number of QoS 1 and QoS 2 publications that the server is willing to process concurrently for the Client.
	// If the client version is v5, this value will be set into Receive Maximum property in CONNACK packet.
	// See: https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901083
	ReceiveMax uint16
	// ClientMaxPacketSize is the maximum packet size that the client is willing to accept.
	// The server will drop the packet if it exceeds ClientMaxPacketSize.
	// See: https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901050
	ClientMaxPacketSize uint32
	// ServerMaxPacketSize is the maximum packet size that the server is willing to accept from the client.
	// See: https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901086
	ServerMaxPacketSize uint32
	// ClientTopicAliasMax is highest value that the client will accept as a Topic Alias sent by the server.
	// See: https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901051
	ClientTopicAliasMax uint16
	// ServerTopicAliasMax is highest value that the server will accept as a Topic Alias sent by the client.
	// See: https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901088
	ServerTopicAliasMax uint16
	// RequestProblemInfo is the value to indicate whether the Reason String or User Properties should be sent in the case of failures.
	// See: https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901053
	RequestProblemInfo bool
	// UserProperties is the user properties provided by the client.
	// See: https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901090
	UserProperties []*packets.UserProperty
	// WildcardSubAvailable indicates whether the client is permitted to send retained messages.
	// See: https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901091
	RetainAvailable bool
	// WildcardSubAvailable indicates whether the client is permitted to subscribe Wildcard Subscriptions.
	// See: https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901091
	WildcardSubAvailable bool
	// SubIDAvailable indicates whether the client is permitted to set Subscription Identifiers.
	// See: https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901092
	SubIDAvailable bool
	// SharedSubAvailable indicates whether the client is permitted to subscribe Shared Subscriptions.
	// See: https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901093
	SharedSubAvailable bool
	// AuthMethod is the auth method send by the client.
	// Only MQTT v5 client can set this value.
	// See: https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901055
	AuthMethod []byte
}

// Client represent a mqtt client.
type Client interface {
	// ClientOptions return a reference of ClientOptions. Do not edit.
	// This is mainly used in hooks.
	ClientOptions() *ClientOptions
	// SessionInfo return a reference of session information of the client. Do not edit.
	// Session info will be available after the client has passed OnSessionCreated or OnSessionResume.
	SessionInfo() *gmqtt.Session
	// Version return the protocol version of the used client.
	Version() packets.Version
	// ConnectedAt returns the connected time
	ConnectedAt() time.Time
	// Connection returns the raw net.Conn
	Connection() net.Conn
	// Close closes the client connection.
	Close()
	// Disconnect sends a disconnect packet to client, it is use to close v5 client.
	Disconnect(disconnect *packets.Disconnect)
}

// client represents a MQTT client and implements the Client interface
type client struct {
	connectedAt  int64
	server       *server
	wg           sync.WaitGroup
	rwc          net.Conn //raw tcp connection
	bufr         *bufio.Reader
	bufw         *bufio.Writer
	packetReader *packets.Reader
	packetWriter *packets.Writer
	in           chan packets.Packet
	out          chan packets.Packet
	close        chan struct{}
	closed       chan struct{}
	connected    chan struct{}
	status       int32
	// if 1, when client close, the session expiry interval will be ignored and the session will be removed.
	forceRemoveSession int32
	error              chan error
	errOnce            sync.Once
	err                error

	opts    *ClientOptions //set up before OnConnect()
	session *gmqtt.Session

	cleanWillFlag bool // whether to remove will Msg

	disconnect *packets.Disconnect

	topicAliasManager TopicAliasManager
	version           packets.Version
	aliasMapper       [][]byte

	// gard serverReceiveMaximumQuota
	serverQuotaMu             sync.Mutex
	serverReceiveMaximumQuota uint16

	config config.Config

	queueStore    queue.Store
	unackStore    unack.Store
	pl            *packetIDLimiter
	queueNotifier *queueNotifier
	// register requests the broker to add the client into the "active client list"  before sending a positive CONNACK to the client.
	register func(connect *packets.Connect, client *client) (sessionResume bool, err error)
	// unregister requests the broker to remove the client from the "active client list" when the client is disconnected.
	unregister func(client *client)
	// deliverMessage
	deliverMessage func(srcClientID string, msg *gmqtt.Message, options subscription.IterationOptions) (matched bool)
}

func (client *client) SessionInfo() *gmqtt.Session {
	return client.session
}

func (client *client) Version() packets.Version {
	return client.version
}

func (client *client) Disconnect(disconnect *packets.Disconnect) {
	client.write(disconnect)
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
		if err != nil && err != io.EOF {
			zaplog.Error("connection lost",
				zap.String("client_id", client.opts.ClientID),
				zap.String("remote_addr", client.rwc.RemoteAddr().String()),
				zap.Error(err))
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
	srv := client.server
	defer func() {
		if re := recover(); re != nil {
			err = errors.New(fmt.Sprint(re))
		}
		client.setError(err)
	}()
	for {
		select {
		case <-client.close:
			return
		case packet := <-client.out:
			switch p := packet.(type) {
			case *packets.Publish:
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
				// OnDelivered hook
				if srv.hooks.OnDelivered != nil {
					srv.hooks.OnDelivered(context.Background(), client, gmqtt.MessageFromPublish(p))
				}
				srv.statsManager.messageSent(p.Qos, client.opts.ClientID)
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
			srv.statsManager.packetSent(packet, client.opts.ClientID)
			if _, ok := packet.(*packets.Disconnect); ok {
				_ = client.rwc.Close()
				return
			}
		}
	}
}

func (client *client) writePacket(packet packets.Packet) error {
	if client.server.config.Log.DumpPacket {
		if ce := zaplog.Check(zapcore.DebugLevel, "sending packet"); ce != nil {
			ce.Write(
				zap.String("packet", packet.String()),
				zap.String("remote_addr", client.rwc.RemoteAddr().String()),
				zap.String("client_id", client.opts.ClientID),
			)
		}
	}

	return client.packetWriter.WriteAndFlush(packet)
}

func (client *client) addServerQuota() {
	client.serverQuotaMu.Lock()
	if client.serverReceiveMaximumQuota < client.opts.ReceiveMax {
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
	srv := client.server
	defer func() {
		if re := recover(); re != nil {
			err = errors.New(fmt.Sprint(re))
		}
		client.setError(err)
		close(client.in)
	}()
	for {
		var packet packets.Packet
		if client.IsConnected() {
			if keepAlive := client.opts.KeepAlive; keepAlive != 0 { //KeepAlive
				_ = client.rwc.SetReadDeadline(time.Now().Add(time.Duration(keepAlive/2+keepAlive) * time.Second))
			}
		}
		packet, err = client.packetReader.ReadPacket()
		if err != nil {
			if err != io.EOF && packet != nil {
				zaplog.Error("read error", zap.String("packet_type", reflect.TypeOf(packet).String()))
			}
			return
		}

		if pub, ok := packet.(*packets.Publish); ok {
			srv.statsManager.messageReceived(pub.Qos, client.opts.ClientID)
			if client.version == packets.Version5 && pub.Qos > packets.Qos0 {
				err = client.tryDecServerQuota()
				if err != nil {
					return
				}
			}
		}
		client.in <- packet
		<-client.connected
		srv.statsManager.packetReceived(packet, client.opts.ClientID)
		if client.server.config.Log.DumpPacket {
			if ce := zaplog.Check(zapcore.DebugLevel, "received packet"); ce != nil {
				ce.Write(
					zap.String("packet", packet.String()),
					zap.String("remote_addr", client.rwc.RemoteAddr().String()),
					zap.String("client_id", client.opts.ClientID),
				)
			}
		}
	}
}

// Close closes the client connection. The returned channel will be closed after unregisterClient process has been done
func (client *client) Close() {
	if client.rwc != nil {
		_ = client.rwc.Close()
	}
}

var pid = os.Getpid()
var counter uint32
var machineID = readMachineID()

func readMachineID() []byte {
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
	b[4] = machineID[0]
	b[5] = machineID[1]
	b[6] = machineID[2]
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

func sendErrConnack(cli *client, err error) {
	codeErr := converError(err)
	// Override the error code if it is invalid for V3 client.
	if packets.IsVersion3X(cli.version) && codeErr.Code > codes.V3NotAuthorized {
		codeErr.Code = codes.NotAuthorized
	}
	cli.out <- &packets.Connack{
		Version:    cli.version,
		Code:       codeErr.Code,
		Properties: getErrorProperties(cli, &codeErr.ErrorDetails),
	}
}

func (client *client) connectWithTimeOut() (ok bool) {
	// if any error occur, this function should set the error to the client and return false
	var err error
	defer func() {
		if err != nil {
			client.setError(err)
			ok = false
		} else {
			ok = true
		}
		close(client.connected)
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
					break
				}
				conn = p.(*packets.Connect)
				var resp *EnhancedAuthResponse
				authOpts, resp, err = client.connectHandler(conn)
				if err != nil {
					break
				}
				if resp != nil && resp.Continue {
					code = codes.ContinueAuthentication
					authData = resp.AuthData
					onAuth = resp.OnAuth
				} else {
					code = codes.Success
				}
			case *packets.Auth:
				if conn == nil || packets.IsVersion3X(client.version) {
					err = codes.ErrProtocol
					break
				}
				if onAuth == nil {
					err = codes.ErrProtocol
					break
				}
				au := p.(*packets.Auth)
				if au.Code != codes.ContinueAuthentication {
					err = codes.ErrProtocol
					break
				}

				var authResp *AuthResponse
				authResp, err = client.authHandler(au, authOpts, onAuth)
				if err != nil {
					break
				}
				if authResp.Continue {
					code = codes.ContinueAuthentication
					authData = authResp.AuthData
				} else {
					code = codes.Success
				}
			default:
				err = &codes.Error{
					Code: codes.MalformedPacket,
				}
				break
			}
			// authentication fail
			if err != nil {
				sendErrConnack(client, err)
				return
			}
			// continue authentication (ContinueAuthentication is introduced in V5)
			if code == codes.ContinueAuthentication {
				client.out <- &packets.Auth{
					Code: code,
					Properties: &packets.Properties{
						AuthMethod: conn.Properties.AuthMethod,
						AuthData:   authData,
					},
				}
				continue
			}

			// authentication success
			client.opts.RetainAvailable = authOpts.RetainAvailable
			client.opts.WildcardSubAvailable = authOpts.WildcardSubAvailable
			client.opts.SubIDAvailable = authOpts.SubIDAvailable
			client.opts.SharedSubAvailable = authOpts.SharedSubAvailable
			client.opts.SessionExpiry = authOpts.SessionExpiry
			client.opts.MaxInflight = authOpts.MaxInflight
			client.opts.ReceiveMax = authOpts.ReceiveMax
			client.opts.ClientMaxPacketSize = math.MaxUint32 // unlimited
			client.opts.ServerMaxPacketSize = authOpts.MaxPacketSize
			client.opts.ServerTopicAliasMax = authOpts.TopicAliasMax
			client.opts.Username = string(conn.Username)

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

			var connackPpt *packets.Properties
			if client.version == packets.Version5 {
				client.opts.MaxInflight = convertUint16(conn.Properties.ReceiveMaximum, client.opts.MaxInflight)
				client.opts.ClientMaxPacketSize = convertUint32(conn.Properties.MaximumPacketSize, client.opts.ClientMaxPacketSize)
				client.opts.ClientTopicAliasMax = convertUint16(conn.Properties.TopicAliasMaximum, client.opts.ClientTopicAliasMax)
				client.opts.AuthMethod = conn.Properties.AuthMethod
				client.serverReceiveMaximumQuota = client.opts.ReceiveMax
				client.aliasMapper = make([][]byte, client.opts.ReceiveMax+1)
				client.opts.KeepAlive = authOpts.KeepAlive

				var maxQoS byte
				if authOpts.MaximumQoS >= 2 {
					maxQoS = byte(1)
				} else {
					maxQoS = byte(0)
				}

				connackPpt = &packets.Properties{
					SessionExpiryInterval: &authOpts.SessionExpiry,
					ReceiveMaximum:        &authOpts.ReceiveMax,
					MaximumQoS:            &maxQoS,
					RetainAvailable:       bool2Byte(authOpts.RetainAvailable),
					TopicAliasMaximum:     &authOpts.TopicAliasMax,
					WildcardSubAvailable:  bool2Byte(authOpts.WildcardSubAvailable),
					SubIDAvailable:        bool2Byte(authOpts.SubIDAvailable),
					SharedSubAvailable:    bool2Byte(authOpts.SharedSubAvailable),
					MaximumPacketSize:     &authOpts.MaxPacketSize,
					ServerKeepAlive:       &authOpts.KeepAlive,
					AssignedClientID:      authOpts.AssignedClientID,
					ResponseInfo:          authOpts.ResponseInfo,
				}
			} else {
				client.opts.KeepAlive = conn.KeepAlive
			}

			if keepAlive := client.opts.KeepAlive; keepAlive != 0 { //KeepAlive
				_ = client.rwc.SetReadDeadline(time.Now().Add(time.Duration(keepAlive/2+keepAlive) * time.Second))
			}
			client.newPacketIDLimiter(client.opts.MaxInflight)

			var sessionResume bool
			sessionResume, err = client.register(conn, client)
			if err != nil {
				sendErrConnack(client, err)
				return
			}
			connack := conn.NewConnackPacket(codes.Success, sessionResume)
			if conn.Version == packets.Version5 {
				connack.Properties = connackPpt
			}
			client.write(connack)
			return
		case <-timeout.C:
			err = ErrConnectTimeOut
			return
		}
	}
}

func (client *client) basicAuth(conn *packets.Connect, authOpts *AuthOptions) (err error) {
	srv := client.server
	if srv.hooks.OnBasicAuth != nil {
		err = srv.hooks.OnBasicAuth(context.Background(), client, &ConnectRequest{
			Connect: conn,
			Options: authOpts,
		})

	}
	return err
}

func (client *client) enhancedAuth(conn *packets.Connect, authOpts *AuthOptions) (resp *EnhancedAuthResponse, err error) {
	srv := client.server
	if srv.hooks.OnEnhancedAuth == nil {
		return nil, errors.New("OnEnhancedAuth hook is nil")
	}

	resp, err = srv.hooks.OnEnhancedAuth(context.Background(), client, &ConnectRequest{
		Connect: conn,
		Options: authOpts,
	})
	if err == nil && resp == nil {
		err = errors.New("return nil response from OnEnhancedAuth hook")
	}
	return resp, err
}

func (client *client) connectHandler(conn *packets.Connect) (authOpts *AuthOptions, enhancedResp *EnhancedAuthResponse, err error) {
	if !client.config.MQTT.AllowZeroLenClientID && len(conn.ClientID) == 0 {
		err = &codes.Error{
			Code: codes.ClientIdentifierNotValid,
		}
		return
	}
	client.version = conn.Version
	// default auth options
	authOpts = client.defaultAuthOptions(conn)

	if packets.IsVersion3X(client.version) || (packets.IsVersion5(client.version) && conn.Properties.AuthMethod == nil) {
		err = client.basicAuth(conn, authOpts)
	}
	if client.version == packets.Version5 && conn.Properties.AuthMethod != nil {
		enhancedResp, err = client.enhancedAuth(conn, authOpts)
	}

	return
}

func (client *client) authHandler(auth *packets.Auth, authOpts *AuthOptions, onAuth OnAuth) (resp *AuthResponse, err error) {
	authResp, err := onAuth(context.Background(), client, &AuthRequest{
		Auth:    auth,
		Options: authOpts,
	})
	if err == nil && authResp == nil {
		return nil, errors.New("return nil response from OnAuth hook")
	}
	return authResp, err
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
		MaxInflight:          client.config.MQTT.MaxInflight,
	}
	if connect.KeepAlive < opts.KeepAlive {
		opts.KeepAlive = connect.KeepAlive
	}
	if client.version == packets.Version5 {
		if i := connect.Properties.SessionExpiryInterval; i == nil {
			opts.SessionExpiry = 0
		} else if *i < opts.SessionExpiry {
			opts.SessionExpiry = *i

		}
	}
	return opts
}

func (client *client) internalClose() {
	if client.IsConnected() {
		// OnClosed hooks
		if client.server.hooks.OnClosed != nil {
			client.server.hooks.OnClosed(context.Background(), client, client.err)
		}
		client.unregister(client)
		client.server.statsManager.clientDisconnected(client.opts.ClientID)
	}
	putBufioReader(client.bufr)
	putBufioWriter(client.bufw)
	close(client.closed)

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

func (client *client) subscribeHandler(sub *packets.Subscribe) *codes.Error {
	srv := client.server
	suback := &packets.Suback{
		Version:    sub.Version,
		PacketID:   sub.PacketID,
		Properties: &packets.Properties{},
		Payload:    make([]codes.Code, len(sub.Topics)),
	}
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
	subReq := &SubscribeRequest{
		Subscribe: sub,
		Subscriptions: make(map[string]*struct {
			Sub   *gmqtt.Subscription
			Error error
		}),
		ID: subID,
	}

	for _, v := range sub.Topics {
		subReq.Subscriptions[v.Name] = &struct {
			Sub   *gmqtt.Subscription
			Error error
		}{Sub: subscription.FromTopic(v, subID), Error: nil}
	}

	if srv.hooks.OnSubscribe != nil {
		err := srv.hooks.OnSubscribe(context.Background(), client, subReq)
		if ce := converError(err); ce != nil {
			suback.Properties = getErrorProperties(client, &ce.ErrorDetails)
			for k := range suback.Payload {
				if packets.IsVersion3X(client.version) {
					suback.Payload[k] = packets.SubscribeFailure
				} else {
					suback.Payload[k] = ce.Code
				}
			}
			client.write(suback)
			return nil
		}
	}
	for k, v := range sub.Topics {
		sub := subReq.Subscriptions[v.Name].Sub
		subErr := converError(subReq.Subscriptions[v.Name].Error)
		var isShared bool
		code := sub.QoS
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
		var err error
		if subErr != nil {
			code = subErr.Code
			if packets.IsVersion3X(client.version) {
				code = packets.SubscribeFailure
			}
		}
		if code < packets.SubscribeFailure {
			subRs, err = srv.subscriptionsDB.Subscribe(client.opts.ClientID, sub)
			if err != nil {
				zaplog.Error("failed to subscribe topic",
					zap.String("topic", v.Name),
					zap.Uint8("qos", v.Qos),
					zap.String("client_id", client.opts.ClientID),
					zap.String("remote_addr", client.rwc.RemoteAddr().String()),
					zap.Error(err))
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
						client.queueNotifier.notifyDropped(v, &queue.InternalError{Err: err})
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
		}
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
			srv.retainedDB.AddOrReplace(msg.Copy())
		}
	}

	var err error
	var topicMatched bool
	if !dup {
		opts := defaultIterateOptions(msg.Topic)
		if srv.hooks.OnMsgArrived != nil {
			req := &MsgArrivedRequest{
				Publish:          pub,
				Message:          msg,
				IterationOptions: opts,
			}
			err = srv.hooks.OnMsgArrived(context.Background(), client, req)
			msg = req.Message
			opts = req.IterationOptions
		}
		if msg != nil && err == nil {
			topicMatched = client.deliverMessage(client.opts.ClientID, msg, opts)
		}
	}

	var ack packets.Packet
	// ack properties
	var ppt *packets.Properties
	code := codes.Success
	if client.version == packets.Version5 {
		if !topicMatched && err == nil {
			code = codes.NotMatchingSubscribers
		}
		if codeErr := converError(err); codeErr != nil {
			ppt = getErrorProperties(client, &codeErr.ErrorDetails)
			code = codeErr.Code
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
		Properties: &packets.Properties{},
	}
	cs := make([]codes.Code, len(unSub.Topics))
	defer func() {
		if client.version == packets.Version5 {
			unSuback.Payload = cs
		}
		client.write(unSuback)
	}()
	req := &UnsubscribeRequest{
		Unsubscribe: unSub,
		Unsubs: make(map[string]*struct {
			TopicName string
			Error     error
		}),
	}

	for _, v := range unSub.Topics {
		req.Unsubs[v] = &struct {
			TopicName string
			Error     error
		}{TopicName: v}
	}
	if srv.hooks.OnUnsubscribe != nil {
		err := srv.hooks.OnUnsubscribe(context.Background(), client, req)
		if ce := converError(err); ce != nil {
			unSuback.Properties = getErrorProperties(client, &ce.ErrorDetails)
			for k := range cs {
				cs[k] = ce.Code
			}
			return
		}
	}
	for k, v := range unSub.Topics {
		code := codes.Success
		topicName := req.Unsubs[v].TopicName
		ce := converError(req.Unsubs[v].Error)
		if ce != nil {
			code = ce.Code
		}
		if code == codes.Success {
			err := srv.subscriptionsDB.Unsubscribe(client.opts.ClientID, topicName)
			if ce := converError(err); ce != nil {
				code = ce.Code
			}
		}
		if code == codes.Success {
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
				zap.Uint8("code", code))
		}
		cs[k] = code

	}
}

func (client *client) reAuthHandler(auth *packets.Auth) *codes.Error {
	srv := client.server
	// default code
	code := codes.Success
	var resp *AuthResponse
	var err error
	if srv.hooks.OnReAuth != nil {
		resp, err = srv.hooks.OnReAuth(context.Background(), client, auth)
		ce := converError(err)
		if ce != nil {
			return ce
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
		lockedPid: bitmap.New(packets.MaxPacketID),
	}
}

func (client *client) pollInflights() (cont bool, err error) {
	var elems []*queue.Elem
	elems, err = client.queueStore.ReadInflight(uint(client.opts.MaxInflight))
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
		max := uint16(100)
		if client.opts.MaxInflight < max {
			max = client.opts.MaxInflight
		}
		ids = client.pl.pollPacketIDs(max)
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
	readWg := &sync.WaitGroup{}

	readWg.Add(1)
	go func() { //read
		client.readLoop()
		readWg.Done()
	}()

	client.wg.Add(1)
	go func() { //write
		client.writeLoop()
		client.wg.Done()
	}()

	if ok := client.connectWithTimeOut(); ok {
		client.wg.Add(2)
		go func() {
			client.pollMessageHandler()
			client.wg.Done()
		}()
		go func() {
			client.readHandle()
			client.wg.Done()
		}()

	}
	readWg.Wait()

	if client.queueStore != nil {
		qerr := client.queueStore.Close()
		if qerr != nil {
			zaplog.Error("fail to close message queue", zap.String("client_id", client.opts.ClientID), zap.Error(qerr))
		}
	}
	if client.pl != nil {
		client.pl.close()
	}
	client.wg.Wait()
	_ = client.rwc.Close()
}
