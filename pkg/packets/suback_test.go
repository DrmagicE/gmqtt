package packets

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/DrmagicE/gmqtt/pkg/codes"
	"github.com/stretchr/testify/assert"
)

func TestReadWriteSubackPacket_V5(t *testing.T) {
	a := assert.New(t)
	tt := []struct {
		pid        PacketID
		codes      []codes.Code
		properties *Properties
		want       []byte
	}{
		{
			pid:        10,
			codes:      []codes.Code{codes.Success},
			properties: &Properties{},
			want: []byte{0x90, 4,
				0, 10, // pid
				0, // properties
				0, //code
			},
		},
		{
			pid:   10,
			codes: []codes.Code{codes.Success, codes.GrantedQoS1},
			properties: &Properties{
				ReasonString: []byte("a"),
			},
			want: []byte{0x90, 9,
				0, 10, // pid
				4, // properties
				0x1f, 0, 1, 'a',
				0, 0x01, //codes
			},
		},
		{
			pid:        10,
			codes:      []codes.Code{codes.GrantedQoS1, codes.GrantedQoS2},
			properties: &Properties{},
			want: []byte{0x90, 5,
				0, 10, // pid
				0,          // properties
				0x01, 0x02, //codes
			},
		},
	}

	for _, v := range tt {

		b := make([]byte, 0, 2048)
		buf := bytes.NewBuffer(b)
		pkg := &Suback{
			Version:    Version5,
			PacketID:   v.pid,
			Properties: v.properties,
			Payload:    v.codes,
		}
		err := NewWriter(buf).WriteAndFlush(pkg)
		a.Nil(err)
		a.Equal(v.want, buf.Bytes())

		bufr := bytes.NewBuffer(buf.Bytes())

		r := NewReader(bufr)
		r.SetVersion(Version5)
		p, err := r.ReadPacket()
		a.Nil(err)
		rp := p.(*Suback)
		a.Equal(v.codes, rp.Payload)
		a.Equal(v.properties, rp.Properties)
		a.Equal(v.pid, rp.PacketID)

	}

}

func TestReadSuback_V311(t *testing.T) {
	a := assert.New(t)
	subackBytes := bytes.NewBuffer([]byte{0x90, 5, //FixHeader
		0, 10, //packetID
		0, 1, 2, //payload
	})
	packet, err := NewReader(subackBytes).ReadPacket()
	a.Nil(err)
	if p, ok := packet.(*Suback); ok {
		a.EqualValues(10, p.PacketID)
		a.Equal([]byte{0, 1, 2}, p.Payload)
	} else {
		t.Fatalf("Packet Type error,want %v,got %v", reflect.TypeOf(&Suback{}), reflect.TypeOf(packet))
	}
}

func TestWriteSuback_V311(t *testing.T) {
	a := assert.New(t)
	subscribeBytes := bytes.NewBuffer([]byte{0x82, 29, //FIxHeader
		0, 10, //pid 10
		0, 5, 97, 47, 98, 47, 99, //Topic Filter :"a/b/c"
		0,                            //qos = 0
		0, 6, 97, 47, 98, 47, 99, 99, //Topic Filter："a/b/cc"
		1,                                //qos = 1
		0, 7, 97, 47, 98, 47, 99, 99, 99, //Topic Filter："a/b/ccc"
		2, //qos = 2
	})
	packet, err := NewReader(subscribeBytes).ReadPacket()
	a.Nil(err)
	p := packet.(*Subscribe)
	suback := p.NewSuback()
	buf := bytes.NewBuffer(make([]byte, 0, 2048))
	err = NewWriter(buf).WriteAndFlush(suback)
	a.Nil(err)
	want := []byte{0x90, 5, 0, 10, 0, 1, 2}
	a.Equal(want, buf.Bytes())

}
