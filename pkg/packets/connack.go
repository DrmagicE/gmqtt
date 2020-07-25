package packets

import (
	`bytes`
	"fmt"
	"io"
)

type ReturnCode = byte
// There are the possible return code in the connack packet in v3.1.1
const (
	ReturnCodeAccepted                    = 0x00
	ReturnCodeUnacceptableProtocolVersion = 0x01
	ReturnCodeIdentifierRejected          = 0x02
	ReturnCodeServerUnavaliable           = 0x03
	ReturnCodeBadUsernameorPsw            = 0x04
	ReturnCodeNotAuthorized               = 0x05
)

// Reason Code
type ReasonCode = byte


//  There are the possible reason code in v5
const (
	ReasonCodeSuccess                     ReasonCode = 0x00
	ReasonCodeNormalDisconnection         ReasonCode = 0x00
	ReasonCodeGrantedQoS0                 ReasonCode = 0x00
	ReasonCodeGrantedQoS1                 ReasonCode = 0x01
	ReasonCodeGrantedQoS2                 ReasonCode = 0x02
	ReasonCodeDisconnectWithWillMessage   ReasonCode = 0x04
	ReasonCodeNotMatchingSubscribers      ReasonCode = 0x10
	ReasonCodeNoSubscriptionExisted       ReasonCode = 0x11
	ReasonCodeContinueAuthentication      ReasonCode = 0x18
	ReasonCodeReAuthenticate              ReasonCode = 0x19
	ReasonCodeUnspecifiedError            ReasonCode = 0x80
	ReasonCodeMalformedPacket             ReasonCode = 0x81
	ReasonCodeProtocolError               ReasonCode = 0x02
	ReasonCodeImplementationSpecificError ReasonCode = 0x83
	ReasonCodeUnsupportedProtocolVersion  ReasonCode = 0x84
	ReasonCodeClientIdentifierNotValid    ReasonCode = 0x85
	ReasonCodeBadUserNameOrPassword       ReasonCode = 0x86
	ReasonCodeNotAuthorized               ReasonCode = 0x87
	ReasonCodeServerUnavailable ReasonCode = 0x88
	ReasonCodeServerBusy ReasonCode = 0x89
	ReasonCodeBanned ReasonCode = 0x8A
	ReasonCodeBadAuthMethod ReasonCode = 0x8C
	ReasonCodeKeepAliveTimeout ReasonCode = 0x8D
	ReasonCodeSessionTakenOver ReasonCode = 0x8E
	ReasonCodeTopicFilterInvalid ReasonCode = 0x8F
	ReasonCodeTopicNameInvalid ReasonCode = 0x90
	ReasonCodePacketIDInUse ReasonCode = 0x91
	ReasonCodePacketIDNotFound ReasonCode = 0x92
	ReasonCodeRecvMaxExceeded ReasonCode = 0x93
	ReasonCodeTopicAliasInvalid ReasonCode = 0x94
	ReasonCodePacketTooLarge ReasonCode = 0x95
	ReasonCodeMessageRateTooHigh ReasonCode = 0x96
	ReasonCodeQuotaExceeded ReasonCode = 0x97
	ReasonCodeAdminAction ReasonCode = 0x98
	ReasonCodePayloadFormatInvalid ReasonCode = 0x99
	ReasonCodeRetainNotSupported ReasonCode = 0x9A
	ReasonCodeQoSNotSupported ReasonCode = 0x9B
	ReasonCodeUseAnotherServer ReasonCode = 0x9C
	ReasonCodeServerMoved ReasonCode = 0x9D
	ReasonCodeSharedSubNotSupported ReasonCode = 0x9E
	ReasonCodeConnectionRateExceeded ReasonCode = 0x9F
	ReasonCodeMaxConnectTime ReasonCode = 0xA0
	ReasonCodeSubIDNotSupported ReasonCode = 0xA1
	ReasonCodeWildcardSubNotSupported ReasonCode = 0xA2

)


// Connack represents the MQTT Connack  packet
type Connack struct {
	FixHeader      *FixHeader
	Version Version
	ReturnCode           ReturnCode  // v3
	ReasonCode ReasonCode //v5
	SessionPresent bool
	Properties *Properties
}

// TODO reasonCode 和returnCode
func (c *Connack) String() string {
	return fmt.Sprintf("Connack, Code:%v, SessionPresent:%v", c.ReturnCode, c.SessionPresent)
}

// Pack encodes the packet struct into bytes and writes it into io.Writer.
func (c *Connack) Pack(w io.Writer) error {
	var err error
	c.FixHeader = &FixHeader{PacketType: CONNACK, Flags: FlagReserved}
	bufw := &bytes.Buffer{}
	if c.SessionPresent {
		bufw.WriteByte(1)
	} else {
		bufw.WriteByte(0)
	}
	if c.Version == Version311 {
		bufw.WriteByte(c.ReturnCode)
	}
	if c.Version == Version5 {
		bufw.WriteByte(c.ReasonCode)
		c.Properties.Pack(bufw, CONNACK)
	}
	c.FixHeader.RemainLength = bufw.Len()
	err = c.FixHeader.Pack(w)
	if err != nil {
		return err
	}
	_, err = bufw.WriteTo(w)
	return err
}

// Unpack read the packet bytes from io.Reader and decodes it into the packet struct
func (c *Connack) Unpack(r io.Reader) error {
	restBuffer := make([]byte, c.FixHeader.RemainLength)
	_, err := io.ReadFull(r, restBuffer)
	if err != nil {
		return errMalformed(err)
	}
	bufr := bytes.NewBuffer(restBuffer)
	sp, err := bufr.ReadByte()
	if (127 & (sp >> 1)) > 0 {
		return errMalformed(ErrInvalConnAcknowledgeFlags)
	}
	c.SessionPresent = sp == 1

	code, _ := bufr.ReadByte()
	// todo validate the code
	if c.Version == Version5 {
		c.ReasonCode = code
		c.Properties = &Properties{}
		err := c.Properties.Unpack(bufr,CONNACK)
		if err != nil {
			return err
		}
	}
	if c.Version == Version311 {
		c.ReturnCode = code
	}
	if c.SessionPresent && code != 0x00 { //v3 [MQTT-3.2.2-4]
		return errMalformed(ErrInvalSessionPresent)
	}
	return nil
}

// NewConnackPacket returns a Connack instance by the given FixHeader and io.Reader
func NewConnackPacket(fh *FixHeader, r io.Reader) (*Connack, error) {
	p := &Connack{FixHeader: fh}
	if fh.Flags != FlagReserved {
		return nil, ErrInvalFlags
	}
	err := p.Unpack(r)
	if err != nil {
		return nil, err
	}
	return p, err
}
