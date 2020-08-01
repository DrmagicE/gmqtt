package packets

import (
	"bytes"
	"io"
	"reflect"
	"testing"

	"github.com/DrmagicE/gmqtt/pkg/codes"
	"github.com/stretchr/testify/assert"
)

func TestReadWritePubrecPacket_V5(t *testing.T) {
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
			want:       []byte{80, 2, 0, 10},
		},
		{
			testname: "code = 0 with properties",
			pid:      10,
			code:     codes.Success,
			properties: &Properties{
				ReasonString: []byte("a"),
			},
			want: []byte{80, 8, 0, 10, 0, 4, 0x1f, 0, 1, 'a'},
		}, {
			testname:   "code != 0 with properties",
			pid:        10,
			code:       codes.NotAuthorized,
			properties: &Properties{},
			want:       []byte{80, 4, 0, 10, codes.NotAuthorized, 0},
		},
	}

	for _, v := range tt {
		t.Run(v.testname, func(t *testing.T) {
			a := assert.New(t)
			b := make([]byte, 0, 2048)
			buf := bytes.NewBuffer(b)
			puback := &Pubrec{
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
			rp := p.(*Pubrec)

			a.Equal(v.code, rp.Code)
			a.Equal(v.properties, rp.Properties)
			a.Equal(v.pid, rp.PacketID)

		})
	}

}

func TestWritePubrecPacket_V311(t *testing.T) {
	a := assert.New(t)
	b := make([]byte, 0, 2048)
	buf := bytes.NewBuffer(b)
	pid := uint16(65535)
	pubrec := &Pubrec{
		Version:  Version311,
		PacketID: pid,
	}
	err := NewWriter(buf).WriteAndFlush(pubrec)
	a.Nil(err)

	packet, err := NewReader(buf).ReadPacket()
	a.Nil(err)
	_, err = buf.ReadByte()
	a.Equal(io.EOF, err)

	if p, ok := packet.(*Pubrec); ok {
		a.EqualValues(pid, p.PacketID)
	} else {
		t.Fatalf("Packet type error,want %v,got %v", reflect.TypeOf(&Pubrec{}), reflect.TypeOf(packet))
	}

}

func TestReadPubrecPacket(t *testing.T) {
	a := assert.New(t)
	pubrecBytes := bytes.NewBuffer([]byte{0x50, 2, 0, 1})
	packet, err := NewReader(pubrecBytes).ReadPacket()
	a.Nil(err)
	if p, ok := packet.(*Pubrec); ok {
		a.EqualValues(1, p.PacketID)
	} else {
		t.Fatalf("Packet Type error,want %v,got %v", reflect.TypeOf(&Pubrec{}), reflect.TypeOf(packet))
	}
}

func TestPubrec_NewPubrel(t *testing.T) {
	pid := uint16(10)
	pubrec := &Pubrec{
		PacketID: pid,
	}
	pubrel := pubrec.NewPubrel()
	if pubrel.PacketID != pid {
		t.Fatalf("PacketID  error,want %d,got %d", 1, pubrel.PacketID)
	}

}
