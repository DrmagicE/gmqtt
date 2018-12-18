package packets

import (
	"bytes"
	"reflect"
	"testing"

	"io"
)

func TestReadConnectPacketErr(t *testing.T) {
	//[MQTT-3.1.2-3],服务端必须验证CONNECT控制报文的保留标志位（第0位）是否为0，如果不为0必须断开客户端连接
	b := []byte{16, 12, 0, 4, 77, 81, 84, 84, 04, 01, 00, 02, 31, 32}
	buf := bytes.NewBuffer(b)
	connectPacket, err := NewReader(buf).ReadPacket()
	if packet, ok := connectPacket.(*Connect); ok {
		if packet != nil {
			t.Fatalf("ReadPacket() packet error,want <nil>,got %v", connectPacket)
		}
		if err != ErrInvalConnFlags {
			t.Fatalf("ReadPacket() err error,want %s,got %s", ErrInvalConnFlags, err)
		}

	} else {
		t.Fatalf("Packet type error,want %v,got %v", reflect.TypeOf(&Connect{}), reflect.TypeOf(connectPacket))
	}
}

func TestReadConnectPacket(t *testing.T) {
	connectPacketBytes := bytes.NewBuffer([]byte{16, 52, //FixHeader
		0, 4, 77, 81, 84, 84, //Protocol Name
		4,    //Protocol Level
		206,  //11001110 Connect Flags username=1 password=1 Will Flag = 1 WillRetain=0 Will Qos=1 CleanSession=1
		0, 0, //KeepAlive
		0, 0, //clientID len=0
		0, 4, 116, 101, 115, 116, //Will Topic
		0, 12, 84, 101, 115, 116, 32, 80, 97, 121, 108, 111, 97, 100, //Will Message
		0, 8, 116, 101, 115, 116, 117, 115, 101, 114, //Username
		0, 8, 116, 101, 115, 116, 112, 97, 115, 115, //password
	})

	packet, err := NewReader(connectPacketBytes).ReadPacket()
	if err != nil {
		t.Fatalf("Unexpected error: %s", err.Error())
	}

	if cp, ok := packet.(*Connect); ok {
		if !bytes.Equal(cp.ProtocolName, []byte("MQTT")) {
			t.Fatalf("protocolName error,want %s, got %s", "MQTT", string(cp.ProtocolName))
		}

		if cp.ProtocolLevel != 0x04 {
			t.Fatalf("ProtocolLevel error,want 4, got %v", cp.ProtocolLevel)
		}

		if !cp.UsernameFlag {
			t.Fatalf("UsernameFlag error,want %t, got %t", true, cp.UsernameFlag)
		}
		if !cp.PasswordFlag {
			t.Fatalf("PasswordFlag error,want %t, got %t", true, cp.PasswordFlag)
		}
		if !cp.WillFlag {
			t.Fatalf("WillFlag error,want %t, got %t", true, cp.WillFlag)
		}
		if cp.WillRetain {
			t.Fatalf("WillRetain error,want %t, got %t", false, cp.WillRetain)
		}
		if cp.WillQos != 1 {
			t.Fatalf("WillRetain error,want %d, got %d", 1, cp.WillQos)
		}
		if !cp.CleanSession {
			t.Fatalf("CleanSession error,want %t, got %t", true, cp.CleanSession)
		}
		if cp.KeepAlive != 0 {
			t.Fatalf("KeepAlive error,want %d, got %d", 0, cp.KeepAlive)
		}
		if len(cp.ClientID) != 0 {
			t.Fatalf("ClientID error,want [], got %d", cp.ClientID)
		}
		if !bytes.Equal(cp.WillTopic, []byte{116, 101, 115, 116}) {
			t.Fatalf("WillTopic error,want %v, got %v", []byte{116, 101, 115, 116}, cp.WillTopic)
		}
		if !bytes.Equal(cp.WillMsg, []byte{84, 101, 115, 116, 32, 80, 97, 121, 108, 111, 97, 100}) {
			t.Fatalf("WillMsg error,want %v, got %v", []byte{84, 101, 115, 116, 32, 80, 97, 121, 108, 111, 97, 100}, cp.WillMsg)
		}
		if !bytes.Equal(cp.Username, []byte{116, 101, 115, 116, 117, 115, 101, 114}) {
			t.Fatalf("Username error,want %v, got %v", []byte{116, 101, 115, 116, 117, 115, 101, 114}, cp.Username)
		}
		if !bytes.Equal(cp.Password, []byte{116, 101, 115, 116, 112, 97, 115, 115}) {
			t.Fatalf("Password error,want %v, got %v", []byte{116, 101, 115, 116, 112, 97, 115, 115}, cp.Password)
		}
	} else {
		t.Fatalf("Packet type error,want %v,got %v", reflect.TypeOf(&Connect{}), reflect.TypeOf(packet))
	}
}

