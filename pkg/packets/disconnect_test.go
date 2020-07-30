package v5

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadWriteDisconnectPacket(t *testing.T) {
	tt := []struct {
		testname   string
		code       ReasonCode
		properties *Properties
		want       []byte
	}{
		{
			testname:   "omit properties when code = 0",
			code:       CodeSuccess,
			properties: nil,
			want:       []byte{0xe0, 0},
		},
		{
			testname: "code = 0 with properties",
			code:     CodeSuccess,
			properties: &Properties{
				ReasonString: []byte("a"),
			},
			want: []byte{0xe0, 6, 0, 4, 0x1f, 0, 1, 'a'},
		}, {
			testname:   "code != 0 with properties",
			code:       CodeNotAuthorized,
			properties: &Properties{},
			want:       []byte{0xe0, 2, CodeNotAuthorized, 0},
		},
	}

	for _, v := range tt {
		t.Run(v.testname, func(t *testing.T) {
			a := assert.New(t)
			b := make([]byte, 0, 2048)
			buf := bytes.NewBuffer(b)
			dis := &Disconnect{
				Properties: v.properties,
				Code:       v.code,
			}
			err := NewWriter(buf).WriteAndFlush(dis)
			a.Nil(err)
			a.Equal(v.want, buf.Bytes())

			bufr := bytes.NewBuffer(buf.Bytes())
			p, err := NewReader(bufr).ReadPacket()
			a.Nil(err)
			rp := p.(*Disconnect)

			a.Equal(v.code, rp.Code)
			a.Equal(v.properties, rp.Properties)

		})
	}

}
