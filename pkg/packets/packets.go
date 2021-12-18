package packets

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"unicode/utf8"

	"github.com/DrmagicE/gmqtt/pkg/codes"
)

// Error type
var (
	ErrInvalUTF8String = errors.New("invalid utf-8 string")
)

// MQTT Version
type Version = byte

type QoS = byte

var version2protoName = map[Version][]byte{
	Version31:  {'M', 'Q', 'I', 's', 'd', 'p'},
	Version311: {'M', 'Q', 'T', 'T'},
	Version5:   {'M', 'Q', 'T', 'T'},
}

const (
	Version31  Version = 0x03
	Version311 Version = 0x04
	Version5   Version = 0x05
	// The maximum packet size of a MQTT packet
	MaximumSize = 268435456
)

//Packet type
const (
	RESERVED = iota
	CONNECT
	CONNACK
	PUBLISH
	PUBACK
	PUBREC
	PUBREL
	PUBCOMP
	SUBSCRIBE
	SUBACK
	UNSUBSCRIBE
	UNSUBACK
	PINGREQ
	PINGRESP
	DISCONNECT
	AUTH
)

// QoS levels & Subscribe failure
const (
	Qos0             uint8 = 0x00
	Qos1             uint8 = 0x01
	Qos2             uint8 = 0x02
	SubscribeFailure       = 0x80
)

// Flag in the FixHeader
const (
	FlagReserved    = 0
	FlagSubscribe   = 2
	FlagUnsubscribe = 2
	FlagPubrel      = 2
)

//PacketID is the type of packet identifier
type PacketID = uint16

//Max & min packet ID
const (
	MaxPacketID PacketID = 65535
	MinPacketID PacketID = 1
)

type PayloadFormat = byte

const (
	PayloadFormatBytes  PayloadFormat = 0
	PayloadFormatString PayloadFormat = 1
)

func IsVersion3X(v Version) bool {
	return v == Version311 || v == Version31
}

func IsVersion5(v Version) bool {
	return v == Version5
}

// FixHeader represents the FixHeader of the MQTT packet
type FixHeader struct {
	PacketType   byte
	Flags        byte
	RemainLength int
}

// Packet defines the interface for structs intended to hold
// decoded MQTT packets, either from being read or before being
// written
type Packet interface {
	// Pack encodes the packet struct into bytes and writes it into io.Writer.
	Pack(w io.Writer) error
	// Unpack read the packet bytes from io.Reader and decodes it into the packet struct
	Unpack(r io.Reader) error
	// String is mainly used in logging, debugging and testing.
	String() string
}

// Topic represents the MQTT Topic
type Topic struct {
	SubOptions
	Name string
}

// SubOptions is the subscription option of subscriptions.
// For details: https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Subscription_Options
type SubOptions struct {
	// Qos is the QoS level of the subscription.
	// 0 = At most once delivery
	// 1 = At least once delivery
	// 2 = Exactly once delivery
	Qos uint8
	// RetainHandling specifies whether retained messages are sent when the subscription is established.
	// 0 = Send retained messages at the time of the subscribe
	// 1 = Send retained messages at subscribe only if the subscription does not currently exist
	// 2 = Do not send retained messages at the time of the subscribe
	RetainHandling byte
	// NoLocal is the No Local option.
	//  If the value is 1, Application Messages MUST NOT be forwarded to a connection with a ClientID equal to the ClientID of the publishing connection
	NoLocal bool
	// RetainAsPublished is the Retain As Published option.
	// If 1, Application Messages forwarded using this subscription keep the RETAIN flag they were published with.
	// If 0, Application Messages forwarded using this subscription have the RETAIN flag set to 0. Retained messages sent when the subscription is established have the RETAIN flag set to 1.
	RetainAsPublished bool
}

// Reader is used to read data from bufio.Reader and create MQTT packet instance.
type Reader struct {
	bufr    *bufio.Reader
	version Version
}

// Writer is used to encode MQTT packet into bytes and write it to bufio.Writer.
type Writer struct {
	bufw *bufio.Writer
}

