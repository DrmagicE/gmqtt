package gmqtt

import (
	"context"
	"net"

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
	OnConnect
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
type OnSubscribe func(ctx context.Context, client Client, topic packets.Topic) (qos uint8)

type OnSubscribeWrapper func(OnSubscribe) OnSubscribe

// OnSubscribed will be called after the topic subscribe successfully
type OnSubscribed func(ctx context.Context, client Client, topic packets.Topic)

type OnSubscribedWrapper func(OnSubscribed) OnSubscribed

// OnUnsubscribe will be called when the topic is being unsubscribed
type OnUnsubscribe func(ctx context.Context, client Client, topicName string)

type OnUnsubscribeWrapper func(OnUnsubscribe) OnUnsubscribe

// OnUnsubscribed will be called after the topic has been unsubscribed
type OnUnsubscribed func(ctx context.Context, client Client, topicName string)

type OnUnsubscribedWrapper func(OnUnsubscribed) OnUnsubscribed

// OnMsgArrived 返回接收到的publish报文是否允许转发，返回false则该报文不会被继续转发
//
// OnMsgArrived returns whether the publish packet will be delivered or not.
// If returns false, the packet will not be delivered to any clients.
type OnMsgArrived func(ctx context.Context, client Client, msg packets.Message) (valid bool)

type OnMsgArrivedWrapper func(OnMsgArrived) OnMsgArrived

// OnClose tcp连接关闭之后触发
//
// OnClose will be called after the tcp connection of the client has been closed
type OnClose func(ctx context.Context, client Client, err error)

type OnCloseWrapper func(OnClose) OnClose

// OnConnect 当合法的connect报文到达的时候触发，返回connack中响应码
//
// OnConnect will be called when a valid connect packet is received.
// It returns the code of the connack packet
type OnConnect func(ctx context.Context, client Client) (code uint8)

type OnConnectWrapper func(OnConnect) OnConnect

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
type OnDeliver func(ctx context.Context, client Client, msg packets.Message)

type OnDeliverWrapper func(OnDeliver) OnDeliver

// OnAcked 当客户端对qos1或qos2返回确认的时候调用
//
// OnAcked  will be called when receiving the ack packet for a published qos1 or qos2 message.
type OnAcked func(ctx context.Context, client Client, msg packets.Message)

type OnAckedWrapper func(OnAcked) OnAcked

// OnMessageDropped 丢弃报文后触发
//
// OnMsgDropped will be called after the msg dropped
type OnMsgDropped func(ctx context.Context, client Client, msg packets.Message)

type OnMsgDroppedWrapper func(OnMsgDropped) OnMsgDropped
