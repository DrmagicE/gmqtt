package server

import (
	"context"
	"net"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/persistence/subscription"
	"github.com/DrmagicE/gmqtt/pkg/packets"
)

type Hooks struct {
	OnAccept
	OnStop
	OnSubscribe
	OnSubscribed
	OnUnsubscribe
	OnUnsubscribed
	OnMsgArrived
	OnBasicAuth
	OnEnhancedAuth
	OnReAuth
	OnConnected
	OnSessionCreated
	OnSessionResumed
	OnSessionTerminated
	OnDelivered
	OnClosed
	OnMsgDropped
	OnWillPublish
	OnWillPublished
}

// WillMsgRequest is the input param for OnWillPublish hook.
type WillMsgRequest struct {
	// Message is the message that is going to send.
	// The caller can edit this field to modify the will message.
	// If nil, the broker will drop the message.
	Message *gmqtt.Message
	// IterationOptions is the same as MsgArrivedRequest.IterationOptions,
	// see MsgArrivedRequest for details
	IterationOptions subscription.IterationOptions
}

// Drop drops the will message, so the message will not be delivered to any clients.
func (w *WillMsgRequest) Drop() {
	w.Message = nil
}

// OnWillPublish will be called before the client with the given clientID sending the will message.
// It provides the ability to modify the message before sending.
type OnWillPublish func(ctx context.Context, clientID string, req *WillMsgRequest)

type OnWillPublishWrapper func(OnWillPublish) OnWillPublish

// OnWillPublished will be called after the will message has been sent by the client.
// The msg param is immutable, DO NOT EDIT.
type OnWillPublished func(ctx context.Context, clientID string, msg *gmqtt.Message)

type OnWillPublishedWrapper func(OnWillPublished) OnWillPublished

// OnAccept will be called after a new connection established in TCP server.
// If returns false, the connection will be close directly.
type OnAccept func(ctx context.Context, conn net.Conn) bool

type OnAcceptWrapper func(OnAccept) OnAccept

// OnStop will be called on server.Stop()
type OnStop func(ctx context.Context)

type OnStopWrapper func(OnStop) OnStop

// SubscribeRequest represents the subscribe request made by a SUBSCRIBE packet.
type SubscribeRequest struct {
	// Subscribe is the SUBSCRIBE packet. It is immutable, do not edit.
	Subscribe *packets.Subscribe
	// Subscriptions wraps all subscriptions by the full topic name.
	// You can modify the value of the map to edit the subscription. But must not change the length of the map.
	Subscriptions map[string]*struct {
		// Sub is the subscription.
		Sub *gmqtt.Subscription
		// Error indicates whether to allow the subscription.
		// Return nil means it is allow to make the subscription.
		// Return an error means it is not allow to make the subscription.
		// It is recommended to use *codes.Error if you want to disallow the subscription. e.g:&codes.Error{Code:codes.NotAuthorized}
		// See: https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901178
		Error error
	}
	// ID is the subscription id, this value will override the id of subscriptions in Subscriptions.Sub.
	// This field take no effect on v3 client.
	ID uint32
}

// GrantQoS grants the qos to the subscription for the given topic name.
func (s *SubscribeRequest) GrantQoS(topicName string, qos packets.QoS) *SubscribeRequest {
	if sub := s.Subscriptions[topicName]; sub != nil {
		sub.Sub.QoS = qos
	}
	return s
}

// Reject rejects the subscription for the given topic name.
func (s *SubscribeRequest) Reject(topicName string, err error) {
	if sub := s.Subscriptions[topicName]; sub != nil {
		sub.Error = err
	}
}

// SetID sets the subscription id for the subscriptions
func (s *SubscribeRequest) SetID(id uint32) *SubscribeRequest {
	s.ID = id
	return s
}

// OnSubscribe will be called when receive a SUBSCRIBE packet.
// It provides the ability to modify and authorize the subscriptions.
// If return an error, the returned error will override the error set in SubscribeRequest.
type OnSubscribe func(ctx context.Context, client Client, req *SubscribeRequest) error

type OnSubscribeWrapper func(OnSubscribe) OnSubscribe

// OnSubscribed will be called after the topic subscribe successfully
type OnSubscribed func(ctx context.Context, client Client, subscription *gmqtt.Subscription)

type OnSubscribedWrapper func(OnSubscribed) OnSubscribed

// OnUnsubscribed will be called after the topic has been unsubscribed
type OnUnsubscribed func(ctx context.Context, client Client, topicName string)