// Flush writes any buffered data to the underlying io.Writer.
func (w *Writer) Flush() error {
	return w.bufw.Flush()
}

// ReadWriter warps Reader and Writer.
type ReadWriter struct {
	*Reader
	*Writer
}

// NewReader returns a new Reader.
func NewReader(r io.Reader) *Reader {
	if bufr, ok := r.(*bufio.Reader); ok {
		return &Reader{bufr: bufr, version: Version311}
	}
	return &Reader{bufr: bufio.NewReaderSize(r, 2048), version: Version311}
}

func (r *Reader) SetVersion(version Version) {
	r.version = version
}

// NewWriter returns a new Writer.
func NewWriter(w io.Writer) *Writer {
	if bufw, ok := w.(*bufio.Writer); ok {
		return &Writer{bufw: bufw}
	}
	return &Writer{bufw: bufio.NewWriterSize(w, 2048)}
}

// ReadPacket reads data from Reader and returns a  Packet instance.
// If any errors occurs, returns nil, error
func (r *Reader) ReadPacket() (Packet, error) {
	first, err := r.bufr.ReadByte()
	if err != nil {
		return nil, err
	}
	fh := &FixHeader{PacketType: first >> 4, Flags: first & 15} //设置FixHeader
	length, err := EncodeRemainLength(r.bufr)
	if err != nil {
		return nil, err
	}
	fh.RemainLength = length
	packet, err := NewPacket(fh, r.version, r.bufr)
	if err != nil {
		return nil, err
	}
	if p, ok := packet.(*Connect); ok {
		r.version = p.Version
	}
	return packet, err
}

// WritePacket writes the packet bytes to the Writer.
// Call Flush after WritePacket to flush buffered data to the underlying io.Writer.
func (w *Writer) WritePacket(packet Packet) error {
	err := packet.Pack(w.bufw)
	if err != nil {
		return err
	}
	return nil
}

// WriteRaw write raw bytes to the Writer.
// Call Flush after WriteRaw to flush buffered data to the underlying io.Writer.
func (w *Writer) WriteRaw(b []byte) error {
	_, err := w.bufw.Write(b)
	if err != nil {
		return err
	}
	return nil
}

// WriteAndFlush writes and flush the packet bytes to the underlying io.Writer.
func (w *Writer) WriteAndFlush(packet Packet) error {
	err := packet.Pack(w.bufw)
	if err != nil {
		return err
	}
	return w.Flush()
}

// Pack encodes the FixHeader struct into bytes and writes it into io.Writer.
func (fh *FixHeader) Pack(w io.Writer) error {
	var err error
	b := make([]byte, 1)
	packetType := fh.PacketType << 4
	b[0] = packetType | fh.Flags
	length, err := DecodeRemainLength(fh.RemainLength)
	if err != nil {
		return err
	}
	b = append(b, length...)
	_, err = w.Write(b)
	return err
}

//DecodeRemainLength 将remain length 转成byte表示
//
//DecodeRemainLength puts the length int into bytes
func DecodeRemainLength(length int) ([]byte, error) {
	var result []byte
	if length < 128 {
		result = make([]byte, 1)
	} else if length < 16384 {
		result = make([]byte, 2)
	} else if length < 2097152 {
		result = make([]byte, 3)
	} else if length < 268435456 {
		result = make([]byte, 4)
	} else {
		return nil, codes.ErrMalformed
	}
	var i int
	for {
		encodedByte := length % 128
		length = length / 128
		// if there are more data to encode, set the top bit of this byte
		if length > 0 {
			encodedByte = encodedByte | 128
		}
		result[i] = byte(encodedByte)
		i++
		if length <= 0 {
			break
		}
	}
	return result, nil
}

