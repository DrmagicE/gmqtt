package packets

import (
	"bytes"
	`fmt`
	"io"
)

const (
	PropPayloadFormat          byte = 0x01
	PropMessageExpiry          byte = 0x02
	PropContentType            byte = 0x03
	PropResponseTopic          byte = 0x08
	PropCorrelationData        byte = 0x09
	PropSubscriptionIdentifier byte = 0x0B
	PropSessionExpiryInterval  byte = 0x11
	PropAssignedClientID       byte = 0x12
	PropServerKeepAlive        byte = 0x13
	PropAuthMethod             byte = 0x15
	PropAuthData               byte = 0x16
	PropRequestProblemInfo     byte = 0x17
	PropWillDelayInterval      byte = 0x18
	PropRequestResponseInfo    byte = 0x19
	PropResponseInfo           byte = 0x1A
	PropServerReference        byte = 0x1C
	PropReasonString           byte = 0x1F
	PropReceiveMaximum         byte = 0x21
	PropTopicAliasMaximum      byte = 0x22
	PropTopicAlias             byte = 0x23
	PropMaximumQOS             byte = 0x24
	PropRetainAvailable        byte = 0x25
	PropUser                   byte = 0x26
	PropMaximumPacketSize      byte = 0x27
	PropWildcardSubAvailable   byte = 0x28
	PropSubIDAvailable         byte = 0x29
	PropSharedSubAvailable     byte = 0x2A
)

func errMorethanOnce(property byte) error {
	return fmt.Errorf("property %v presents more than once", property)
}

func errInvalidValue(property byte, invalid interface{}) error {
	return fmt.Errorf("property %v contains invalid value: %v", property, invalid)
}
const PayloadBytes byte = 0
const PayloadUTF8 byte = 1

type StringPair struct {
	K []byte
	V []byte
}