type OnUnsubscribedWrapper func(OnUnsubscribed) OnUnsubscribed

// UnsubscribeRequest is the input param for OnSubscribed hook.
type UnsubscribeRequest struct {
	// Unsubscribe is the UNSUBSCRIBE packet. It is immutable, do not edit.
	Unsubscribe *packets.Unsubscribe
	// Unsubs groups all unsubscribe topic by the full topic name.
	// You can modify the value of the map to edit the unsubscribe topic. But you cannot change the length of the map.
	Unsubs map[string]*struct {
		// TopicName is the topic that is going to unsubscribe.
		TopicName string
		// Error indicates whether to allow the unsubscription.
		// Return nil means it is allow to unsubscribe the topic.
		// Return an error means it is not allow to unsubscribe the topic.
		// It is recommended to use *codes.Error if you want to disallow the unsubscription. e.g:&codes.Error{Code:codes.NotAuthorized}
		// See: https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901194
		Error error
	}
}

// Reject rejects the subscription for the given topic name.
func (u *UnsubscribeRequest) Reject(topicName string, err error) {
	if sub := u.Unsubs[topicName]; sub != nil {
		sub.Error = err
	}
}

// OnUnsubscribe will be called when receive a UNSUBSCRIBE packet.
// User can use this function to modify and authorize unsubscription.
// If return an error, the returned error will override the error set in UnsubscribeRequest.
type OnUnsubscribe func(ctx context.Context, client Client, req *UnsubscribeRequest) error

type OnUnsubscribeWrapper func(OnUnsubscribe) OnUnsubscribe

// OnMsgArrived will be called when receive a Publish packets.It provides the ability to modify the message before topic match process.
// The return error is for V5 client to provide additional information for diagnostics and will be ignored if the version of used client is V3.
// If the returned error type is *codes.Error, the code, reason string and user property will be set into the ack packet(puback for qos1, and pubrel for qos2);
// otherwise, the code,reason string  will be set to 0x80 and error.Error().
type OnMsgArrived func(ctx context.Context, client Client, req *MsgArrivedRequest) error

// MsgArrivedRequest is the input param for OnMsgArrived hook.
type MsgArrivedRequest struct {
	// Publish is the origin MQTT PUBLISH packet, it is immutable. DO NOT EDIT.
	Publish *packets.Publish
	// Message is the message that is going to be passed to topic match process.
	// The caller can modify it.
	Message *gmqtt.Message
	// IterationOptions provides the the ability to change the options of topic matching process.
	// In most of cases, you don't need to modify it.
	// The default value is:
	// 	subscription.IterationOptions{
	//		Type:      subscription.TypeAll,
	//		MatchType: subscription.MatchFilter,
	//		TopicName: msg.Topic,
	//	}
	// The user of this field is the federation plugin.
	// It will change the Type from subscription.TypeAll to subscription.subscription.TypeAll ^ subscription.TypeShared
	// that will prevent publishing the shared message to local client.
	IterationOptions subscription.IterationOptions
}

// Drop drops the message, so the message will not be delivered to any clients.
func (m *MsgArrivedRequest) Drop() {
	m.Message = nil
}

type OnMsgArrivedWrapper func(OnMsgArrived) OnMsgArrived

// OnClosed will be called after the tcp connection of the client has been closed
type OnClosed func(ctx context.Context, client Client, err error)

type OnClosedWrapper func(OnClosed) OnClosed