func TestConnect_NewConnackPacket_RejectConnection(t *testing.T) {
	connectPacketBytes := bytes.NewBuffer([]byte{16, 53, //FixHeader
		0, 4, 77, 81, 84, 84, //Protocol Name
		2,    //Invalid Protocol Level
		206,  //Connect Flags 11001100 username=1 password=1 Will Flag = 1 WillRetain=0 Will Qos=1 CleanSession=1
		0, 0, //KeepAlive
		0, 1, 0x31, //clientID
		0, 4, 116, 101, 115, 116, //Will Topic
		0, 12, 84, 101, 115, 116, 32, 80, 97, 121, 108, 111, 97, 100, //Will Message
		0, 8, 116, 101, 115, 116, 117, 115, 101, 114, //Username
		0, 8, 116, 101, 115, 116, 112, 97, 115, 115, //password
	})

	packet, _ := NewReader(connectPacketBytes).ReadPacket()
	connectPacket := packet.(*Connect)

	connackPacket := connectPacket.NewConnackPacket(true)
	if connackPacket.Code != connectPacket.AckCode {
		t.Fatalf("connect.AckCode:%d and connack.Code:%d should be equal", connectPacket.AckCode, connackPacket.Code)
	}
	if connackPacket.SessionPresent != 0 {
		t.Fatalf("SessionPresent error,want %d, got %d", 0, connackPacket.SessionPresent)
	}
}

func TestConnect_NewConnackPacket_Success(t *testing.T) {
	connectPacketBytes := bytes.NewBuffer([]byte{16, 53, //FixHeader
		0, 4, 77, 81, 84, 84, //Protocol Name
		4,    // Protocol Level
		204,  //Connect Flags 11001100 username=1 password=1 Will Flag = 1 WillRetain=0 Will Qos=1 CleanSession=0
		0, 0, //KeepAlive
		0, 1, 0x31, //clientID
		0, 4, 116, 101, 115, 116, //Will Topic
		0, 12, 84, 101, 115, 116, 32, 80, 97, 121, 108, 111, 97, 100, //Will Message
		0, 8, 116, 101, 115, 116, 117, 115, 101, 114, //Username
		0, 8, 116, 101, 115, 116, 112, 97, 115, 115, //password
	})

	packet, err := NewReader(connectPacketBytes).ReadPacket()
	if err != nil {
		t.Fatalf("Unexpected error: %s", err.Error())
	}

	connectPacket := packet.(*Connect)
	connackPacket := connectPacket.NewConnackPacket(true)
	if connackPacket.Code != CodeAccepted {
		t.Fatalf("connack.Code should be %d, but %d", CodeAccepted, connackPacket.Code)
	}

	if connackPacket.Code != connectPacket.AckCode {
		t.Fatalf("connect.AckCode:%d and connack.Code:%d should be equal", connectPacket.AckCode, connackPacket.Code)
	}
	if connackPacket.SessionPresent != 1 {
		t.Fatalf("SessionPresent error,want %d, got %d", 1, connackPacket.SessionPresent)
	}
}