// Properties is a struct representing the all the described properties
// allowed by the MQTT protocol, determining the validity of a property
// relvative to the packettype it was received in is provided by the
// ValidateID function
type Properties struct {
	// PayloadFormat indicates the format of the payload of the message
	// 0 is unspecified bytes
	// 1 is UTF8 encoded character data
	PayloadFormat *byte
	// MessageExpiry is the lifetime of the message in seconds
	MessageExpiry *uint32
	// ContentType is a UTF8 string describing the content of the message
	// for example it could be a MIME type
	ContentType []byte
	// ResponseTopic is a UTF8 string indicating the topic name to which any
	// response to this message should be sent
	ResponseTopic []byte
	// CorrelationData is binary data used to associate future response
	// messages with the original request message
	CorrelationData []byte
	// SubscriptionIdentifier is an identifier of the subscription to which
	// the Publish matched
	SubscriptionIdentifier []uint32
	// SessionExpiryInterval is the time in seconds after a client disconnects
	// that the server should retain the session Info (subscriptions etc)
	SessionExpiryInterval *uint32
	// AssignedClientID is the server assigned client identifier in the case
	// that a client connected without specifying a clientID the server
	// generates one and returns it in the Connack
	AssignedClientID []byte
	// ServerKeepAlive allows the server to specify in the Connack packet
	// the time in seconds to be used as the keep alive value
	ServerKeepAlive *uint16
	// AuthMethod is a UTF8 string containing the name of the authentication
	// method to be used for extended authentication
	AuthMethod []byte
	// AuthData is binary data containing authentication data
	AuthData []byte
	// RequestProblemInfo is used by the Client to indicate to the server to
	// include the Reason String and/or User Properties in case of failures
	RequestProblemInfo *byte
	// WillDelayInterval is the number of seconds the server waits after the
	// point at which it would otherwise send the will message before sending
	// it. The client reconnecting before that time expires causes the server
	// to cancel sending the will
	WillDelayInterval *uint32
	// RequestResponseInfo is used by the Client to request the Server provide
	// Response Info in the Connack
	RequestResponseInfo *byte
	// ResponseInfo is a UTF8 encoded string that can be used as the basis for
	// createing a Response Topic. The way in which the Client creates a
	// Response Topic from the Response Info is not defined. A common
	// use of this is to pass a globally unique portion of the topic tree which
	// is reserved for this Client for at least the lifetime of its Session. This
	// often cannot just be a random name as both the requesting Client and the
	// responding Client need to be authorized to use it. It is normal to use this
	// as the root of a topic tree for a particular Client. For the Server to
	// return this Info, it normally needs to be correctly configured.
	// Using this mechanism allows this configuration to be done once in the
	// Server rather than in each Client
	ResponseInfo []byte
	// ServerReference is a UTF8 string indicating another server the client
	// can use
	ServerReference []byte
	// ReasonString is a UTF8 string representing the reason associated with
	// this response, intended to be human readable for diagnostic purposes
	ReasonString []byte
	// ReceiveMaximum is the maximum number of QOS1 & 2 messages allowed to be
	// 'inflight' (not having received a PUBACK/PUBCOMP response for)
	ReceiveMaximum *uint16
	// TopicAliasMaximum is the highest value permitted as a Topic Alias
	TopicAliasMaximum *uint16
	// TopicAlias is used in place of the topic string to reduce the size of
	// packets for repeated messages on a topic
	TopicAlias *uint16
	// MaximumQOS is the highest QOS level permitted for a Publish
	MaximumQOS *byte
	// RetainAvailable indicates whether the server supports messages with the
	// retain flag set
	RetainAvailable *byte
	// User is a map of user provided properties
	User []StringPair

	// MaximumPacketSize allows the client or server to specify the maximum packet
	// size in bytes that they support
	MaximumPacketSize *uint32
	// WildcardSubAvailable indicates whether wildcard subscriptions are permitted
	WildcardSubAvailable *byte
	// SubIDAvailable indicates whether subscription identifiers are supported
	SubIDAvailable *byte
	// SharedSubAvailable indicates whether shared subscriptions are supported
	SharedSubAvailable *byte
}

