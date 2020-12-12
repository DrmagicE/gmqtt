package packets

import (
	"bytes"
	"testing"

	"github.com/DrmagicE/gmqtt/pkg/codes"
	"github.com/stretchr/testify/assert"
)

func TestReadWriteDisconnectPacket_V5(t *testing.T) {
	tt := []struct {
		testname   string
		code       codes.Code
		properties *Properties
		want       []byte
	}{
		{
			testname:   "omit properties when code = 0",
			code:       codes.Success,
			properties: &Properties{},
			want:       []byte{0xe0, 0x02, 0x00, 0x00},
		},
		{
			testname: "code = 0 with properties",
			code:     codes.Success,
			properties: &Properties{
				ReasonString: []byte("a"),
			},
			want: []byte{0xe0, 6, 0, 4, 0x1f, 0, 1, 'a'},
		}, {
			testname:   "code != 0 with properties",
			code:       codes.NotAuthorized,
			properties: &Properties{},
			want:       []byte{0xe0, 2, codes.NotAuthorized, 0},
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
			r := NewReader(bufr)
			r.SetVersion(Version5)
			p, err := r.ReadPacket()
			a.Nil(err)
			rp := p.(*Disconnect)

			a.Equal(v.code, rp.Code)
			a.Equal(v.properties, rp.Properties)

		})
	}

}

func TestReadDisconnect_V311(t *testing.T) {
	a := assert.New(t)
	b := []byte{0xe0, 0}
	buf := bytes.NewBuffer(b)
	packet, err := NewReader(buf).ReadPacket()
	a.Nil(err)

	_, ok := packet.(*Disconnect)
	a.True(ok)
}

func TestWriteDisconnect_V311(t *testing.T) {
	a := assert.New(t)
	disconnect := &Disconnect{Version: Version311}
	buf := bytes.NewBuffer(make([]byte, 0, 2048))
	err := NewWriter(buf).WriteAndFlush(disconnect)
	a.Nil(err)
	want := []byte{0xe0, 0}
	a.Equal(want, buf.Bytes())
}
