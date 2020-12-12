package packets

import (
	"bytes"
	"testing"

	"github.com/DrmagicE/gmqtt/pkg/codes"
	"github.com/stretchr/testify/assert"
)

func TestReadWriteUnsubackPacket_V5(t *testing.T) {
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
			want: []byte{0xb0, 4,
				0, 10, // pid
				0, // properties
				0, //code
			},
		},
		{
			pid:   10,
			codes: []codes.Code{codes.Success, codes.NoSubscriptionExisted},
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
		rp := p.(*Unsuback)

		a.EqualValues(v.codes, rp.Payload)
		a.Equal(v.properties, rp.Properties)
		a.Equal(v.pid, rp.PacketID)

	}

}

func TestReadWriteUnsubackPacket_V311(t *testing.T) {
	a := assert.New(t)
	tt := []struct {
		pid  PacketID
		want []byte
	}{
		{
			pid: 10,
			want: []byte{0xb0, 2,
				0, 10, // pid
			},
		},
	}
	for _, v := range tt {

		b := make([]byte, 0, 2048)
		buf := bytes.NewBuffer(b)
		pkg := &Unsuback{
			Version:  Version311,
			PacketID: v.pid,
		}
		err := NewWriter(buf).WriteAndFlush(pkg)
		a.Nil(err)
		a.Equal(v.want, buf.Bytes())

		bufr := bytes.NewBuffer(buf.Bytes())

		r := NewReader(bufr)
		r.SetVersion(Version311)
		p, err := r.ReadPacket()
		a.Nil(err)
		rp := p.(*Unsuback)
		a.Equal(v.pid, rp.PacketID)

	}
}