func TestWriteConnect(t *testing.T) {
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
		cleanSession  bool
		keepAlive     uint16
		clientID      []byte
		username      []byte
		password      []byte
	}{
		{protocolLevel: 0x04, usernameFlag: true, protocolName: []byte("MQTT"), passwordFlag: true, willRetain: true, willQos: 2, willFlag: true, willTopic: []byte("messageTopic1"), willMsg: []byte("messageContent1"), cleanSession: true, keepAlive: 60, clientID: []byte("client1"), username: []byte("admin1"), password: []byte("1236")},
		{protocolLevel: 0x04, usernameFlag: false, protocolName: []byte("MQTT"), passwordFlag: false, willRetain: true, willQos: 1, willFlag: true, willTopic: []byte("messageTopic2"), willMsg: []byte("messageContent2"), cleanSession: true, keepAlive: 60, clientID: []byte("client2"), username: []byte(""), password: []byte("")},
		{protocolLevel: 0x04, usernameFlag: true, protocolName: []byte("MQTT"), passwordFlag: true, willRetain: false, willQos: 0, willFlag: false, willTopic: []byte(""), willMsg: []byte(""), cleanSession: true, keepAlive: 60, clientID: []byte("client3"), username: []byte("admin2"), password: []byte("1234")},
		{protocolLevel: 0x04, usernameFlag: true, protocolName: []byte("MQTT"), passwordFlag: true, willRetain: false, willQos: 0, willFlag: false, willTopic: []byte(""), willMsg: []byte(""), cleanSession: false, keepAlive: 60, clientID: []byte("client4"), username: []byte("admin3"), password: []byte("123")},
		{protocolLevel: 0x04, usernameFlag: true, protocolName: []byte("MQTT"), passwordFlag: true, willRetain: false, willQos: 0, willFlag: false, willTopic: []byte(""), willMsg: []byte(""), cleanSession: false, keepAlive: 60, clientID: []byte("client5"), username: []byte("admin4"), password: []byte("1235")},
		{protocolLevel: 0x04, usernameFlag: false, protocolName: []byte("MQTT"), passwordFlag: false, willRetain: false, willQos: 0, willFlag: false, willTopic: []byte(""), willMsg: []byte(""), cleanSession: true, keepAlive: 60, clientID: []byte(""), username: []byte(""), password: []byte("")},
		{protocolLevel: 0x04, usernameFlag: false, protocolName: []byte("MQTT"), passwordFlag: false, willRetain: true, willQos: 2, willFlag: true, willTopic: []byte("messageTopic3"), willMsg: []byte("messageContent3"), cleanSession: true, keepAlive: 60, clientID: []byte("client6"), username: []byte(""), password: []byte("")},
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
			CleanSession:  v.cleanSession,
			KeepAlive:     v.keepAlive,
			ClientID:      v.clientID,
			Username:      v.username,
			Password:      v.password,
		}
		err := NewWriter(buf).WriteAndFlush(con)
		if err != nil {
			t.Fatalf("unexpected error: %s,%v", err.Error(), string(v.clientID))
		}
		packet, err := NewReader(buf).ReadPacket()
		if err != nil {
			t.Fatalf("unexpected error: %s,%v", err.Error(), string(v.clientID))
		}
		n, err := buf.ReadByte()
		if err != io.EOF {
			t.Fatalf("ReadByte() error,want io.EOF,got %s and %v bytes", err, n)
		}
		if p, ok := packet.(*Connect); ok {
			if !bytes.Equal(p.WillTopic, con.WillTopic) {
				t.Fatalf("TopicName error,want %v, got %v", con.WillTopic, p.WillTopic)
			}
			if !bytes.Equal(p.ClientID, con.ClientID) {
				t.Fatalf("ClientID error,want %v, got %v", con.ClientID, p.ClientID)
			}
			if !bytes.Equal(p.Username, con.Username) {
				t.Fatalf("Username error,want %v, got %v", con.Username, p.Username)
			}
			if !bytes.Equal(p.Password, con.Password) {
				t.Fatalf("Password error,want %v, got %v", con.Password, p.Password)
			}
			if !bytes.Equal(p.WillMsg, con.WillMsg) {
				t.Fatalf("WillMsg error,want %v, got %v", con.WillMsg, p.WillMsg)
			}
			if !bytes.Equal(p.ProtocolName, con.ProtocolName) {
				t.Fatalf("ProtocolName error,want %v, got %v", con.ProtocolName, p.ProtocolName)
			}
			if p.ProtocolLevel != con.ProtocolLevel {
				t.Fatalf("ProtocolLevel error,want %v, got %v", con.ProtocolLevel, p.ProtocolLevel)
			}
			if p.WillRetain != con.WillRetain {
				t.Fatalf("WillRetain error,want %v, got %v", con.WillRetain, p.WillRetain)
			}
			if p.KeepAlive != con.KeepAlive {
				t.Fatalf("KeepAlive error,want %v, got %v", con.KeepAlive, p.KeepAlive)
			}
			if p.WillFlag != con.WillFlag {
				t.Fatalf("WillFlag error,want %v, got %v", con.WillFlag, p.WillFlag)
			}
			if p.WillQos != con.WillQos {
				t.Fatalf("WillQos error,want %v, got %v", con.WillQos, p.WillQos)
			}
			if p.CleanSession != con.CleanSession {
				t.Fatalf("CleanSession error,want %v, got %v", con.CleanSession, p.CleanSession)
			}
			if p.UsernameFlag != con.UsernameFlag {
				t.Fatalf("UsernameFlag error,want %v, got %v", con.UsernameFlag, p.UsernameFlag)
			}
			if p.PasswordFlag != con.PasswordFlag {
				t.Fatalf("PasswordFlag error,want %v, got %v", con.PasswordFlag, p.PasswordFlag)
			}
		} else {
			t.Fatalf("Packet type error,want %v,got %v", reflect.TypeOf(&Connect{}), reflect.TypeOf(packet))
		}
	}

}
