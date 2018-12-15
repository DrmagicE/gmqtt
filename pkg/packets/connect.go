package packets

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

type Connect struct {
	FixHeader *FixHeader
	//variable header
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
	CleanSession bool
	KeepAlive    uint16 //如果非零，1.5倍时间没收到则断开连接[MQTT-3.1.2-24]
	//if set
	ClientId []byte
	Username []byte
	Password []byte
	AckCode  uint8 //ack的返回码
}

func (c *Connect) String() string {
	return fmt.Sprintf("Connect, ProtocolLevel: %v, UsernameFlag: %v, PasswordFlag: %v, ProtocolName: %s, CleanSession: %v, KeepAlive: %v, ClientId: %s, Username: %s, Password: %s"+
		", WillFlag: %v, WillRetain: %v, WillQos: %v, WillMsg: %s",
		c.ProtocolLevel, c.UsernameFlag, c.PasswordFlag, c.ProtocolName, c.CleanSession, c.KeepAlive, c.ClientId, c.Username, c.Password, c.WillFlag, c.WillRetain, c.WillQos, c.WillMsg)
}

func (c *Connect) Pack(w io.Writer) error {
	var err error
	c.FixHeader = &FixHeader{PacketType: CONNECT, Flags: FLAG_RESERVED}
	remainLength := 10 + len(c.ClientId) + 2
	if c.WillFlag {
		remainLength += len(c.WillTopic) + 2 + len(c.WillMsg) + 2
	}
	if c.UsernameFlag {
		remainLength += len(c.Username) + 2
	}
	if c.PasswordFlag {
		remainLength += len(c.Password) + 2
	}
	c.FixHeader.RemainLength = remainLength
	err = c.FixHeader.Pack(w)
	if err != nil {
		return err
	}
	lenProtocolName := []byte{0, 4}
	w.Write(lenProtocolName)
	w.Write(c.ProtocolName)

	w.Write([]byte{c.ProtocolLevel})
	var (
		usenameFlag  = 0
		passwordFlag = 0
		willRetain   = 0
		willFlag     = 0
		willQos      = 0
		cleanSession = 0
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
	if c.CleanSession {
		cleanSession = 2
	}
	connFlag := usenameFlag | passwordFlag | willRetain | willFlag | willQos | cleanSession | reserved
	w.Write([]byte{uint8(connFlag)})

	keepAlive := make([]byte, 2)
	binary.BigEndian.PutUint16(keepAlive, c.KeepAlive)
	w.Write(keepAlive)

	clienIdByte, _, erro := EncodeUTF8String(c.ClientId)
	if erro != nil {
		return erro
	}
	_, err = w.Write(clienIdByte)
	if c.WillFlag {
		willTopicByte, _, _ := EncodeUTF8String(c.WillTopic)
		w.Write(willTopicByte)
		willMsgByte, _, erro := EncodeUTF8String(c.WillMsg)
		_, err = w.Write(willMsgByte)
		if erro != nil {
			return erro
		}
	}
	if c.UsernameFlag {
		usernameByte, _, erro := EncodeUTF8String(c.Username)
		_, err = w.Write(usernameByte)
		if erro != nil {
			return erro
		}
	}
	if c.PasswordFlag {
		passwordByte, _, erro := EncodeUTF8String(c.Password)
		_, err = w.Write(passwordByte)
		if erro != nil {
			return erro
		}
	}
	return err
}

func (c *Connect) Unpack(r io.Reader) error {
	restBuffer := make([]byte, c.FixHeader.RemainLength)
	_, err := io.ReadFull(r, restBuffer)
	if err != nil {
		return err
	}
	if !bytes.Equal(restBuffer[0:6], []byte{0, 4, 77, 81, 84, 84}) { //protocol name

		return ErrInvalProtocolName // [MQTT-3.1.2-1] 不符合的protocol name直接关闭
	}
	c.ProtocolName = []byte{77, 81, 84, 84}
	c.ProtocolLevel = restBuffer[6]
	if c.ProtocolLevel != 0x04 {
		c.AckCode = CODE_UNACCEPTABLE_PROTOCOL_VERSION // [MQTT-3.1.2-2]
	}
	connectFlags := restBuffer[7]

	reserved := 1 & connectFlags
	if reserved != 0 { //[MQTT-3.1.2-3]
		return ErrInvalConnFlags
	}
	c.CleanSession = (1 & (connectFlags >> 1)) > 0
	c.WillFlag = (1 & (connectFlags >> 2)) > 0
	c.WillQos = 3 & (connectFlags >> 3)
	if !c.WillFlag && c.WillQos != 0 { //[MQTT-3.1.2-11]
		return ErrInvalWillQos
	}
	c.WillRetain = (1 & (connectFlags >> 5)) > 0
	if !c.WillFlag && c.WillRetain { //[MQTT-3.1.2-11]
		return ErrInvalWillRetain
	}
	c.PasswordFlag = (1 & (connectFlags >> 6)) > 0
	c.UsernameFlag = (1 & (connectFlags >> 7)) > 0
	c.KeepAlive = binary.BigEndian.Uint16(restBuffer[8:10])
	return c.unpackPayload(restBuffer[10:])
}

func (c *Connect) unpackPayload(restBuffer []byte) error {
	var vh []byte
	var size int
	var err error
	vh, size, err = DecodeUTF8String(restBuffer)
	if err != nil {
		return err
	}
	restBuffer = restBuffer[size:]
	c.ClientId = vh
	if len(c.ClientId) == 0 && c.CleanSession == false { //[MQTT-3.1.3-7]
		c.AckCode = CODE_IDENTIFIER_REJECTED //[MQTT-3.1.3-8]
	}

	if c.WillFlag {
		vh, size, err = DecodeUTF8String(restBuffer)
		if err != nil {
			return err
		}
		restBuffer = restBuffer[size:]
		c.WillTopic = vh
		vh, size, err = DecodeUTF8String(restBuffer)
		if err != nil {
			return err
		}
		restBuffer = restBuffer[size:]
		c.WillMsg = vh
	}

	if c.UsernameFlag {
		vh, size, err = DecodeUTF8String(restBuffer)
		if err != nil {
			return err
		}
		restBuffer = restBuffer[size:]
		c.Username = vh
	}
	if c.PasswordFlag {
		vh, size, err = DecodeUTF8String(restBuffer)
		if err != nil {
			return err
		}
		restBuffer = restBuffer[size:]
		c.Password = vh
	}
	return nil
}
func NewConnectPacket(fh *FixHeader, r io.Reader) (*Connect, error) {
	//b1 := buffer[0] //一定是16
	p := &Connect{FixHeader: fh}
	//判断 标志位 flags 是否合法[MQTT-2.2.2-2]
	if fh.Flags != FLAG_RESERVED {
		return nil, ErrInvalFlags
	}
	err := p.Unpack(r)
	if err != nil {
		return nil, err
	}
	return p, err
}

//构建一个connect包
func (c *Connect) NewConnackPacket(sessionReuse bool) *Connack {
	//b1 := buffer[0] //一定是16
	ack := &Connack{}
	ack.Code = c.AckCode
	if c.CleanSession == true { //[MQTT-3.2.2-1]
		ack.SessionPresent = 0
	} else {
		if sessionReuse {
			ack.SessionPresent = 1 //[MQTT-3.2.2-2]
		} else {
			ack.SessionPresent = 0 //[MQTT-3.2.2-3]
		}
	}
	if ack.Code != CODE_ACCEPTED {
		ack.SessionPresent = 0
	}
	return ack
}
