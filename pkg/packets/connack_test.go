package packets

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DrmagicE/gmqtt/pkg/codes"
)

func TestWriteConnack_V5(t *testing.T) {
	a := assert.New(t)
	var tt = []struct {
		connack *Connack
		want    []byte
	}{
		{connack: &Connack{Version: Version5, Code: codes.Success, SessionPresent: true}, want: []byte{0x20, 3, 1, 0, 0}},
		{connack: &Connack{Version: Version5, Code: codes.Success, SessionPresent: false}, want: []byte{0x20, 3, 0, 0, 0}},
		{connack: &Connack{Version: Version5, Code: codes.NotAuthorized, SessionPresent: false}, want: []byte{0x20, 3, 0, codes.NotAuthorized, 0}},
		// with Properties
		{connack: &Connack{
			Version:        Version5,
			Code:           codes.Success,
			SessionPresent: false,
			Properties: &Properties{
				SessionExpiryInterval: uint32P(1),
				ReceiveMaximum:        uint16P(2),
				MaximumQoS:            byteP(1),
			},
		}, want: []byte{0x20, 13, 0, codes.Success,
			10, // properties length
			0x11, 0, 0, 0, 1,
			0x21, 0, 2,
			0x24, 1,
		}},
	}

	for _, v := range tt {
		connack := v.connack
		buf := bytes.NewBuffer(make([]byte, 0, 2048))
		err := NewWriter(buf).WriteAndFlush(connack)
		a.Nil(err)
		if err != nil {
			t.Fatalf("unexpected error: %s", err.Error())
		}
		a.Equal(v.want, buf.Bytes())
	}

}

func TestReadConnackPacket_V5(t *testing.T) {
	a := assert.New(t)
	connackPacketBytes := bytes.NewBuffer([]byte{32, 8, //FixHeader
		0, //SessionPresent
		0, //Code
		5, // property length
		0x11, 0, 0, 0, 1,
	})
	r := NewReader(connackPacketBytes)
	r.SetVersion(Version5)
	packet, err := r.ReadPacket()
	if err != nil {
		t.Fatalf("Unexpected error: %s", err.Error())
	}
	if cp, ok := packet.(*Connack); ok {
		a.Equal(false, cp.SessionPresent)
		a.EqualValues(0, cp.Code)
	} else {
		t.Fatalf("Packet type error,want %v,got %v", reflect.TypeOf(&Connack{}), reflect.TypeOf(packet))
	}
}

func TestWriteConnack_V311(t *testing.T) {
	a := assert.New(t)
	var tt = []struct {
		connack *Connack
		want    []byte
	}{
		{connack: &Connack{Version: Version311, Code: codes.V3Accepted, SessionPresent: true}, want: []byte{0x20, 2, 1, 0}},
		{connack: &Connack{Version: Version311, Code: codes.V3Accepted, SessionPresent: false}, want: []byte{0x20, 2, 0, 0}},
		{connack: &Connack{Version: Version311, Code: codes.V3NotAuthorized, SessionPresent: false}, want: []byte{0x20, 2, 0, 0x05}},
	}
	for _, v := range tt {
		connack := v.connack
		buf := bytes.NewBuffer(make([]byte, 0, 2048))
		err := NewWriter(buf).WriteAndFlush(connack)
		a.Nil(err)
		a.Equal(v.want, buf.Bytes())
	}

}

func TestReadConnackPacket_V311(t *testing.T) {
	a := assert.New(t)
	connackPacketBytes := bytes.NewBuffer([]byte{32, 2, //FixHeader
		0, //SessionPresent
		1, //Code
	})
	packet, err := NewReader(connackPacketBytes).ReadPacket()
	a.Nil(err)
	if cp, ok := packet.(*Connack); ok {
		a.False(cp.SessionPresent)
		a.EqualValues(1, cp.Code)
	} else {
		t.Fatalf("Packet type error,want %v,got %v", reflect.TypeOf(&Connack{}), reflect.TypeOf(packet))
	}
}