func (p *Properties) PackWillProperties(bufw *bytes.Buffer) {

}
// Pack takes all the defined properties for an Properties and produces
// a slice of bytes representing the wire format for the Info
func (p *Properties) Pack(bufw *bytes.Buffer, packetType byte) {
	if p == nil {
		return
	}
	newBufw := &bytes.Buffer{}
	if packetType == CONNACK {
		if p.SessionExpiryInterval != nil {
			newBufw.WriteByte(PropSessionExpiryInterval)
			writeUint32(newBufw, *p.SessionExpiryInterval)
		}
		if p.ReceiveMaximum != nil {
			newBufw.WriteByte(PropReceiveMaximum)
			writeUint16(newBufw, *p.ReceiveMaximum)
		}
		if p.MaximumQOS != nil {
			newBufw.WriteByte(PropMaximumQOS)
			newBufw.WriteByte(*p.MaximumQOS)
		}
		if p.RetainAvailable != nil {
			newBufw.WriteByte(PropRetainAvailable)
			newBufw.WriteByte(*p.RetainAvailable)
		}
		if p.MaximumPacketSize != nil {
			newBufw.WriteByte(PropMaximumPacketSize)
			writeUint32(newBufw, *p.MaximumPacketSize)
		}
		if p.AssignedClientID != nil {
			newBufw.WriteByte(PropAssignedClientID)
			writeUTF8String(newBufw, p.AssignedClientID)
		}
		if p.TopicAliasMaximum != nil {
			newBufw.WriteByte(PropTopicAliasMaximum)
			writeUint16(newBufw, *p.TopicAliasMaximum)
		}
		if p.ReasonString != nil {
			newBufw.WriteByte(PropReasonString)
			writeUTF8String(newBufw, p.ReasonString)
		}
		if p.WildcardSubAvailable != nil {
			newBufw.WriteByte(PropWildcardSubAvailable)
			newBufw.WriteByte(*p.WildcardSubAvailable)
		}
		if p.SubIDAvailable != nil {
			newBufw.WriteByte(PropSubIDAvailable)
			newBufw.WriteByte(*p.SubIDAvailable)
		}
		if p.SharedSubAvailable != nil {
			newBufw.WriteByte(PropSharedSubAvailable)
			newBufw.WriteByte(*p.SharedSubAvailable)
		}
		if p.ServerKeepAlive != nil {
			newBufw.WriteByte(PropServerKeepAlive)
			writeUint16(newBufw, *p.ServerKeepAlive)
		}
		if p.ResponseInfo != nil {
			newBufw.WriteByte(PropResponseInfo)
			writeUTF8String(newBufw, p.ResponseInfo)
		}
		if p.ServerReference != nil {
			newBufw.WriteByte(PropServerReference)
			writeUTF8String(newBufw, p.ServerReference)
		}
		if p.AuthMethod != nil {
			newBufw.WriteByte(PropAuthMethod)
			writeUTF8String(newBufw, p.AuthMethod)
		}
		if p.AuthData != nil {
			newBufw.WriteByte(PropAuthData)
			writeBinary(newBufw, p.AuthData)
		}
	}

	if packetType == PUBLISH {
		if p.PayloadFormat != nil {
			newBufw.WriteByte(PropPayloadFormat)
			newBufw.WriteByte(*p.PayloadFormat)
		}
		if p.MessageExpiry != nil {
			newBufw.WriteByte(PropMessageExpiry)
			writeUint32(newBufw, *p.MessageExpiry)
		}
		if p.TopicAlias != nil {
			newBufw.WriteByte(PropTopicAlias)
			writeUint16(newBufw, *p.TopicAlias)
		}
		if p.ResponseTopic != nil {
			newBufw.WriteByte(PropResponseTopic)
			writeUTF8String(newBufw, p.ResponseTopic)
		}
		if p.CorrelationData != nil {
			newBufw.WriteByte(PropCorrelationData)
			writeBinary(newBufw, p.CorrelationData)
		}
		if p.ContentType != nil {
			newBufw.WriteByte(PropContentType)
			writeUTF8String(newBufw, p.ContentType)
		}
	}

	if packetType == PUBACK ||
		packetType == PUBREC ||
		packetType == PUBREL ||
		packetType == PUBCOMP ||
		packetType == SUBACK ||
		packetType == UNSUBACK ||
		packetType == DISCONNECT {
		if p.ReasonString != nil {
			newBufw.WriteByte(PropReasonString)
			writeUTF8String(newBufw, p.ReasonString)
		}
	}
	if packetType == DISCONNECT {
		if p.SessionExpiryInterval != nil {
			newBufw.WriteByte(PropSessionExpiryInterval)
			writeUint32(bufw, *p.SessionExpiryInterval)
		}
		if p.ServerReference != nil {
			newBufw.WriteByte(PropServerReference)
			writeUTF8String(bufw, p.ServerReference)
		}

	}

	if p.User != nil {
		for _, v := range p.User {
			newBufw.WriteByte(PropUser)
			writeUTF8String(newBufw, v.K)
			writeUTF8String(newBufw, v.V)
		}
	}

	if packetType == PUBLISH || packetType == SUBSCRIBE {
		if p.SubscriptionIdentifier != nil {
			for _,v := range p.SubscriptionIdentifier {
				newBufw.WriteByte(PropSubscriptionIdentifier)
				writeUint32(newBufw,v)
			}
		}
	}
	//
	//if packetType == CONNECT || packetType == CONNACK {
	//	if p.ReceiveMaximum != nil {
	//		newBufw.WriteByte(PropReceiveMaximum)
	//		writeUint16(bufw, *p.ReceiveMaximum)
	//	}
	//	if p.TopicAliasMaximum != nil {
	//		newBufw.WriteByte(PropTopicAliasMaximum)
	//		writeUint16(bufw, *p.TopicAliasMaximum)
	//	}
	//	if p.MaximumQOS != nil {
	//		newBufw.WriteByte(PropMaximumQOS)
	//		newBufw.WriteByte(*p.MaximumQOS)
	//	}
	//
	//	if p.MaximumPacketSize != nil {
	//		newBufw.WriteByte(PropMaximumPacketSize)
	//		writeUint32(bufw, *p.MaximumPacketSize)
	//	}
	//}
	//
	//if packetType == CONNACK {
	//	if p.AssignedClientID != nil {
	//		newBufw.WriteByte(PropAssignedClientID)
	//		writeUTF8String(bufw, p.AssignedClientID)
	//	}
	//
	//	if p.ServerKeepAlive != nil {
	//		newBufw.WriteByte(PropServerKeepAlive)
	//		writeUint16(bufw, *p.ServerKeepAlive)
	//	}
	//
	//	if p.WildcardSubAvailable != nil {
	//		newBufw.WriteByte(PropWildcardSubAvailable)
	//		newBufw.WriteByte(*p.WildcardSubAvailable)
	//	}
	//
	//	if p.SubIDAvailable != nil {
	//		newBufw.WriteByte(PropSubIDAvailable)
	//		newBufw.WriteByte(*p.SubIDAvailable)
	//	}
	//
	//	if p.SharedSubAvailable != nil {
	//		newBufw.WriteByte(PropSharedSubAvailable)
	//		newBufw.WriteByte(*p.SharedSubAvailable)
	//	}
	//
	//	if p.RetainAvailable != nil {
	//		newBufw.WriteByte(PropRetainAvailable)
	//		newBufw.WriteByte(*p.RetainAvailable)
	//	}
	//
	//	if p.ResponseInfo != nil {
	//		newBufw.WriteByte(PropResponseInfo)
	//		writeUTF8String(bufw, p.ResponseInfo)
	//	}
	//}
	//
	//if packetType == CONNECT {
	//	if p.RequestProblemInfo != nil {
	//		newBufw.WriteByte(PropRequestProblemInfo)
	//		newBufw.WriteByte(*p.RequestProblemInfo)
	//	}
	//
	//	if p.WillDelayInterval != nil {
	//		newBufw.WriteByte(PropWillDelayInterval)
	//		writeUint32(bufw, *p.WillDelayInterval)
	//	}
	//
	//	if p.RequestResponseInfo != nil {
	//		newBufw.WriteByte(PropRequestResponseInfo)
	//		newBufw.WriteByte(*p.RequestResponseInfo)
	//	}
	//}
	//
	//if packetType == CONNECT || packetType == DISCONNECT {
	//	if p.SessionExpiryInterval != nil {
	//		newBufw.WriteByte(PropSessionExpiryInterval)
	//		writeUint32(bufw, *p.SessionExpiryInterval)
	//	}
	//}
	//
	//if packetType == CONNECT || packetType == CONNACK || packetType == AUTH {
	//	if p.AuthMethod != nil {
	//		newBufw.WriteByte(PropAuthMethod)
	//		writeUTF8String(bufw, p.AuthMethod)
	//	}
	//	if p.AuthData != nil && len(p.AuthData) > 0 {
	//		newBufw.WriteByte(PropAuthData)
	//		writeBinary(bufw, p.AuthData)
	//	}
	//}
	//
	//if packetType == CONNACK || packetType == DISCONNECT {
	//	if p.ServerReference != nil {
	//		newBufw.WriteByte(PropServerReference)
	//		writeUTF8String(bufw, p.ServerReference)
	//	}
	//}
	//
	//if packetType != CONNECT {
	//	if p.ReasonString != nil {
	//		newBufw.WriteByte(PropReasonString)
	//		writeUTF8String(bufw, p.ReasonString)
	//	}
	//}
	//
	//for _, v := range p.User {
	//	newBufw.WriteByte(PropUser)
	//	writeUTF8String(bufw, v.Key)
	//	writeUTF8String(bufw, v.Value)
	//}

	b, _ := DecodeRemainLength(newBufw.Len())
	bufw.Write(b)
	newBufw.WriteTo(bufw)
}

