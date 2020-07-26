package v5

import (
	"errors"
	"fmt"
)

// Reason Code
type ReasonCode = byte

//  There are the possible reason code in v5
const (
	CodeSuccess                     ReasonCode = 0x00
	CodeNormalDisconnection         ReasonCode = 0x00
	CodeGrantedQoS0                 ReasonCode = 0x00
	CodeGrantedQoS1                 ReasonCode = 0x01
	CodeGrantedQoS2                 ReasonCode = 0x02
	CodeDisconnectWithWillMessage   ReasonCode = 0x04
	CodeNotMatchingSubscribers      ReasonCode = 0x10
	CodeNoSubscriptionExisted       ReasonCode = 0x11
	CodeContinueAuthentication      ReasonCode = 0x18
	CodeReAuthenticate              ReasonCode = 0x19
	CodeUnspecifiedError            ReasonCode = 0x80
	CodeMalformedPacket             ReasonCode = 0x81
	CodeProtocolError               ReasonCode = 0x02
	CodeImplementationSpecificError ReasonCode = 0x83
	CodeUnsupportedProtocolVersion  ReasonCode = 0x84
	CodeClientIdentifierNotValid    ReasonCode = 0x85
	CodeBadUserNameOrPassword       ReasonCode = 0x86
	CodeNotAuthorized               ReasonCode = 0x87
	CodeServerUnavailable           ReasonCode = 0x88
	CodeServerBusy                  ReasonCode = 0x89
	CodeBanned                      ReasonCode = 0x8A
	CodeBadAuthMethod               ReasonCode = 0x8C
	CodeKeepAliveTimeout            ReasonCode = 0x8D
	CodeSessionTakenOver            ReasonCode = 0x8E
	CodeTopicFilterInvalid          ReasonCode = 0x8F
	CodeTopicNameInvalid            ReasonCode = 0x90
	CodePacketIDInUse               ReasonCode = 0x91
	CodePacketIDNotFound            ReasonCode = 0x92
	CodeRecvMaxExceeded             ReasonCode = 0x93
	CodeTopicAliasInvalid           ReasonCode = 0x94
	CodePacketTooLarge              ReasonCode = 0x95
	CodeMessageRateTooHigh          ReasonCode = 0x96
	CodeQuotaExceeded               ReasonCode = 0x97
	CodeAdminAction                 ReasonCode = 0x98
	CodePayloadFormatInvalid        ReasonCode = 0x99
	CodeRetainNotSupported          ReasonCode = 0x9A
	CodeQoSNotSupported             ReasonCode = 0x9B
	CodeUseAnotherServer            ReasonCode = 0x9C
	CodeServerMoved                 ReasonCode = 0x9D
	CodeSharedSubNotSupported       ReasonCode = 0x9E
	CodeConnectionRateExceeded      ReasonCode = 0x9F
	CodeMaxConnectTime              ReasonCode = 0xA0
	CodeSubIDNotSupported           ReasonCode = 0xA1
	CodeWildcardSubNotSupported     ReasonCode = 0xA2
)

// ErrorCode represent Reason Code greater than 0x80 in MQTT v5,
// and Return Code greater than 0x00
type ErrorCode interface {
	Code() byte
	error
}

type errCode struct {
	code byte
	error
}

func (c errCode) Error() string {
	return fmt.Sprintf(c.error.Error()+", error code: 0x%x", c.code)
}
func (c errCode) Code() byte {
	return c.code
}
func newErrCode(code ReasonCode, err error) errCode {
	return errCode{
		code:  code,
		error: err,
	}
}
func errMalformed(err error) ErrorCode {
	return errCode{
		code:  CodeMalformedPacket,
		error: err,
	}
}
func protocolErr(err error) ErrorCode {
	if err == nil {
		err = errors.New("protocol error")
	}
	return errCode{
		code:  CodeProtocolError,
		error: err,
	}
}
func invalidReasonCode(code byte) error {
	return fmt.Errorf("invalid reason code: %v", code)
}