// EncodeRemainLength 读remainLength,如果格式错误返回 error
//
// EncodeRemainLength reads the remain length bytes from bufio.Reader and returns length int.
func EncodeRemainLength(r io.ByteReader) (int, error) {
	var vbi uint32
	var multiplier uint32
	for {
		digit, err := r.ReadByte()
		if err != nil && err != io.EOF {
			return 0, err
		}
		vbi |= uint32(digit&127) << multiplier
		if vbi > 268435455 {
			return 0, codes.ErrMalformed
		}
		if (digit & 128) == 0 {
			break
		}
		multiplier += 7
	}
	return int(vbi), nil
}

// EncodeUTF8String encodes the bytes into UTF-8 encoded strings, returns the encoded bytes, bytes size and error.
func EncodeUTF8String(buf []byte) (b []byte, size int, err error) {
	buflen := len(buf)
	if buflen > 65535 {
		return nil, 0, codes.ErrMalformed
	}
	//length := int(binary.BigEndian.Uint16(buf[0:2]))
	bufw := make([]byte, 2, 2+buflen)
	binary.BigEndian.PutUint16(bufw, uint16(buflen))
	bufw = append(bufw, buf...)
	return bufw, 2 + buflen, nil
}

func readUint16(r *bytes.Buffer) (uint16, error) {
	if r.Len() < 2 {
		return 0, codes.ErrMalformed
	}
	return binary.BigEndian.Uint16(r.Next(2)), nil
}

func writeUint16(w *bytes.Buffer, i uint16) {
	w.WriteByte(byte(i >> 8))
	w.WriteByte(byte(i))
}
func writeUint32(w *bytes.Buffer, i uint32) {
	w.WriteByte(byte(i >> 24))
	w.WriteByte(byte(i >> 16))
	w.WriteByte(byte(i >> 8))
	w.WriteByte(byte(i))
}

func readUint32(r *bytes.Buffer) (uint32, error) {
	if r.Len() < 4 {
		return 0, codes.ErrMalformed
	}
	return binary.BigEndian.Uint32(r.Next(4)), nil
}

func readUTF8String(mustUTF8 bool, r *bytes.Buffer) (b []byte, err error) {
	if r.Len() < 2 {
		return nil, codes.ErrMalformed
	}
	length := int(binary.BigEndian.Uint16(r.Next(2)))
	if r.Len() < length {
		return nil, codes.ErrMalformed
	}
	payload := r.Next(length)
	if mustUTF8 {
		if !ValidUTF8(payload) {
			return nil, codes.ErrMalformed
		}
	}
	return payload, nil
}

// the length of s cannot be greater than 65535
func writeUTF8String(w *bytes.Buffer, s []byte) {
	writeUint16(w, uint16(len(s)))
	w.Write(s)
}
func writeBinary(w *bytes.Buffer, b []byte) {
	writeUint16(w, uint16(len(b)))
	w.Write(b)
}

// DecodeUTF8String decodes the  UTF-8 encoded strings into bytes, returns the decoded bytes, bytes size and error.
func DecodeUTF8String(buf []byte) (b []byte, size int, err error) {
	buflen := len(buf)
	if buflen < 2 {
		return nil, 0, ErrInvalUTF8String
	}
	length := int(binary.BigEndian.Uint16(buf[0:2]))
	if buflen < length+2 {
		return nil, 0, ErrInvalUTF8String
	}
	payload := buf[2 : length+2]
	if !ValidUTF8(payload) {
		return nil, 0, ErrInvalUTF8String
	}

	return payload, length + 2, nil
}

// NewPacket returns a packet representing the decoded MQTT packet and an error.
func NewPacket(fh *FixHeader, version Version, r io.Reader) (Packet, error) {
	switch fh.PacketType {
	case CONNECT:
		return NewConnectPacket(fh, version, r)
	case CONNACK:
		return NewConnackPacket(fh, version, r)
	case PUBLISH:
		return NewPublishPacket(fh, version, r)
	case PUBACK:
		return NewPubackPacket(fh, version, r)
	case PUBREC:
		return NewPubrecPacket(fh, version, r)
	case PUBREL:
		return NewPubrelPacket(fh, r)
	case PUBCOMP:
		return NewPubcompPacket(fh, version, r)
	case SUBSCRIBE:
		return NewSubscribePacket(fh, version, r)
	case SUBACK:
		return NewSubackPacket(fh, version, r)
	case UNSUBSCRIBE:
		return NewUnsubscribePacket(fh, version, r)
	case PINGREQ:
		return NewPingreqPacket(fh, r)
	case DISCONNECT:
		return NewDisConnectPackets(fh, version, r)
	case UNSUBACK:
		return NewUnsubackPacket(fh, version, r)
	case PINGRESP:
		return NewPingrespPacket(fh, r)
	case AUTH:
		return NewAuthPacket(fh, r)
	default:
		return nil, codes.ErrProtocol

	}
}