// Unpack takes a buffer of bytes and reads out the defined properties
// filling in the appropriate entries in the struct, it returns the number
// of bytes used to store the Prop data and any error in decoding them
func (p *Properties) Unpack(bufr *bytes.Buffer, packetType byte) error {
	var err error
	length, err := EncodeRemainLength(bufr)
	// 整个buffer最多只能读到lenght这么长
	if err != nil {
		return err
	}
	if length == 0 {
		return nil
	}
	newBufr := bytes.NewBuffer(bufr.Next(length))
	for {
		if err != nil {
			return err
		}
		propType, err := newBufr.ReadByte()
		if err != nil && err != io.EOF {
			return err
		}
		if err == io.EOF {
			return nil
		}
		if !ValidateID(packetType, propType) {
			return protocolErr(ErrInvalPacketType)
		}
		switch propType {
		case PropPayloadFormat:
			err = propertyReadBool(p.PayloadFormat,newBufr,propType)
		case PropMessageExpiry:
			err = propertyReadUint32(p.MessageExpiry, newBufr, propType, nil)
		case PropContentType:
			p.ContentType, err = propertyReadUTF8String(p.ContentType, newBufr, propType, nil)
		case PropResponseTopic:
			p.ResponseTopic, err =  propertyReadUTF8String(p.ResponseTopic, newBufr, propType, func(u []byte) bool {
				return ValidTopicName(true,u) // [MQTT-3.3.2-14]
			})
		case PropCorrelationData:
			p.CorrelationData, err =  propertyReadBinary(p.CorrelationData, newBufr, propType, nil)
		case PropSubscriptionIdentifier:
			si, err := readUint32(newBufr)
			if err != nil {
				return err
			}
			if si == 0 {
				return protocolErr(errInvalidValue(propType, si))
			}
			p.SubscriptionIdentifier = append(p.SubscriptionIdentifier, si)

			//if p.SubscriptionIdentifier != nil && packetType == SUBSCRIBE {
			//	return ErrProtocolError
			//}
			//si, err := readUint32(newBufr)
			//if si == 0 {
			//	return ErrProtocolError
			//}
			//if err != nil {
			//	return err
			//}
			//p.SubscriptionIdentifier = append(p.SubscriptionIdentifier, si)
		case PropSessionExpiryInterval:
			if p.SessionExpiryInterval != nil {
				return protocolErr(errMorethanOnce(propType))
			}
			se, err := readUint32(newBufr)
			if err != nil {
				return errMalformed(err)
			}
			p.SessionExpiryInterval = &se
		case PropAssignedClientID:
			p.AssignedClientID, err = propertyReadUTF8String(p.AssignedClientID,newBufr,propType, nil)
		case PropServerKeepAlive:
			err = propertyReadUint16(p.ServerKeepAlive, newBufr,propType,nil)
		case PropAuthMethod:
			p.AuthMethod, err = propertyReadUTF8String(p.AuthMethod,newBufr,propType, nil)
		case PropAuthData:
			p.AuthData, err = propertyReadUTF8String(p.AuthData,newBufr,propType, nil)
		case PropRequestProblemInfo:
			err := propertyReadBool(p.RequestProblemInfo,newBufr,propType)
			if err != nil {
				return err
			}
		case PropWillDelayInterval:
			if p.WillDelayInterval != nil {
				return protocolErr(errMorethanOnce(propType))
			}
			wd, err := readUint32(newBufr)
			if err != nil {
				return err
			}
			p.WillDelayInterval = &wd
		case PropRequestResponseInfo:
			err := propertyReadBool(p.RequestResponseInfo,newBufr,propType)
			if err != nil {
				return err
			}
		case PropResponseInfo:
			p.ResponseInfo, err = propertyReadUTF8String(p.ResponseInfo,newBufr,propType,nil)
		case PropServerReference:
			p.ServerReference, err = propertyReadUTF8String(p.ServerReference,newBufr,propType,nil)
		case PropReasonString:
			p.ReasonString, err = propertyReadUTF8String(p.ReasonString,newBufr,propType,nil)
		case PropReceiveMaximum:
			if p.ReceiveMaximum != nil {
				return protocolErr(errMorethanOnce(propType))
			}
			rm, err := readUint16(newBufr)
			if err != nil {
				return errMalformed(err)
			}
			if rm == 0 {
				return errMalformed(errInvalidValue(propType, 0))
			}
			p.ReceiveMaximum = &rm
		case PropTopicAliasMaximum:
			err = propertyReadUint16(p.TopicAliasMaximum,newBufr,propType,nil)
		case PropTopicAlias:
			err = propertyReadUint16(p.TopicAlias, newBufr, propType, func(u uint16) bool {
				return u != 0 // [MQTT-3.3.2-8]
			})
		case PropMaximumQOS:
			err := propertyReadBool(p.MaximumQOS, newBufr, propType)
			if err != nil {
				return err
			}
		case PropRetainAvailable:
			err := propertyReadBool(p.RetainAvailable, newBufr, propType)
			if err != nil {
				return err
			}
		case PropUser:
			k, err := readUTF8String(true, newBufr)
			if err != nil {
				return errMalformed(err)
			}
			v, err := readUTF8String(true, newBufr)
			if err != nil {
				return errMalformed(err)
			}
			p.User = append(p.User, StringPair{K: k, V: v})
		case PropMaximumPacketSize:
			err = propertyReadUint32(p.MaximumPacketSize,newBufr,propType, func(u uint32) bool {
				return u != 0
			})
		case PropWildcardSubAvailable:
			err =  propertyReadBool(p.WildcardSubAvailable, newBufr,propType)
		case PropSubIDAvailable:
			err =  propertyReadBool(p.SubIDAvailable, newBufr,propType)
		case PropSharedSubAvailable:
			err =  propertyReadBool(p.SharedSubAvailable, newBufr,propType)
		default:
			// TODO
			return errMalformed(err)
		}
	}
	if p.AuthData != nil && p.AuthMethod == nil {
		//TODO
		return errMalformed(err)
	}
	return nil
}



