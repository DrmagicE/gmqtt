package packets

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
	"unicode/utf8"
)

// Error type
var (
	ErrInvalPacketType           = errors.New("invalid Packet Type")
	ErrInvalFlags                = errors.New("invalid Flags")
	ErrInvalConnFlags            = errors.New("invalid Connect Flags")
	ErrInvalConnAcknowledgeFlags = errors.New("invalid Connect Acknowledge Flags")
	ErrInvalSessionPresent       = errors.New("invalid Session Present")
	ErrInvalRemainLength         = errors.New("Malformed Remaining Length")
	ErrInvalProtocolName         = errors.New("invalid protocol name")
	ErrInvalUtf8                 = errors.New("invalid utf-8 string")
	ErrInvalTopicName            = errors.New("invalid topic name")
	ErrInvalTopicFilter          = errors.New("invalid topic filter")
	ErrInvalQos                  = errors.New("invalid Qos,only support qos0 | qos1 | qos2")
	ErrInvalWillQos              = errors.New("invalid Will Qos")
	ErrInvalWillRetain           = errors.New("invalid Will Retain")
	ErrInvalUTF8String           = errors.New("invalid utf-8 string")
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
)

// Flag in the FixHeader
const (
	FLAG_RESERVED    = 0
	FLAG_SUBSCRIBE   = 2
	FLAG_UNSUBSCRIBE = 2
	FLAG_PUBREL      = 2
)

// QoS levels & Subscribe failure
const (
	QOS_0             uint8 = 0x00
	QOS_1             uint8 = 0x01
	QOS_2             uint8 = 0x02
	SUBSCRIBE_FAILURE       = 0x80
)

//PacketID is the type of packet identifier
type PacketID = uint16

//Max & min packet ID
const (
	MAX_PACKET_ID PacketID = 65535
	MIN_PACKET_ID PacketID = 1
)

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

// FixHeader represents the FixHeader of the MQTT packet
type FixHeader struct {
	PacketType   byte
	Flags        byte
	RemainLength int
}

// Topic represents the MQTT Topic
type Topic struct {
	Qos  uint8
	Name string
}

// Reader is used to read data from bufio.Reader and create MQTT packet instance.
type Reader struct {
	bufr *bufio.Reader
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
		return &Reader{bufr: bufr}
	}
	return &Reader{bufr: bufio.NewReaderSize(r, 2048)}
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
	packet, err := NewPacket(fh, r.bufr)
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
		return nil, ErrInvalRemainLength
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
func EncodeRemainLength(r *bufio.Reader) (int, error) {
	var i int
	multiplier := 1
	var value int
	buf := make([]byte, 0, 1)
	for {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		buf = append(buf, b)
		value += int(buf[i]&byte(127)) * multiplier
		multiplier *= 128
		if multiplier > 128*128*128*128 {
			return 0, ErrInvalRemainLength
		}
		if buf[i]&128 != 0 { //高位不等于零还有下一个字节
			i++
		} else { //否则返回value
			return value, nil
		}
	}
}

// EncodeUTF8String encodes the bytes into UTF-8 encoded strings, returns the encoded bytes, bytes size and error.
func EncodeUTF8String(buf []byte) (b []byte, size int, err error) {
	buflen := len(buf)
	if buflen > 65535 {
		return nil, 0, ErrInvalUTF8String
	}
	//length := int(binary.BigEndian.Uint16(buf[0:2]))
	bufw := make([]byte, 2, 2+buflen)
	binary.BigEndian.PutUint16(bufw, uint16(buflen))
	bufw = append(bufw, buf...)
	return bufw, 2 + buflen, nil
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
func NewPacket(fh *FixHeader, r io.Reader) (Packet, error) {
	switch fh.PacketType {
	case CONNECT:
		return NewConnectPacket(fh, r)
	case CONNACK:
		return NewConnackPacket(fh, r)
	case PUBLISH:
		return NewPublishPacket(fh, r)
	case PUBACK:
		return NewPubackPacket(fh, r)
	case PUBREC:
		return NewPubrecPacket(fh, r)
	case PUBREL:
		return NewPubrelPacket(fh, r)
	case PUBCOMP:
		return NewPubcompPacket(fh, r)
	case SUBSCRIBE:
		return NewSubscribePacket(fh, r)
	case SUBACK:
		return NewSubackPacket(fh, r)
	case UNSUBSCRIBE:
		return NewUnsubscribePacket(fh, r)
	case PINGREQ:
		return NewPingreqPacket(fh, r)
	case DISCONNECT:
		return NewDisConnectPackets(fh, r)
	case UNSUBACK:
		return NewUnsubackPacket(fh, r)
	case PINGRESP:
		return NewPingrespPacket(fh, r)
	default:
		return nil, ErrInvalPacketType

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

// ValidTopicName 验证主题名是否合法  [MQTT-4.7.1-1]
//
// ValidTopicName returns whether the bytes is a valid topic name.[MQTT-4.7.1-1].
func ValidTopicName(p []byte) bool {
	if len(p) == 0 {
		return false
	}
	for {
		ru, size := utf8.DecodeRune(p)
		if !utf8.ValidRune(ru) {
			return false
		}
		if size == 1 {
			//主题名不允许使用通配符
			if p[0] == byte('+') || p[0] == byte('#') {
				return false
			}
		}
		if size == 0 {
			return true
		}
		p = p[size:]
	}
}

// ValidTopicFilter 验证主题过滤器是否合法
//
// ValidTopicFilter  returns whether the bytes is a valid topic filter. [MQTT-4.7.1-2]  [MQTT-4.7.1-3]
func ValidTopicFilter(p []byte) bool {
	if len(p) == 0 {
		return false
	}
	var prevByte byte //前一个字节
	var isSetPrevByte bool
	for {
		ru, size := utf8.DecodeRune(p)
		if !utf8.ValidRune(ru) {
			return false
		}
		if size == 1 && isSetPrevByte {
			// + 前（如果有前后字节）,一定是'/' [MQTT-4.7.1-2]  [MQTT-4.7.1-3]
			plen := len(p)
			if (p[0] == byte('+') || p[0] == byte('#')) && prevByte != byte('/') {
				return false
			}
			if p[0] == byte('#') && plen != 1 { // #一定是最后一个字符
				return false
			}
			if plen > 1 { // p[0] 不是最后一个字节
				if p[0] == byte('+') && p[1] != byte('/') { // + 后（如果有字节）,一定是 '/'
					return false
				}
			}
		}
		if size == 0 {
			return true
		}
		prevByte = p[0]
		isSetPrevByte = true
		p = p[size:]
	}
}

// TopicMatch 返回topic和topic filter是否
//
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
