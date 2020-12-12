package packets

import (
	"bytes"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/DrmagicE/gmqtt/pkg/codes"
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
	PropMaximumQoS             byte = 0x24
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

type UserProperty struct {
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
	// MaximumQoS is the highest QOS level permitted for a Publish
	MaximumQoS *byte
	// RetainAvailable indicates whether the server supports messages with the
	// retain flag set
	RetainAvailable *byte
	// User is a map of user provided properties
	User []UserProperty

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

func sprintf(name string, v interface{}) string {
	if v == nil {
		return fmt.Sprintf("%s: %v", name, v)
	}
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Ptr {
		rv := reflect.ValueOf(v)
		if rv.IsNil() {
			return fmt.Sprintf("%s: nil", name)
		}
		return fmt.Sprintf("%s: %v", name, reflect.ValueOf(v).Elem())
	}
	return fmt.Sprintf("%s: %v", name, v)

}

func (p *Properties) String() string {
	var str []string
	str = append(str, sprintf("PayloadFormat", p.PayloadFormat))
	str = append(str, sprintf("MessageExpiry", p.MessageExpiry))
	str = append(str, sprintf("ContentType", p.ContentType))
	str = append(str, sprintf("ResponseTopic", p.ResponseTopic))
	str = append(str, sprintf("CorrelationData", p.CorrelationData))
	str = append(str, sprintf("SubscriptionIdentifier", p.SubscriptionIdentifier))
	str = append(str, sprintf("SessionExpiryInterval", p.SessionExpiryInterval))
	str = append(str, sprintf("AssignedClientID", p.AssignedClientID))
	str = append(str, sprintf("ServerKeepAlive", p.ServerKeepAlive))
	str = append(str, sprintf("AuthMethod", p.AuthMethod))
	str = append(str, sprintf("AuthData", p.AuthData))
	str = append(str, sprintf("RequestProblemInfo", p.RequestProblemInfo))
	str = append(str, sprintf("WillDelayInterval", p.WillDelayInterval))
	str = append(str, sprintf("RequestResponseInfo", p.RequestResponseInfo))
	str = append(str, sprintf("ResponseInfo", p.ResponseInfo))
	str = append(str, sprintf("ServerReference", p.ServerReference))
	str = append(str, sprintf("ReasonString", p.ReasonString))
	str = append(str, sprintf("ReceiveMaximum", p.ReceiveMaximum))
	str = append(str, sprintf("TopicAliasMaximum", p.TopicAliasMaximum))
	str = append(str, sprintf("TopicAlias", p.TopicAlias))
	str = append(str, sprintf("MaximumQoS", p.MaximumQoS))
	str = append(str, sprintf("RetainAvailable", p.RetainAvailable))
	str = append(str, sprintf("User", p.User))
	str = append(str, sprintf("MaximumPacketSize", p.MaximumPacketSize))
	str = append(str, sprintf("WildcardSubAvailable", p.WildcardSubAvailable))
	str = append(str, sprintf("SubIDAvailable", p.SubIDAvailable))
	str = append(str, sprintf("SharedSubAvailable", p.SharedSubAvailable))
	return strings.Join(str, ", ")
}

func (p *Properties) PackWillProperties(bufw *bytes.Buffer) {
	newBufw := &bytes.Buffer{}
	defer func() {
		b, _ := DecodeRemainLength(newBufw.Len())
		bufw.Write(b)
		newBufw.WriteTo(bufw)
	}()
	if p == nil {
		return
	}
	propertyWriteByte(PropPayloadFormat, p.PayloadFormat, newBufw)
	propertyWriteUint32(PropMessageExpiry, p.MessageExpiry, newBufw)
	propertyWriteString(PropContentType, p.ContentType, newBufw)
	propertyWriteString(PropResponseTopic, p.ResponseTopic, newBufw)
	propertyWriteString(PropCorrelationData, p.CorrelationData, newBufw)
	propertyWriteUint32(PropWillDelayInterval, p.WillDelayInterval, newBufw)
	if len(p.User) != 0 {
		for _, v := range p.User {
			newBufw.WriteByte(PropUser)
			writeBinary(newBufw, v.K)
			writeBinary(newBufw, v.V)
		}
	}
}

// Pack takes all the defined properties for an Properties and produces
// a slice of bytes representing the wire format for the Info
func (p *Properties) Pack(bufw *bytes.Buffer, packetType byte) {
	newBufw := &bytes.Buffer{}
	defer func() {
		b, _ := DecodeRemainLength(newBufw.Len())
		bufw.Write(b)
		newBufw.WriteTo(bufw)
	}()
	if p == nil {
		return
	}
	propertyWriteByte(PropPayloadFormat, p.PayloadFormat, newBufw)
	propertyWriteUint32(PropMessageExpiry, p.MessageExpiry, newBufw)
	propertyWriteString(PropContentType, p.ContentType, newBufw)
	propertyWriteString(PropResponseTopic, p.ResponseTopic, newBufw)
	propertyWriteString(PropCorrelationData, p.CorrelationData, newBufw)

	if len(p.SubscriptionIdentifier) != 0 {
		for _, v := range p.SubscriptionIdentifier {
			newBufw.WriteByte(PropSubscriptionIdentifier)
			b, _ := DecodeRemainLength(int(v))
			newBufw.Write(b)
		}
	}
	propertyWriteUint32(PropSessionExpiryInterval, p.SessionExpiryInterval, newBufw)
	propertyWriteString(PropAssignedClientID, p.AssignedClientID, newBufw)
	propertyWriteUint16(PropServerKeepAlive, p.ServerKeepAlive, newBufw)
	propertyWriteString(PropAuthMethod, p.AuthMethod, newBufw)
	propertyWriteString(PropAuthData, p.AuthData, newBufw)
	propertyWriteByte(PropRequestProblemInfo, p.RequestProblemInfo, newBufw)
	propertyWriteUint32(PropWillDelayInterval, p.WillDelayInterval, newBufw)
	propertyWriteByte(PropRequestResponseInfo, p.RequestResponseInfo, newBufw)
	propertyWriteString(PropResponseInfo, p.ResponseInfo, newBufw)
	propertyWriteString(PropServerReference, p.ServerReference, newBufw)
	propertyWriteString(PropReasonString, p.ReasonString, newBufw)
	propertyWriteUint16(PropReceiveMaximum, p.ReceiveMaximum, newBufw)
	propertyWriteUint16(PropTopicAliasMaximum, p.TopicAliasMaximum, newBufw)
	propertyWriteUint16(PropTopicAlias, p.TopicAlias, newBufw)
	propertyWriteByte(PropMaximumQoS, p.MaximumQoS, newBufw)
	propertyWriteByte(PropRetainAvailable, p.RetainAvailable, newBufw)

	if len(p.User) != 0 {
		for _, v := range p.User {
			newBufw.WriteByte(PropUser)
			writeBinary(newBufw, v.K)
			writeBinary(newBufw, v.V)
		}
	}

	propertyWriteUint32(PropMaximumPacketSize, p.MaximumPacketSize, newBufw)
	propertyWriteByte(PropWildcardSubAvailable, p.WildcardSubAvailable, newBufw)
	propertyWriteByte(PropSubIDAvailable, p.SubIDAvailable, newBufw)
	propertyWriteByte(PropSharedSubAvailable, p.SharedSubAvailable, newBufw)
}

func (p *Properties) UnpackWillProperties(bufr *bytes.Buffer) error {
	var err error
	length, err := EncodeRemainLength(bufr)
	// 整个buffer最多只能读到length这么长
	if err != nil {
		return err
	}
	if length == 0 {
		return nil
	}
	newBufr := bytes.NewBuffer(bufr.Next(length))
	var propType byte
	for {
		if err != nil {
			return err
		}
		propType, err = newBufr.ReadByte()
		if err != nil && err != io.EOF {
			return err
		}
		if err == io.EOF {
			break
		}
		switch propType {
		case PropWillDelayInterval:
			p.WillDelayInterval, err = propertyReadUint32(p.WillDelayInterval, newBufr, propType, nil)
		case PropPayloadFormat:
			p.PayloadFormat, err = propertyReadBool(p.PayloadFormat, newBufr, propType)
		case PropMessageExpiry:
			p.MessageExpiry, err = propertyReadUint32(p.MessageExpiry, newBufr, propType, nil)
		case PropContentType:
			p.ContentType, err = propertyReadUTF8String(p.ContentType, newBufr, propType, nil)
		case PropResponseTopic:
			p.ResponseTopic, err = propertyReadUTF8String(p.ResponseTopic, newBufr, propType, func(u []byte) bool {
				return ValidTopicName(true, u) // [MQTT-3.3.2-14]
			})
		case PropCorrelationData:
			p.CorrelationData, err = propertyReadBinary(p.CorrelationData, newBufr, propType, nil)
		case PropUser:
			k, err := readUTF8String(true, newBufr)
			if err != nil {
				return codes.ErrMalformed
			}
			v, err := readUTF8String(true, newBufr)
			if err != nil {
				return codes.ErrMalformed
			}
			p.User = append(p.User, UserProperty{K: k, V: v})
		default:
			return codes.ErrMalformed
		}
	}
	return nil
}

// Unpack takes a buffer of bytes and reads out the defined properties
// filling in the appropriate entries in the struct, it returns the number
// of bytes used to store the Prop data and any error in decoding them
func (p *Properties) Unpack(bufr *bytes.Buffer, packetType byte) error {
	var err error
	length, err := EncodeRemainLength(bufr)
	// 整个buffer最多只能读到length这么长
	if err != nil {
		return err
	}
	if length == 0 {
		return nil
	}
	newBufr := bytes.NewBuffer(bufr.Next(length))
	var propType byte
	for {
		if err != nil {
			return err
		}
		propType, err = newBufr.ReadByte()
		if err != nil && err != io.EOF {
			return err
		}
		if err == io.EOF {
			break
		}

		if !ValidateID(packetType, propType) {
			return codes.ErrProtocol
		}

		switch propType {
		case PropPayloadFormat:
			p.PayloadFormat, err = propertyReadBool(p.PayloadFormat, newBufr, propType)
		case PropMessageExpiry:
			p.MessageExpiry, err = propertyReadUint32(p.MessageExpiry, newBufr, propType, nil)
		case PropContentType:
			p.ContentType, err = propertyReadUTF8String(p.ContentType, newBufr, propType, nil)
		case PropResponseTopic:
			p.ResponseTopic, err = propertyReadUTF8String(p.ResponseTopic, newBufr, propType, func(u []byte) bool {
				return ValidTopicName(true, u) // [MQTT-3.3.2-14]
			})
		case PropCorrelationData:
			p.CorrelationData, err = propertyReadBinary(p.CorrelationData, newBufr, propType, nil)
		case PropSubscriptionIdentifier:
			if len(p.SubscriptionIdentifier) != 0 {
				return codes.ErrProtocol
			}
			si, err := EncodeRemainLength(newBufr)
			if err != nil {
				return codes.ErrMalformed
			}
			if si == 0 {
				return codes.ErrProtocol
			}
			p.SubscriptionIdentifier = append(p.SubscriptionIdentifier, uint32(si))
		case PropSessionExpiryInterval:
			p.SessionExpiryInterval, err = propertyReadUint32(p.SessionExpiryInterval, newBufr, propType, nil)
		case PropAssignedClientID:
			p.AssignedClientID, err = propertyReadUTF8String(p.AssignedClientID, newBufr, propType, nil)
		case PropServerKeepAlive:
			p.ServerKeepAlive, err = propertyReadUint16(p.ServerKeepAlive, newBufr, propType, nil)
		case PropAuthMethod:
			p.AuthMethod, err = propertyReadUTF8String(p.AuthMethod, newBufr, propType, nil)
		case PropAuthData:
			p.AuthData, err = propertyReadUTF8String(p.AuthData, newBufr, propType, nil)
		case PropRequestProblemInfo:
			p.RequestProblemInfo, err = propertyReadBool(p.RequestProblemInfo, newBufr, propType)
		case PropWillDelayInterval:
			p.WillDelayInterval, err = propertyReadUint32(p.WillDelayInterval, newBufr, propType, nil)
		case PropRequestResponseInfo:
			p.RequestResponseInfo, err = propertyReadBool(p.RequestResponseInfo, newBufr, propType)
		case PropResponseInfo:
			p.ResponseInfo, err = propertyReadUTF8String(p.ResponseInfo, newBufr, propType, nil)
		case PropServerReference:
			p.ServerReference, err = propertyReadUTF8String(p.ServerReference, newBufr, propType, nil)
		case PropReasonString:
			p.ReasonString, err = propertyReadUTF8String(p.ReasonString, newBufr, propType, nil)
		case PropReceiveMaximum:
			p.ReceiveMaximum, err = propertyReadUint16(p.ReceiveMaximum, newBufr, propType, func(u uint16) bool {
				return u != 0
			})
		case PropTopicAliasMaximum:
			p.TopicAliasMaximum, err = propertyReadUint16(p.TopicAliasMaximum, newBufr, propType, nil)
		case PropTopicAlias:
			p.TopicAlias, err = propertyReadUint16(p.TopicAlias, newBufr, propType, func(u uint16) bool {
				return u != 0 // [MQTT-3.3.2-8]
			})
		case PropMaximumQoS:
			p.MaximumQoS, err = propertyReadBool(p.MaximumQoS, newBufr, propType)
		case PropRetainAvailable:
			p.RetainAvailable, err = propertyReadBool(p.RetainAvailable, newBufr, propType)
		case PropUser:
			k, err := readUTF8String(true, newBufr)
			if err != nil {
				return codes.ErrMalformed
			}
			v, err := readUTF8String(true, newBufr)
			if err != nil {
				return codes.ErrMalformed
			}
			p.User = append(p.User, UserProperty{K: k, V: v})
		case PropMaximumPacketSize:
			p.MaximumPacketSize, err = propertyReadUint32(p.MaximumPacketSize, newBufr, propType, func(u uint32) bool {
				return u != 0
			})
		case PropWildcardSubAvailable:
			p.WildcardSubAvailable, err = propertyReadBool(p.WildcardSubAvailable, newBufr, propType)
		case PropSubIDAvailable:
			p.SubIDAvailable, err = propertyReadBool(p.SubIDAvailable, newBufr, propType)
		case PropSharedSubAvailable:
			p.SharedSubAvailable, err = propertyReadBool(p.SharedSubAvailable, newBufr, propType)
		default:
			return codes.ErrMalformed
		}
	}
	if p.AuthData != nil && p.AuthMethod == nil {
		return codes.ErrMalformed
	}
	return nil
}

// ValidProperties is a map of the various properties and the
// PacketTypes that is valid for server to unpack.
var ValidProperties = map[byte]map[byte]struct{}{
	PropPayloadFormat:          {CONNECT: {}, PUBLISH: {}},
	PropMessageExpiry:          {CONNECT: {}, PUBLISH: {}},
	PropContentType:            {CONNECT: {}, PUBLISH: {}},
	PropResponseTopic:          {CONNECT: {}, PUBLISH: {}},
	PropCorrelationData:        {CONNECT: {}, PUBLISH: {}},
	PropSubscriptionIdentifier: {SUBSCRIBE: {}},
	PropSessionExpiryInterval:  {CONNECT: {}, CONNACK: {}, DISCONNECT: {}},
	PropAssignedClientID:       {CONNACK: {}},
	PropServerKeepAlive:        {CONNACK: {}},
	PropAuthMethod:             {CONNECT: {}, CONNACK: {}, AUTH: {}},
	PropAuthData:               {CONNECT: {}, CONNACK: {}, AUTH: {}},
	PropRequestProblemInfo:     {CONNECT: {}},
	PropWillDelayInterval:      {CONNECT: {}},
	PropRequestResponseInfo:    {CONNECT: {}},
	PropResponseInfo:           {CONNACK: {}},
	PropServerReference:        {CONNACK: {}, DISCONNECT: {}},
	PropReasonString:           {CONNACK: {}, PUBACK: {}, PUBREC: {}, PUBREL: {}, PUBCOMP: {}, SUBACK: {}, UNSUBACK: {}, DISCONNECT: {}, AUTH: {}},
	PropReceiveMaximum:         {CONNECT: {}, CONNACK: {}},
	PropTopicAliasMaximum:      {CONNECT: {}, CONNACK: {}},
	PropTopicAlias:             {PUBLISH: {}},
	PropMaximumQoS:             {CONNACK: {}},
	PropRetainAvailable:        {CONNACK: {}},
	PropUser:                   {CONNECT: {}, CONNACK: {}, PUBLISH: {}, PUBACK: {}, PUBREC: {}, PUBREL: {}, PUBCOMP: {}, SUBSCRIBE: {}, UNSUBSCRIBE: {}, SUBACK: {}, UNSUBACK: {}, DISCONNECT: {}, AUTH: {}},
	PropMaximumPacketSize:      {CONNECT: {}, CONNACK: {}},
	PropWildcardSubAvailable:   {CONNACK: {}},
	PropSubIDAvailable:         {CONNACK: {}},
	PropSharedSubAvailable:     {CONNACK: {}},
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

func propertyReadBool(i *byte, r *bytes.Buffer, propType byte) (*byte, error) {
	if i != nil {
		return nil, codes.ErrProtocol
	}
	o, err := r.ReadByte()
	if err != nil {
		return nil, codes.ErrMalformed
	}
	if o != 0 && o != 1 {
		return nil, codes.ErrProtocol
	}
	return &o, nil
}

func propertyReadUint32(i *uint32, r *bytes.Buffer, propType byte,
	validate func(u uint32) bool) (*uint32, error) {
	if i != nil {
		return nil, errMorethanOnce(propType)
	}
	o, err := readUint32(r)
	if err != nil {
		return nil, codes.ErrMalformed
	}
	if validate != nil {
		if !validate(o) {
			return nil, codes.ErrProtocol
		}
	}
	return &o, nil
}

func propertyReadUint16(i *uint16, r *bytes.Buffer, propType byte, validate func(u uint16) bool) (*uint16, error) {
	if i != nil {
		return nil, codes.ErrProtocol
	}
	o, err := readUint16(r)
	if err != nil {
		return nil, codes.ErrMalformed
	}
	if validate != nil {
		if !validate(o) {
			return nil, codes.ErrProtocol
		}
	}
	return &o, nil
}
func propertyReadUTF8String(i []byte, r *bytes.Buffer, propType byte, validate func(u []byte) bool) (b []byte, err error) {
	if i != nil {
		return nil, errMorethanOnce(propType)
	}
	o, err := readUTF8String(true, r)
	if err != nil {
		return nil, err
	}
	if validate != nil {
		if !validate(o) {
			return nil, codes.ErrProtocol
		}
	}
	return o, nil
}
func propertyReadBinary(i []byte, r *bytes.Buffer, propType byte,
	validate func(u []byte) bool) (b []byte, err error) {
	if i != nil {
		return nil, errMorethanOnce(propType)
	}
	o, err := readUTF8String(false, r)
	if err != nil {
		return nil, codes.ErrMalformed
	}
	if validate != nil {
		if !validate(o) {
			return nil, codes.ErrProtocol
		}
	}
	return o, nil
}

func propertyWriteByte(t byte, i *byte, w *bytes.Buffer) {
	if i != nil {
		w.Write([]byte{t, *i})
	}
}

func propertyWriteUint16(t byte, i *uint16, w *bytes.Buffer) {
	if i != nil {
		w.WriteByte(t)
		writeUint16(w, *i)
	}
}
func propertyWriteUint32(t byte, i *uint32, w *bytes.Buffer) {
	if i != nil {
		w.WriteByte(t)
		writeUint32(w, *i)
	}
}

func propertyWriteString(t byte, i []byte, w *bytes.Buffer) {
	if i != nil {
		w.WriteByte(t)
		writeBinary(w, i)
	}
}