// ValidProperties is a map of the various properties and the
// PacketTypes that property is valid for.
var ValidProperties = map[byte]map[byte]struct{}{
	PropPayloadFormat:          {CONNECT:{},PUBLISH: {}},
	PropMessageExpiry:           {CONNECT:{},PUBLISH: {}},
	PropContentType:             {CONNECT:{},PUBLISH: {}},
	PropResponseTopic:           {CONNECT:{},PUBLISH: {}},
	PropCorrelationData:         {CONNECT:{},PUBLISH: {}},
	PropSubscriptionIdentifier: {
		//PUBLISH: {},
		SUBSCRIBE: {}},
	PropSessionExpiryInterval:  {CONNECT: {},CONNACK:{}, DISCONNECT: {}},
	PropAssignedClientID:{CONNACK:{}},
	PropServerKeepAlive:{CONNACK:{}},
	PropAuthMethod:             {CONNECT: {},CONNACK:{},AUTH: {}},
	PropAuthData:               {CONNECT: {}, CONNACK:{},AUTH: {}},
	PropRequestProblemInfo:     {CONNECT: {}},
	PropWillDelayInterval:      {CONNECT: {}},
	PropRequestResponseInfo:    {CONNECT: {}},
	PropResponseInfo:{CONNACK:{}},
	PropServerReference:        { CONNACK:{},DISCONNECT: {}},
	PropReasonString:           {CONNACK:{},PUBACK: {}, PUBREC: {}, PUBREL: {}, PUBCOMP: {}, SUBACK: {}, UNSUBACK: {}, DISCONNECT: {}, AUTH: {}},
	PropReceiveMaximum:         {CONNECT: {},CONNACK:{}},
	PropTopicAliasMaximum:      {CONNECT: {},CONNACK:{}},
	PropTopicAlias :{PUBLISH:{}},
	PropMaximumQOS:{CONNACK:{}},
	PropRetainAvailable:{CONNACK:{}},
	PropUser:                 {CONNECT: {}, CONNACK: {}, PUBLISH: {}, PUBACK: {}, PUBREC: {}, PUBREL: {}, PUBCOMP: {}, SUBSCRIBE: {}, UNSUBSCRIBE: {}, SUBACK: {}, UNSUBACK: {}, DISCONNECT: {}, AUTH: {}},
	PropMaximumPacketSize:{CONNECT:{},CONNACK:{}},
	PropWildcardSubAvailable:{CONNACK:{}},
	PropSubIDAvailable:{CONNACK:{}},
	PropSharedSubAvailable:{CONNACK:{}},
}


