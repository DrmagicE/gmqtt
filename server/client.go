package server

import (
	"time"
)

type Client struct {
	// if UINT_MAX, 不超时
	SessionExpiryInterval time.Duration

	// 这个就是Inflight队列 65,535
	ReceiveMaximum byte

	// 最大报文长度
	MaximumPacketSize uint32

	// 最大的topic aliase是多少
	TopicAliasMaximum uint16

	// 是否需在Connack中返回 info
	RequestResponseInfo bool

	// The Client uses this value to indicate whether the Reason String or User Properties are sent in the case of failures.
	// If the value of Request Problem Information is 0, the Server MAY return a Reason String or User Properties on a CONNACK or DISCONNECT packet,
	// but MUST NOT send a Reason String or User Properties on any packet other than PUBLISH, CONNACK, or DISCONNECT
	// default 1
	RequestProblemInfo bool

	UserProperties [][2]string

	// WillDelayInterval default 0
	WillDelayInterval uint32

	// publish:

	//
	PayloadFormat byte

	//
	MessageExpiryInterval time.Duration

	//
	ContentType string

	// Request Response
	ResponseTopic string

	// Request ID
	CorrelationData []byte

	WillUserProperties [][2]string
}