// AuthOptions provides several options which controls how the server interacts with the client.
// The default value of these options is defined in the configuration file.
type AuthOptions struct {
	// SessionExpiry is session expired time in seconds.
	SessionExpiry uint32
	// ReceiveMax limits the number of QoS 1 and QoS 2 publications that the server is willing to process concurrently for the client.
	// If the client version is v5, this value will be set into  Receive Maximum property in CONNACK packet.
	// See: https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901083
	ReceiveMax uint16
	// MaximumQoS is the highest QOS level permitted for a Publish.
	MaximumQoS uint8
	// MaxPacketSize is the maximum packet size that the server is willing to accept from the client.
	// If the client version is v5, this value will be set into Receive Maximum property in CONNACK packet.
	// See: https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901086
	MaxPacketSize uint32
	// TopicAliasMax indicates the highest value that the server will accept as a Topic Alias sent by the client.
	// The server uses this value to limit the number of Topic Aliases that it is willing to hold on this connection.
	// This option only affect v5 client.
	// See: https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901088
	TopicAliasMax uint16
	// RetainAvailable indicates whether the server supports retained messages.
	// See: https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901085
	RetainAvailable bool
	// WildcardSubAvailable indicates whether the server supports Wildcard Subscriptions.
	// See: https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901091
	WildcardSubAvailable bool
	// SubIDAvailable indicates whether the server supports Subscription Identifiers.
	// This option only affect v5 client.
	// See: https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901092
	SubIDAvailable bool
	// SharedSubAvailable indicates whether the server supports Shared Subscriptions.
	// See: https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901093
	SharedSubAvailable bool
	// KeepAlive is the keep alive time assigned by the server.
	// This option only affect v5 client.
	// See: https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901094
	KeepAlive uint16
	// UserProperties is be used to provide additional information to the client.
	// This option only affect v5 client.
	// See: https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901090
	UserProperties []*packets.UserProperty
	// AssignedClientID allows the server to assign a client id for the client.
	// It will override the client id in the connect packet.
	AssignedClientID []byte
	// ResponseInfo is used as the basis for creating a Response Topic.
	// This option only affect v5 client.
	// See: https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901095
	ResponseInfo []byte
	// MaxInflight limits the number of QoS 1 and QoS 2 publications that the client is willing to process concurrently.
	MaxInflight uint16
}

// OnBasicAuth will be called when receive v311 connect packet or v5 connect packet with empty auth method property.
type OnBasicAuth func(ctx context.Context, client Client, req *ConnectRequest) (err error)

// ConnectRequest represents a connect request made by a CONNECT packet.
type ConnectRequest struct {
	// Connect is the CONNECT packet.It is immutable, do not edit.
	Connect *packets.Connect
	// Options represents the setting which will be applied to the current client if auth success.
	// Caller can edit this property to change the setting.
	Options *AuthOptions
}

type OnBasicAuthWrapper func(OnBasicAuth) OnBasicAuth

// OnEnhancedAuth will be called when receive v5 connect packet with auth method property.
type OnEnhancedAuth func(ctx context.Context, client Client, req *ConnectRequest) (resp *EnhancedAuthResponse, err error)

type EnhancedAuthResponse struct {
	Continue bool
	OnAuth   OnAuth
	AuthData []byte
}
type OnEnhancedAuthWrapper func(OnEnhancedAuth) OnEnhancedAuth

type AuthRequest struct {
	Auth    *packets.Auth
	Options *AuthOptions
}

// ReAuthResponse is the response of the OnAuth hook.
type AuthResponse struct {
	// Continue indicate that whether more authentication data is needed.
	Continue bool
	// AuthData is the auth data property of the auth packet.
	AuthData []byte
}

type OnAuth func(ctx context.Context, client Client, req *AuthRequest) (*AuthResponse, error)

type OnReAuthWrapper func(OnReAuth) OnReAuth

type OnReAuth func(ctx context.Context, client Client, auth *packets.Auth) (*AuthResponse, error)

type OnAuthWrapper func(OnAuth) OnAuth

// OnConnected will be called when a mqtt client connect successfully.
type OnConnected func(ctx context.Context, client Client)

type OnConnectedWrapper func(OnConnected) OnConnected

// OnSessionCreated will be called when new session created.
type OnSessionCreated func(ctx context.Context, client Client)

type OnSessionCreatedWrapper func(OnSessionCreated) OnSessionCreated

// OnSessionResumed will be called when session resumed.
type OnSessionResumed func(ctx context.Context, client Client)

type OnSessionResumedWrapper func(OnSessionResumed) OnSessionResumed

type SessionTerminatedReason byte

const (
	NormalTermination SessionTerminatedReason = iota
	TakenOverTermination
	ExpiredTermination
)

// OnSessionTerminated will be called when session has been terminated.
type OnSessionTerminated func(ctx context.Context, clientID string, reason SessionTerminatedReason)

type OnSessionTerminatedWrapper func(OnSessionTerminated) OnSessionTerminated

// OnDelivered will be called when publishing a message to a client.
type OnDelivered func(ctx context.Context, client Client, msg *gmqtt.Message)

type OnDeliveredWrapper func(OnDelivered) OnDelivered

// OnMsgDropped will be called after the Msg dropped.
// The err indicates the reason of dropping.
// See: persistence/queue/error.go
type OnMsgDropped func(ctx context.Context, clientID string, msg *gmqtt.Message, err error)

type OnMsgDroppedWrapper func(OnMsgDropped) OnMsgDropped
