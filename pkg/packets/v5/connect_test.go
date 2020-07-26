package v5

import (
	"bytes"
	"io"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadConnectPacketErr(t *testing.T) {
	//[MQTT-3.1.2-3],服务端必须验证CONNECT控制报文的保留标志位（第0位）是否为0，如果不为0必须断开客户端连接
	a := assert.New(t)

	b := []byte{16, 12, 0, 4, 'M', 'Q', 'T', 'T', 05, 01, 00, 02, 31, 32}
	buf := bytes.NewBuffer(b)
	connectPacket, err := NewReader(buf).ReadPacket()
	if packet, ok := connectPacket.(*Connect); ok {
		a.Nil(packet)
		a.Equal(ErrInvalConnFlags, err.(errCode).error)
	} else {
		t.Fatalf("Packet type error,want %v,got %v", reflect.TypeOf(&Connect{}), reflect.TypeOf(connectPacket))
	}
}

func TestReadWriteConnectPacket(t *testing.T) {
	a := assert.New(t)
	pb := []byte{16, 67, //FixHeader
		0, 4, 77, 81, 84, 84, //Protocol Name
		5,    //Protocol Level
		206,  //11001110 Connect Flags username=1 password=1 Will Flag = 1 WillRetain=0 Will Qos=1 CleanStart=1
		0, 0, //KeepAlive
		8,                // properties len
		0x11, 0, 0, 0, 1, // Session Expiry Interval = 1
		0x21, 0, 1, // Receive Maximum = 1
		0, 0, //clientID len=0
		5,                // Will Properties
		0x18, 0, 0, 0, 1, // Will Delay Interval = 1
		0, 4, 116, 101, 115, 116, //Will Topic
		0, 12, 84, 101, 115, 116, 32, 80, 97, 121, 108, 111, 97, 100, //Will Message
		0, 8, 116, 101, 115, 116, 117, 115, 101, 114, //Username
		0, 8, 116, 101, 115, 116, 112, 97, 115, 115, //password
	}
	connectPacketBytes := bytes.NewBuffer(pb)

	var packet Packet
	var err error
	t.Run("unpack", func(t *testing.T) {
		packet, err = NewReader(connectPacketBytes).ReadPacket()
		a.Nil(err)

		if cp, ok := packet.(*Connect); ok {
			a.Equal([]byte("MQTT"), cp.ProtocolName)
			a.EqualValues(0x05, cp.ProtocolLevel)
			a.True(cp.UsernameFlag)
			a.True(cp.PasswordFlag)
			a.True(cp.WillFlag)
			a.False(cp.WillRetain)
			a.EqualValues(1, cp.WillQos)
			a.True(cp.CleanStart)
			a.True(cp.PasswordFlag)
			a.True(cp.WillFlag)
			a.False(cp.WillRetain)
			a.EqualValues(1, cp.WillQos)
			a.EqualValues(0, cp.KeepAlive)
			a.Len(cp.ClientID, 0)

			a.Equal([]byte{116, 101, 115, 116}, cp.WillTopic)
			a.Equal([]byte{84, 101, 115, 116, 32, 80, 97, 121, 108, 111, 97, 100}, cp.WillMsg)
			a.Equal([]byte{116, 101, 115, 116, 117, 115, 101, 114}, cp.Username)
			a.Equal([]byte{116, 101, 115, 116, 112, 97, 115, 115}, cp.Password)

			a.EqualValues(1, *cp.Properties.SessionExpiryInterval)
			a.EqualValues(1, *cp.Properties.ReceiveMaximum)
			a.EqualValues(1, *cp.WillProperties.WillDelayInterval)
		} else {
			t.Fatalf("Packet type error,want %v,got %v", reflect.TypeOf(&Connect{}), reflect.TypeOf(packet))
		}
	})

	t.Run("pack", func(t *testing.T) {
		bufw := &bytes.Buffer{}
		a.Nil(packet.Pack(bufw))
		a.EqualValues(pb, bufw.Bytes())
	})
}
func TestWriteConnect(t *testing.T) {
	a := assert.New(t)
	var tt = []struct {
		protocolLevel byte
		usernameFlag  bool
		protocolName  []byte
		passwordFlag  bool
		willRetain    bool
		willQos       uint8
		willFlag      bool
		willTopic     []byte
		willMsg       []byte
		CleanStart    bool
		keepAlive     uint16
		clientID      []byte
		username      []byte
		password      []byte
	}{
		{protocolLevel: 0x05, usernameFlag: true, protocolName: []byte("MQTT"), passwordFlag: true, willRetain: true, willQos: 2, willFlag: true, willTopic: []byte("messageTopic1"), willMsg: []byte("messageContent1"), CleanStart: true, keepAlive: 60, clientID: []byte("client1"), username: []byte("admin1"), password: []byte("1236")},
		{protocolLevel: 0x05, usernameFlag: false, protocolName: []byte("MQTT"), passwordFlag: false, willRetain: true, willQos: 1, willFlag: true, willTopic: []byte("messageTopic2"), willMsg: []byte("messageContent2"), CleanStart: true, keepAlive: 60, clientID: []byte("client2"), username: []byte(""), password: []byte("")},
		{protocolLevel: 0x05, usernameFlag: true, protocolName: []byte("MQTT"), passwordFlag: true, willRetain: false, willQos: 0, willFlag: false, willTopic: []byte(""), willMsg: []byte(""), CleanStart: true, keepAlive: 60, clientID: []byte("client3"), username: []byte("admin2"), password: []byte("1234")},
		{protocolLevel: 0x05, usernameFlag: true, protocolName: []byte("MQTT"), passwordFlag: true, willRetain: false, willQos: 0, willFlag: false, willTopic: []byte(""), willMsg: []byte(""), CleanStart: false, keepAlive: 60, clientID: []byte("client4"), username: []byte("admin3"), password: []byte("123")},
		{protocolLevel: 0x05, usernameFlag: true, protocolName: []byte("MQTT"), passwordFlag: true, willRetain: false, willQos: 0, willFlag: false, willTopic: []byte(""), willMsg: []byte(""), CleanStart: false, keepAlive: 60, clientID: []byte("client5"), username: []byte("admin4"), password: []byte("1235")},
		{protocolLevel: 0x05, usernameFlag: false, protocolName: []byte("MQTT"), passwordFlag: false, willRetain: false, willQos: 0, willFlag: false, willTopic: []byte(""), willMsg: []byte(""), CleanStart: true, keepAlive: 60, clientID: []byte(""), username: []byte(""), password: []byte("")},
		{protocolLevel: 0x05, usernameFlag: false, protocolName: []byte("MQTT"), passwordFlag: false, willRetain: true, willQos: 2, willFlag: true, willTopic: []byte("messageTopic3"), willMsg: []byte("messageContent3"), CleanStart: true, keepAlive: 60, clientID: []byte("client6"), username: []byte(""), password: []byte("")},
	}

	for _, v := range tt {
		b := make([]byte, 0, 2048)
		buf := bytes.NewBuffer(b)
		con := &Connect{
			ProtocolLevel: v.protocolLevel,
			UsernameFlag:  v.usernameFlag,
			ProtocolName:  v.protocolName,
			PasswordFlag:  v.passwordFlag,
			WillRetain:    v.willRetain,
			WillQos:       v.willQos,
			WillFlag:      v.willFlag,
			WillTopic:     v.willTopic,
			WillMsg:       v.willMsg,
			CleanStart:    v.CleanStart,
			KeepAlive:     v.keepAlive,
			ClientID:      v.clientID,
			Username:      v.username,
			Password:      v.password,
		}
		err := NewWriter(buf).WriteAndFlush(con)
		a.Nil(err)
		packet, err := NewReader(buf).ReadPacket()
		a.Nil(err)
		_, err = buf.ReadByte()
		a.Equal(io.EOF, err)
		if p, ok := packet.(*Connect); ok {
			a.ElementsMatch(con.WillTopic, p.WillTopic)
			a.ElementsMatch(con.ClientID, p.ClientID)
			a.ElementsMatch(con.Username, p.Username)
			a.ElementsMatch(con.Password, p.Password)
			a.ElementsMatch(con.WillMsg, p.WillMsg)
			a.ElementsMatch(con.ProtocolName, p.ProtocolName)
			a.EqualValues(con.ProtocolLevel, p.ProtocolLevel)
			a.EqualValues(con.WillRetain, p.WillRetain)
			a.EqualValues(con.KeepAlive, p.KeepAlive)
			a.EqualValues(con.WillFlag, p.WillFlag)
			a.EqualValues(con.WillQos, p.WillQos)
			a.EqualValues(con.CleanStart, p.CleanStart)
			a.EqualValues(con.UsernameFlag, p.UsernameFlag)
			a.EqualValues(con.PasswordFlag, p.PasswordFlag)
		} else {
			t.Fatalf("Packet type error,want %v,got %v", reflect.TypeOf(&Connect{}), reflect.TypeOf(packet))
		}
	}

}
