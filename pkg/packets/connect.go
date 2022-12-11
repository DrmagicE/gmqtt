package packets

import (
	"bytes"
	"fmt"
	"io"

	"github.com/DrmagicE/gmqtt/pkg/codes"
)

// Connect represents the MQTT Connect  packet
type Connect struct {
	Version   Version
	FixHeader *FixHeader
	//Variable header
	ProtocolLevel byte
	//Connect Flags
	UsernameFlag bool
	ProtocolName []byte
	PasswordFlag bool
	WillRetain   bool
	WillQos      uint8
	WillFlag     bool
	WillTopic    []byte
	WillMsg      []byte
	CleanStart   bool
	KeepAlive    uint16 //如果非零，1.5倍时间没收到则断开连接[MQTT-3.1.2-24]
	//if set
	ClientID []byte
	Username []byte
	Password []byte

	Properties     *Properties
	WillProperties *Properties
}

func (c *Connect) String() string {
	return fmt.Sprintf("Connect, Version: %v,"+"ProtocolLevel: %v, UsernameFlag: %v, PasswordFlag: %v, ProtocolName: %s, CleanStart: %v, KeepAlive: %v, ClientID: %s, Username: %s, Password: %s"+
		", WillFlag: %v, WillRetain: %v, WillQos: %v, WillMsg: %s, Properties: %s, WillProperties: %s",
		c.Version, c.ProtocolLevel, c.UsernameFlag, c.PasswordFlag, c.ProtocolName, c.CleanStart, c.KeepAlive, c.ClientID, c.Username, c.Password, c.WillFlag, c.WillRetain, c.WillQos, c.WillMsg, c.Properties, c.WillProperties)
}

// Pack encodes the packet struct into bytes and writes it into io.Writer.
func (c *Connect) Pack(w io.Writer) error {
	var err error
	c.FixHeader = &FixHeader{PacketType: CONNECT, Flags: FlagReserved}

	bufw := &bytes.Buffer{}
	bufw.Write([]byte{0x00, 0x04})
	bufw.Write(c.ProtocolName)
	bufw.WriteByte(c.ProtocolLevel)
	// write flag
	var (
		usenameFlag  = 0
		passwordFlag = 0
		willRetain   = 0
		willFlag     = 0
		willQos      = 0
		CleanStart   = 0
		reserved     = 0
	)
	if c.UsernameFlag {
		usenameFlag = 128
	}
	if c.PasswordFlag {
		passwordFlag = 64
	}
	if c.WillRetain {
		willRetain = 32
	}
	if c.WillQos == 1 {
		willQos = 8
	} else if c.WillQos == 2 {
		willQos = 16
	}
	if c.WillFlag {
		willFlag = 4
	}
	if c.CleanStart {
		CleanStart = 2
	}
	connFlag := usenameFlag | passwordFlag | willRetain | willFlag | willQos | CleanStart | reserved
	bufw.Write([]byte{uint8(connFlag)})
	writeUint16(bufw, c.KeepAlive)

	if c.Version == Version5 {
		c.Properties.Pack(bufw, CONNECT)
	}
	clienIDByte, _, err := EncodeUTF8String(c.ClientID)

	if err != nil {
		return err
	}
	bufw.Write(clienIDByte)

	if c.WillFlag {
		if c.Version == Version5 {
			c.WillProperties.PackWillProperties(bufw)
		}
		willTopicByte, _, err := EncodeUTF8String(c.WillTopic)
		if err != nil {
			return err
		}
		bufw.Write(willTopicByte)
		willMsgByte, _, err := EncodeUTF8String(c.WillMsg)
		if err != nil {
			return err
		}
		bufw.Write(willMsgByte)
	}
	if c.UsernameFlag {
		usernameByte, _, err := EncodeUTF8String(c.Username)
		if err != nil {
			return err
		}
		bufw.Write(usernameByte)
	}
	if c.PasswordFlag {
		passwordByte, _, err := EncodeUTF8String(c.Password)
		if err != nil {
			return err
		}
		bufw.Write(passwordByte)
	}

	c.FixHeader.RemainLength = bufw.Len()
	err = c.FixHeader.Pack(w)
	if err != nil {
		return err
	}
	_, err = bufw.WriteTo(w)
	return err
}

