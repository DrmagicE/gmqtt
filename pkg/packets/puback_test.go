package packets

import (
	"bytes"
	"io"
	"reflect"
	"testing"

	"github.com/DrmagicE/gmqtt/pkg/codes"
	"github.com/stretchr/testify/assert"
)

func TestReadWritePubackPacket_V5(t *testing.T) {
	tt := []struct {
		testname   string
		pid        PacketID
		code       codes.Code
		properties *Properties
		want       []byte
	}{
		{
			testname:   "omit properties when code = 0",
			pid:        10,
			code:       codes.Success,
			properties: nil,
			want:       []byte{64, 2, 0, 10},
		},
		{
			testname: "code = 0 with properties",
			pid:      10,
			code:     codes.Success,
			properties: &Properties{
				ReasonString: []byte("a"),
			},
			want: []byte{64, 8, 0, 10, 0, 4, 0x1f, 0, 1, 'a'},
		}, {
			testname:   "code != 0 with properties",
			pid:        10,
			code:       codes.NotAuthorized,
			properties: &Properties{},
			want:       []byte{64, 4, 0, 10, codes.NotAuthorized, 0},
		},
	}

	for _, v := range tt {
		t.Run(v.testname, func(t *testing.T) {
			a := assert.New(t)
			b := make([]byte, 0, 2048)
			buf := bytes.NewBuffer(b)
			puback := &Puback{
				Version:    Version5,
				PacketID:   v.pid,
				Properties: v.properties,
				Code:       v.code,
			}
			err := NewWriter(buf).WriteAndFlush(puback)
			a.Nil(err)
			a.Equal(v.want, buf.Bytes())

			bufr := bytes.NewBuffer(buf.Bytes())
			r := NewReader(bufr)
			r.SetVersion(Version5)
			p, err := r.ReadPacket()
			a.Nil(err)
			rp := p.(*Puback)

			a.Equal(v.code, rp.Code)
			a.Equal(v.properties, rp.Properties)
			a.Equal(v.pid, rp.PacketID)

		})
	}

}

func TestWritePubackPacket_V311(t *testing.T) {
	a := assert.New(t)
	b := make([]byte, 0, 2048)
	buf := bytes.NewBuffer(b)
	pid := uint16(65535)
	puback := &Puback{
		Version:  Version311,
		PacketID: pid,
	}
	err := NewWriter(buf).WriteAndFlush(puback)
	a.Nil(err)
	packet, err := NewReader(buf).ReadPacket()
	a.Nil(err)
	_, err = buf.ReadByte()
	a.Equal(io.EOF, err)

	if p, ok := packet.(*Puback); ok {
		a.EqualValues(pid, p.PacketID)
	} else {
		t.Fatalf("Packet type error,want %v,got %v", reflect.TypeOf(&Puback{}), reflect.TypeOf(packet))
	}

}

func TestReadPubackPacket(t *testing.T) {
	a := assert.New(t)
	pubackBytes := bytes.NewBuffer([]byte{64, 2, 0, 1})
	packet, err := NewReader(pubackBytes).ReadPacket()
	a.Nil(err)

	if p, ok := packet.(*Puback); ok {
		a.EqualValues(1, p.PacketID)
	} else {
		t.Fatalf("Packet Type error,want %v,got %v", reflect.TypeOf(&Puback{}), reflect.TypeOf(packet))
	}
}
