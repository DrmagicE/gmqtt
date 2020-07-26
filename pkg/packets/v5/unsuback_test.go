package v5

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadWriteUnsubackPacket(t *testing.T) {
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
			want: []byte{0xb0, 4,
				0, 10, // pid
				0, // properties
				0, //code
			},
		},
		{
			pid:   10,
			codes: []ReasonCode{CodeSuccess, CodeNoSubscriptionExisted},
			properties: &Properties{
				ReasonString: []byte("a"),
			},
			want: []byte{0xb0, 9,
				0, 10, // pid
				4, // properties
				0x1f, 0, 1, 'a',
				0, 0x11, //codes
			},
		},
	}
	for _, v := range tt {

		b := make([]byte, 0, 2048)
		buf := bytes.NewBuffer(b)
		pkg := &Unsuback{
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
		rp := p.(*Unsuback)

		a.EqualValues(v.codes, rp.Payload)
		a.Equal(v.properties, rp.Properties)
		a.Equal(v.pid, rp.PacketID)

	}

}