// Unpack read the packet bytes from io.Reader and decodes it into the packet struct.
func (c *Connect) Unpack(r io.Reader) (err error) {
	restBuffer := make([]byte, c.FixHeader.RemainLength)
	_, err = io.ReadFull(r, restBuffer)
	if err != nil {
		return err
	}
	bufr := bytes.NewBuffer(restBuffer)
	c.ProtocolName, err = readUTF8String(false, bufr)
	if err != nil {
		return err
	}
	c.ProtocolLevel, err = bufr.ReadByte()
	if err != nil {
		return codes.ErrMalformed
	}
	c.Version = c.ProtocolLevel
	if name, ok := version2protoName[c.ProtocolLevel]; !ok {
		return codes.NewError(codes.V3UnacceptableProtocolVersion)
	} else if !bytes.Equal(c.ProtocolName, name) {
		return codes.NewError(codes.UnsupportedProtocolVersion)
	}

	connectFlags, err := bufr.ReadByte()
	if err != nil {
		return codes.ErrMalformed
	}
	reserved := 1 & connectFlags
	if reserved != 0 { //[MQTT-3.1.2-3]
		return codes.ErrMalformed
	}
	c.CleanStart = (1 & (connectFlags >> 1)) > 0
	c.WillFlag = (1 & (connectFlags >> 2)) > 0
	c.WillQos = 3 & (connectFlags >> 3)
	if !c.WillFlag && c.WillQos != 0 { //[MQTT-3.1.2-11]
		return codes.ErrMalformed
	}
	c.WillRetain = (1 & (connectFlags >> 5)) > 0
	if !c.WillFlag && c.WillRetain { //[MQTT-3.1.2-11]
		return codes.ErrMalformed
	}
	c.PasswordFlag = (1 & (connectFlags >> 6)) > 0
	c.UsernameFlag = (1 & (connectFlags >> 7)) > 0
	c.KeepAlive, err = readUint16(bufr)
	if err != nil {
		return codes.ErrMalformed
	}

	if c.Version == Version5 {
		// resolve properties
		c.Properties = &Properties{}
		c.WillProperties = &Properties{}
		if err := c.Properties.Unpack(bufr, CONNECT); err != nil {
			return err
		}
	}
	return c.unpackPayload(bufr)
}

func (c *Connect) unpackPayload(bufr *bytes.Buffer) error {
	var err error
	c.ClientID, err = readUTF8String(true, bufr)
	if err != nil {
		return err
	}

	if IsVersion3X(c.Version) && len(c.ClientID) == 0 && !c.CleanStart { // v311 [MQTT-3.1.3-7]
		return codes.NewError(codes.V3IdentifierRejected) // v311 //[MQTT-3.1.3-8]
	}

	if c.WillFlag {
		if c.Version == Version5 {
			err := c.WillProperties.UnpackWillProperties(bufr)
			if err != nil {
				return err
			}
		}
		c.WillTopic, err = readUTF8String(true, bufr)
		if err != nil {
			return err
		}
		c.WillMsg, err = readUTF8String(false, bufr)
		if err != nil {
			return err
		}
	}

	if c.UsernameFlag {
		c.Username, err = readUTF8String(true, bufr)
		if err != nil {
			return err
		}
	}
	if c.PasswordFlag {
		c.Password, err = readUTF8String(true, bufr)
		if err != nil {
			return err
		}
	}
	return nil
}

// NewConnectPacket returns a Connect instance by the given FixHeader and io.Reader
func NewConnectPacket(fh *FixHeader, version Version, r io.Reader) (*Connect, error) {
	//b1 := buffer[0] //一定是16
	p := &Connect{FixHeader: fh, Version: version}
	//判断 标志位 flags 是否合法[MQTT-2.2.2-2]
	if fh.Flags != FlagReserved {
		return nil, codes.ErrMalformed
	}
	err := p.Unpack(r)
	if err != nil {
		return nil, err
	}
	return p, err
}

// NewConnackPacket returns the Connack struct which is the ack packet of the Connect packet.
func (c *Connect) NewConnackPacket(code codes.Code, sessionReuse bool) *Connack {
	ack := &Connack{Code: code, Version: c.Version}
	if !c.CleanStart && sessionReuse && code == codes.Success {
		ack.SessionPresent = true //[MQTT-3.2.2-2]
	}
	return ack
}
