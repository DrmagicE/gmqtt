package server

import (
	"context"
	"net"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/pkg/codes"
	"github.com/DrmagicE/gmqtt/pkg/packets"
)

type Hooks struct {
	OnAccept
	OnStop
	OnSubscribe
	OnSubscribed
	OnUnSubscribe
	OnUnsubscribed
	OnMsgArrived
	OnBasicAuth
	OnEnhancedAuth
	OnReAuth
	OnConnected
	OnSessionCreated
	OnSessionResumed
	OnSessionTerminated
	OnDeliver
	OnAcked
	OnClose
	OnMsgDropped
}

// OnAccept 会在新连接建立的时候调用，只在TCP server中有效。如果返回false，则会直接关闭连接
//
// OnAccept will be called after a new connection established in TCP server. If returns false, the connection will be close directly.
type OnAccept func(ctx context.Context, conn net.Conn) bool

type OnAcceptWrapper func(OnAccept) OnAccept

// OnStop will be called on server.Stop()
type OnStop func(ctx context.Context)

type OnStopWrapper func(OnStop) OnStop

/*
OnSubscribe 返回topic允许订阅的最高QoS等级

OnSubscribe returns the maximum available QoS for the topic:
 0x00 - Success - Maximum QoS 0
 0x01 - Success - Maximum QoS 1
 0x02 - Success - Maximum QoS 2
 0x80 - Failure
*/
type OnSubscribe func(ctx context.Context, client Client, subscribe *packets.Subscribe) (resp *SubscribeResponse, err *codes.ErrorDetails)

type SubscribeResponse struct {
	// Topics is the topics that will be subscribed, and the length MUST equal to the length of Topics field in given subscribe packets.
	// If you don't want to modify the topics, set this field equal to subscribe.Topics.
	Topics []packets.Topic
	// ID is the subscription identifier.
	// This field will be ignored if the client version is not v5.
	// nil =  Use default subscription identifier which is defined by given subscribe packet.
	// 0 = Delete subscription identifier.
	ID *uint32
}

type OnSubscribeWrapper func(OnSubscribe) OnSubscribe

// OnSubscribed will be called after the topic subscribe successfully
type OnSubscribed func(ctx context.Context, client Client, subscription *gmqtt.Subscription)

type OnSubscribedWrapper func(OnSubscribed) OnSubscribed

// OnUnsubscribed will be called after the topic has been unsubscribed
type OnUnsubscribed func(ctx context.Context, client Client, topicName string)

type OnUnsubscribedWrapper func(OnUnsubscribed) OnUnsubscribed

type OnUnSubscribe func(ctx context.Context, client Client, unsubscribe *packets.Unsubscribe) (topicNames []string, err *codes.ErrorDetails)

// OnMsgArrived will be called when receive a Publish packets.
// The returned message will be passed to topic match process and delivered to those matched clients.
// If return nil message or error, no message will be deliver.
// The error is for V5 client to provide additional information for diagnostics and will be ignored if the version of used client is V3.
// If the returned error type is *codes.Error, the code, reason string and user property will be set into the ack packet(puback for qos1, and pubrel for qos2);
// otherwise, the code,reason string  will be set to 0x80 and error.Error().
type OnMsgArrived func(ctx context.Context, client Client, publish *packets.Publish) (*gmqtt.Message, error)

type OnMsgArrivedWrapper func(OnMsgArrived) OnMsgArrived

// OnClose will be called after the tcp connection of the client has been closed
type OnClose func(ctx context.Context, client Client, err error)

type OnCloseWrapper func(OnClose) OnClose

type ConnectRequest struct {
	Connect                  *packets.Connect
	DefaultConnackProperties *packets.Properties
}

// OnBasicAuth will be called when receive v311 connect packet or v5 connect packet with empty auth method property.
type OnBasicAuth func(ctx context.Context, client Client, req *ConnectRequest) (resp *ConnectResponse)
type ConnectResponse struct {
	Code              codes.Code
	ConnackProperties *packets.Properties
}
type OnBasicAuthWrapper func(OnBasicAuth) OnBasicAuth

// OnEnhancedAuth will be called when receive v5 connect packet with auth method property.
type OnEnhancedAuth func(ctx context.Context, client Client, req *ConnectRequest) (resp *EnhancedAuthResponse)
type EnhancedAuthResponse struct {
	Code              codes.Code
	OnAuth            OnAuth
	AuthData          []byte
	ConnackProperties *packets.Properties
}
type OnEnhancedAuthWrapper func(OnEnhancedAuth) OnEnhancedAuth

type AuthRequest struct {
	Auth                     *packets.Auth
	DefaultConnackProperties *packets.Properties
}

type AuthResponse struct {
	codes codes.Code
	// AuthData is the auth data property of the auth packet.
	AuthData []byte
	// ConnackProperties is the connack properties.
	ConnackProperties *packets.Properties
}

type OnAuth func(ctx context.Context, client Client, authRequest *AuthRequest) *AuthResponse

type OnReAuthWrapper func(OnReAuth) OnReAuth

// ReAuthResponse is the response of the OnReAuth hook.
type ReAuthResponse struct {
	// Continue indicate that whether more authentication data is needed.
	Continue bool
	// AuthData is the auth data property of the auth packet.
	AuthData []byte
}

type OnReAuth func(ctx context.Context, client Client, authData []byte) (*ReAuthResponse, error)

type OnAuthWrapper func(OnAuth) OnAuth

// OnConnected 当客户端成功连接后触发
//
// OnConnected will be called when a mqtt client connect successfully.
type OnConnected func(ctx context.Context, client Client)

type OnConnectedWrapper func(OnConnected) OnConnected

// OnSessionCreated 新建session时触发
//
// OnSessionCreated will be called when session  created.
type OnSessionCreated func(ctx context.Context, client Client)

type OnSessionCreatedWrapper func(OnSessionCreated) OnSessionCreated

// OnSessionResumed 恢复session时触发
//
// OnSessionResumed will be called when session resumed.
type OnSessionResumed func(ctx context.Context, client Client)

type OnSessionResumedWrapper func(OnSessionResumed) OnSessionResumed

type SessionTerminatedReason byte

const (
	NormalTermination SessionTerminatedReason = iota
	ConflictTermination
	ExpiredTermination
)

// OnSessionTerminated session 下线时触发
//
// OnSessionTerminated will be called when session terminated.
type OnSessionTerminated func(ctx context.Context, client Client, reason SessionTerminatedReason)

type OnSessionTerminatedWrapper func(OnSessionTerminated) OnSessionTerminated

// OnDeliver 分发消息时触发
//
//  OnDeliver will be called when publishing a message to a client.
type OnDeliver func(ctx context.Context, client Client, msg *gmqtt.Message)

type OnDeliverWrapper func(OnDeliver) OnDeliver

// OnAcked 当客户端对qos1或qos2返回确认的时候调用
//
// OnAcked  will be called when receiving the ack in for a published qos1 or qos2 message.
type OnAcked func(ctx context.Context, client Client, msg *gmqtt.Message)

type OnAckedWrapper func(OnAcked) OnAcked

// OnMessageDropped 丢弃报文后触发
//
// OnMsgDropped will be called after the Msg dropped
type OnMsgDropped func(ctx context.Context, client Client, msg *gmqtt.Message)

type OnMsgDroppedWrapper func(OnMsgDropped) OnMsgDropped