// ValidUTF8 验证是否utf8
//
// ValidUTF8 returns whether the given bytes is in UTF-8 form.
func ValidUTF8(p []byte) bool {
	for {
		if len(p) == 0 {
			return true
		}
		ru, size := utf8.DecodeRune(p)
		if ru >= '\u0000' && ru <= '\u001f' { //[MQTT-1.5.3-2]
			return false
		}
		if ru >= '\u007f' && ru <= '\u009f' {
			return false
		}
		if ru == utf8.RuneError {
			return false
		}
		if !utf8.ValidRune(ru) {
			return false
		}
		if size == 0 {
			return true
		}
		p = p[size:]
	}
}

// ValidTopicName returns whether the bytes is a valid non-shared topic filter.[MQTT-4.7.1-1].
func ValidTopicName(mustUTF8 bool, p []byte) bool {
	for len(p) > 0 {
		ru, size := utf8.DecodeRune(p)
		if mustUTF8 && ru == utf8.RuneError {
			return false
		}
		if size == 1 {
			//主题名不允许使用通配符
			if p[0] == byte('+') || p[0] == byte('#') {
				return false
			}
		}
		p = p[size:]
	}
	return true
}

// ValidV5Topic returns whether the given bytes is a valid MQTT V5 topic
func ValidV5Topic(p []byte) bool {
	if len(p) == 0 {
		return false
	}
	if bytes.HasPrefix(p, []byte("$share/")) {
		if len(p) < 9 {
			return false
		}
		if p[7] != '/' {
			subp := p[7:]
			for len(subp) > 0 {
				ru, size := utf8.DecodeRune(subp)
				if ru == utf8.RuneError {
					return false
				}
				if size == 1 {
					if subp[0] == '/' {
						return ValidTopicFilter(true, subp[1:])
					}
					if subp[0] == byte('+') || subp[0] == byte('#') {
						return false
					}
				}
				subp = subp[size:]
			}

		}
		return false
	}
	return ValidTopicFilter(true, p)
}

// ValidTopicFilter 验证主题过滤器是否合法
//
// ValidTopicFilter  returns whether the bytes is a valid topic filter. [MQTT-4.7.1-2]  [MQTT-4.7.1-3]
// ValidTopicFilter 验证主题过滤器是否合法
//
// ValidTopicFilter  returns whether the bytes is a valid topic filter. [MQTT-4.7.1-2]  [MQTT-4.7.1-3]
func ValidTopicFilter(mustUTF8 bool, p []byte) bool {
	if len(p) == 0 {
		return false
	}
	var prevByte byte //前一个字节
	var isSetPrevByte bool

	for len(p) > 0 {
		ru, size := utf8.DecodeRune(p)
		if mustUTF8 && ru == utf8.RuneError {
			return false
		}
		plen := len(p)
		if p[0] == byte('#') && plen != 1 { // #一定是最后一个字符
			return false
		}
		if size == 1 && isSetPrevByte {
			// + 前（如果有前后字节）,一定是'/' [MQTT-4.7.1-2]  [MQTT-4.7.1-3]
			if (p[0] == byte('+') || p[0] == byte('#')) && prevByte != byte('/') {
				return false
			}

			if plen > 1 { // p[0] 不是最后一个字节
				if p[0] == byte('+') && p[1] != byte('/') { // + 后（如果有字节）,一定是 '/'
					return false
				}
			}
		}
		prevByte = p[0]
		isSetPrevByte = true
		p = p[size:]
	}
	return true
}

