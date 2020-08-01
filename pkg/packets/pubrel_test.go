package packets

import (
	"bytes"
	"testing"

	"github.com/DrmagicE/gmqtt/pkg/codes"
	"github.com/stretchr/testify/assert"
)

func TestReadWritePubrelPacket(t *testing.T) {
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
			want:       []byte{98, 2, 0, 10},
		},
		{
			testname: "code = 0 with properties",
			pid:      10,
			code:     codes.Success,
			properties: &Properties{
				ReasonString: []byte("a"),
			},
			want: []byte{98, 8, 0, 10, 0, 4, 0x1f, 0, 1, 'a'},
		}, {
			testname:   "code != 0 with properties",
			pid:        10,
			code:       codes.NotAuthorized,
			properties: &Properties{},
			want:       []byte{98, 4, 0, 10, codes.NotAuthorized, 0},
		},
	}

	for _, v := range tt {
		t.Run(v.testname, func(t *testing.T) {
			a := assert.New(t)
			b := make([]byte, 0, 2048)
			buf := bytes.NewBuffer(b)
			puback := &Pubrel{
				PacketID:   v.pid,
				Properties: v.properties,
				Code:       v.code,
			}
			err := NewWriter(buf).WriteAndFlush(puback)
			a.Nil(err)
			a.Equal(v.want, buf.Bytes())

			bufr := bytes.NewBuffer(buf.Bytes())

			p, err := NewReader(bufr).ReadPacket()
			a.Nil(err)
			rp := p.(*Pubrel)

			a.Equal(v.code, rp.Code)
			a.Equal(v.properties, rp.Properties)
			a.Equal(v.pid, rp.PacketID)

		})
	}

}

func TestPubrel_NewPubcomp(t *testing.T) {
	a := assert.New(t)
	pid := uint16(10)
	pubrel := &Pubrel{
		PacketID: pid,
	}
	pubcomp := pubrel.NewPubcomp()
	a.Equal(pid, pubcomp.PacketID)
}
