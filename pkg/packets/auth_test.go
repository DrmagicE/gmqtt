package packets

import (
	"bytes"
	"testing"

	"github.com/DrmagicE/gmqtt/pkg/codes"
	"github.com/stretchr/testify/assert"
)

func TestReadWriteAuthPacket(t *testing.T) {
	tt := []struct {
		testname   string
		code       codes.Code
		properties *Properties
		want       []byte
	}{
		{
			testname:   "omit properties when code = 0",
			code:       codes.Success,
			properties: nil,
			want:       []byte{0xF0, 0},
		},
		{
			testname: "code = 0 with properties",
			code:     codes.Success,
			properties: &Properties{
				ReasonString: []byte("a"),
			},
			want: []byte{0xF0, 6, 0, 4, 0x1F, 0, 1, 'a'},
		}, {
			testname:   "code != 0 with properties",
			code:       codes.NotAuthorized,
			properties: &Properties{},
			want:       []byte{0xF0, 2, codes.NotAuthorized, 0},
		},
	}

	for _, v := range tt {
		t.Run(v.testname, func(t *testing.T) {
			a := assert.New(t)
			b := make([]byte, 0, 2048)
			buf := bytes.NewBuffer(b)
			au := &Auth{
				Properties: v.properties,
				Code:       v.code,
			}
			err := NewWriter(buf).WriteAndFlush(au)
			a.Nil(err)
			a.Equal(v.want, buf.Bytes())

			bufr := bytes.NewBuffer(buf.Bytes())
			p, err := NewReader(bufr).ReadPacket()
			a.Nil(err)
			rp := p.(*Auth)

			a.Equal(v.code, rp.Code)
			a.Equal(v.properties, rp.Properties)

		})
	}

}
