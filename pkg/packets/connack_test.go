package packets

import (
	"bytes"
	"reflect"
	"testing"
)

func TestWriteConnack(t *testing.T) {
	var tt = []struct {
		connack *Connack
		want    []byte
	}{
		{connack: &Connack{Code: CodeAccepted, SessionPresent: 1}, want: []byte{0x20, 2, 1, 0}},
		{connack: &Connack{Code: CodeAccepted, SessionPresent: 0}, want: []byte{0x20, 2, 0, 0}},
		{connack: &Connack{Code: CodeNotAuthorized, SessionPresent: 0}, want: []byte{0x20, 2, 0, 0x05}},
	}
	for _, v := range tt {
		connack := v.connack
		buf := bytes.NewBuffer(make([]byte, 0, 2048))
		err := NewWriter(buf).WriteAndFlush(connack)
		if err != nil {
			t.Fatalf("unexpected error: %s", err.Error())
		}
		if !bytes.Equal(buf.Bytes(), v.want) {
			t.Fatalf("write error,want %v, got %v", v.want, buf.Bytes())
		}
	}

}

func TestReadConnackPacket(t *testing.T) {
	connackPacketBytes := bytes.NewBuffer([]byte{32, 2, //FixHeader
		0, //SessionPresent
		1, //Code
	})
	packet, err := NewReader(connackPacketBytes).ReadPacket()
	if err != nil {
		t.Fatalf("Unexpected error: %s", err.Error())
	}
	if cp, ok := packet.(*Connack); ok {
		if cp.SessionPresent != 0 {
			t.Fatalf("WillRetain error,want %d, got %d", 0, cp.SessionPresent)
		}
		if cp.Code != 1 {
			t.Fatalf("WillRetain error,want %d, got %d", 1, cp.Code)
		}
	} else {
		t.Fatalf("Packet type error,want %v,got %v", reflect.TypeOf(&Connack{}), reflect.TypeOf(packet))
	}
}
