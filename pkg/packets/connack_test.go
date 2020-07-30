package v5

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWriteConnack(t *testing.T) {
	a := assert.New(t)
	var tt = []struct {
		connack *Connack
		want    []byte
	}{
		{connack: &Connack{ReasonCode: CodeSuccess, SessionPresent: true}, want: []byte{0x20, 3, 1, 0, 0}},
		{connack: &Connack{ReasonCode: CodeSuccess, SessionPresent: false}, want: []byte{0x20, 3, 0, 0, 0}},
		{connack: &Connack{ReasonCode: CodeNotAuthorized, SessionPresent: false}, want: []byte{0x20, 3, 0, CodeNotAuthorized, 0}},
		// with Properties
		{connack: &Connack{
			ReasonCode:     CodeSuccess,
			SessionPresent: false,
			Properties: &Properties{
				SessionExpiryInterval: uint32P(1),
				ReceiveMaximum:        uint16P(2),
				MaximumQOS:            byteP(1),
			},
		}, want: []byte{0x20, 13, 0, CodeSuccess,
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

func TestReadConnackPacket(t *testing.T) {
	a := assert.New(t)
	connackPacketBytes := bytes.NewBuffer([]byte{32, 8, //FixHeader
		0, //SessionPresent
		0, //Code
		5, // property length
		0x11, 0, 0, 0, 1,
	})
	packet, err := NewReader(connackPacketBytes).ReadPacket()
	if err != nil {
		t.Fatalf("Unexpected error: %s", err.Error())
	}
	if cp, ok := packet.(*Connack); ok {
		a.Equal(false, cp.SessionPresent)
		a.EqualValues(0, cp.ReasonCode)
	} else {
		t.Fatalf("Packet type error,want %v,got %v", reflect.TypeOf(&Connack{}), reflect.TypeOf(packet))
	}
}