// ValidateID takes a PacketType and a property name and returns
// a boolean indicating if that property is valid for that
// PacketType
func ValidateID(packetType byte, i byte) bool {
	_, ok := ValidProperties[i][packetType]
	return ok
}

func ValidateCode(packType byte, code byte) bool {
	return true
}



func propertyReadBool(i *byte, r *bytes.Buffer, propType byte) (err error) {
	if i != nil {
		return protocolErr(errMorethanOnce(propType))
	}
	o, err := r.ReadByte()
	if err != nil {
		return errMalformed(err)
	}
	if o != 0 && o != 1 {
		return protocolErr(errInvalidValue(propType, o))
	}
	i = &o
	return nil
}

func propertyReadUint32 (i *uint32, r *bytes.Buffer, propType byte, validate func(u uint32) bool ) (err error) {
	if i != nil {
		return errMorethanOnce(propType)
	}
	o, err := readUint32(r)
	if err != nil {
		return errMalformed(err)
	}
	if validate != nil {
		if !validate(o) {
			return protocolErr(errInvalidValue(propType, o))
		}
	}
	i = &o
	return nil
}

func propertyReadUint16 (i *uint16, r *bytes.Buffer, propType byte, validate func(u uint16) bool ) (err error) {
	if i != nil {
		return errMorethanOnce(propType)
	}
	o, err := readUint16(r)
	if err != nil {
		return errMalformed(err)
	}
	if validate != nil {
		if !validate(o) {
			return protocolErr(errInvalidValue(propType, o))
		}
	}
	i = &o
	return nil
}
func propertyReadUTF8String(i []byte, r *bytes.Buffer, propType byte, validate func(u []byte) bool ) (b []byte, err error) {
	if i != nil {
		return nil,errMorethanOnce(propType)
	}
	o, err := readUTF8String(true,r)
	if err != nil {
		return nil,errMalformed(err)
	}
	if validate != nil {
		if !validate(o) {
			return nil,protocolErr(errInvalidValue(propType, o))
		}
	}
	return o, nil
}
func propertyReadBinary(i []byte, r *bytes.Buffer, propType byte, validate func(u []byte) bool ) (b []byte, err error) {
	if i != nil {
		return nil,errMorethanOnce(propType)
	}
	o, err := readUTF8String(false,r)
	if err != nil {
		return nil,errMalformed(err)
	}
	if validate != nil {
		if !validate(o) {
			return nil,protocolErr(errInvalidValue(propType, o))
		}
	}
	return o, nil
}

