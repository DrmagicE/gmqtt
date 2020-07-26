package v5

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadWriteSubackPacket(t *testing.T) {
	a := assert.New(t)
	tt := []struct {
		pid        PacketID
		codes      []ReasonCode
		properties *Properties
		want       []byte
	}{
		{
			pid:        10,
			codes:      []ReasonCode{CodeSuccess},
			properties: &Properties{},
			want: []byte{0x90, 4,
				0, 10, // pid
				0, // properties
				0, //code
			},
		},
		{
			pid:   10,
			codes: []ReasonCode{CodeSuccess, CodeGrantedQoS1},
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
			codes:      []ReasonCode{CodeGrantedQoS1, CodeGrantedQoS2},
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
			PacketID:   v.pid,
			Properties: v.properties,
			Payload:    v.codes,
		}
		err := NewWriter(buf).WriteAndFlush(pkg)
		a.Nil(err)
		a.Equal(v.want, buf.Bytes())

		bufr := bytes.NewBuffer(buf.Bytes())

		p, err := NewReader(bufr).ReadPacket()
		a.Nil(err)
		rp := p.(*Suback)

		a.Equal(v.codes, rp.Payload)
		a.Equal(v.properties, rp.Properties)
		a.Equal(v.pid, rp.PacketID)

	}

}
