package codes

import (
	"fmt"
)

var (
	ErrMalformed = &Error{code: MalformedPacket}
	ErrProtocol  = &Error{code: ProtocolError}
)

// There are the possible code in v311 connack packet.
const (
	Accepted                    = 0x00
	UnacceptableProtocolVersion = 0x01
	IdentifierRejected          = 0x02
	ServerUnavaliable           = 0x03
	BadUsernameorPsw            = 0x04
	NotAuthorizedV3             = 0x05
)

//  Code
type Code = byte

//  There are the possible reason code in v5
const (
	Success                     Code = 0x00
	NormalDisconnection         Code = 0x00
	GrantedQoS0                 Code = 0x00
	GrantedQoS1                 Code = 0x01
	GrantedQoS2                 Code = 0x02
	DisconnectWithWillMessage   Code = 0x04
	NotMatchingSubscribers      Code = 0x10
	NoSubscriptionExisted       Code = 0x11
	ContinueAuthentication      Code = 0x18
	ReAuthenticate              Code = 0x19
	UnspecifiedError            Code = 0x80
	MalformedPacket             Code = 0x81
	ProtocolError               Code = 0x82
	ImplementationSpecificError Code = 0x83
	UnsupportedProtocolVersion  Code = 0x84
	ClientIdentifierNotValid    Code = 0x85
	BadUserNameOrPassword       Code = 0x86
	NotAuthorized               Code = 0x87
	ServerUnavailable           Code = 0x88
	ServerBusy                  Code = 0x89
	Banned                      Code = 0x8A
	BadAuthMethod               Code = 0x8C
	KeepAliveTimeout            Code = 0x8D
	SessionTakenOver            Code = 0x8E
	TopicFilterInvalid          Code = 0x8F
	TopicNameInvalid            Code = 0x90
	PacketIDInUse               Code = 0x91
	PacketIDNotFound            Code = 0x92
	RecvMaxExceeded             Code = 0x93
	TopicAliasInvalid           Code = 0x94
	PacketTooLarge              Code = 0x95
	MessageRateTooHigh          Code = 0x96
	QuotaExceeded               Code = 0x97
	AdminAction                 Code = 0x98
	PayloadFormatInvalid        Code = 0x99
	RetainNotSupported          Code = 0x9A
	QoSNotSupported             Code = 0x9B
	UseAnotherServer            Code = 0x9C
	ServerMoved                 Code = 0x9D
	SharedSubNotSupported       Code = 0x9E
	ConnectionRateExceeded      Code = 0x9F
	MaxConnectTime              Code = 0xA0
	SubIDNotSupported           Code = 0xA1
	WildcardSubNotSupported     Code = 0xA2
)

// Error wraps a pointer of a status proto. It implements error and Status,
// and a nil *Error should never be returned by this package.
type Error struct {
	code Code
}

func (e *Error) Code() Code {
	return e.code
}
func (e *Error) Error() string {
	return fmt.Sprintf("operation error: code = %x", e.code)
}
func NewError(code Code) *Error {
	return &Error{code: code}
}