// TopicMatch returns whether the topic and topic filter is matched.
func TopicMatch(topic []byte, topicFilter []byte) bool {
	var spos int
	var tpos int
	var sublen int
	var topiclen int
	var multilevelWildcard bool //是否是多层的通配符
	sublen = len(topicFilter)
	topiclen = len(topic)
	if sublen == 0 || topiclen == 0 {
		return false
	}
	if (topicFilter[0] == '$' && topic[0] != '$') || (topic[0] == '$' && topicFilter[0] != '$') {
		return false
	}
	for {
		//e.g. ([]byte("foo/bar"),[]byte("foo/+/#")
		if spos < sublen && tpos <= topiclen {
			if tpos != topiclen && topicFilter[spos] == topic[tpos] { // sublen是订阅 topiclen是发布,首字母匹配
				if tpos == topiclen-1 { //遍历到topic的最后一个字节
					/* Check for e.g. foo matching foo/# */
					if spos == sublen-3 && topicFilter[spos+1] == '/' && topicFilter[spos+2] == '#' {
						return true
					}
				}
				spos++
				tpos++
				if spos == sublen && tpos == topiclen { //长度相等，内容相同，匹配
					return true
				} else if tpos == topiclen && spos == sublen-1 && topicFilter[spos] == '+' {
					//订阅topic比发布topic多一个字节，并且多出来的内容是+ ,比如: sub: foo/+ ,topic: foo/
					if spos > 0 && topicFilter[spos-1] != '/' {
						return false
					}
					spos++
					return true
				}
			} else {
				if topicFilter[spos] == '+' { //sub 和 topic 内容不匹配了
					spos++
					for { //找到topic的下一个主题分割符
						if tpos < topiclen && topic[tpos] != '/' {
							tpos++
						} else {
							break
						}
					}
					if tpos == topiclen && spos == sublen { //都遍历完了,返回true
						return true
					}
				} else if topicFilter[spos] == '#' {
					multilevelWildcard = true
					return true
				} else {
					/* Check for e.g. foo/bar matching foo/+/# */
					if spos > 0 && spos+2 == sublen && tpos == topiclen && topicFilter[spos-1] == '+' && topicFilter[spos] == '/' && topicFilter[spos+1] == '#' {
						multilevelWildcard = true
						return true
					}
					return false
				}
			}
		} else {
			break
		}
	}
	if !multilevelWildcard && (tpos < topiclen || spos < sublen) {
		return false
	}
	return false
}

// TotalBytes returns how many bytes of the packet
func TotalBytes(p Packet) uint32 {
	var header *FixHeader
	switch pt := p.(type) {
	case *Auth:
		header = pt.FixHeader
	case *Connect:
		header = pt.FixHeader
	case *Connack:
		header = pt.FixHeader
	case *Disconnect:
		header = pt.FixHeader
	case *Pingreq:
		header = pt.FixHeader
	case *Pingresp:
		header = pt.FixHeader
	case *Puback:
		header = pt.FixHeader
	case *Pubcomp:
		header = pt.FixHeader
	case *Publish:
		header = pt.FixHeader
	case *Pubrec:
		header = pt.FixHeader
	case *Pubrel:
		header = pt.FixHeader
	case *Suback:
		header = pt.FixHeader
	case *Subscribe:
		header = pt.FixHeader
	case *Unsuback:
		header = pt.FixHeader
	case *Unsubscribe:
		header = pt.FixHeader
	}
	if header == nil {
		return 0
	}
	var headerLength uint32
	if header.RemainLength <= 127 {
		headerLength = 2
	} else if header.RemainLength <= 16383 {
		headerLength = 3
	} else if header.RemainLength <= 2097151 {
		headerLength = 4
	} else if header.RemainLength <= 268435455 {
		headerLength = 5
	}
	return headerLength + uint32(header.RemainLength)

}
