package packets

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DrmagicE/gmqtt/pkg/codes"
)

func TestReadConnectPacketErr_V5(t *testing.T) {
	//[MQTT-3.1.2-3],服务端必须验证CONNECT控制报文的保留标志位（第0位）是否为0，如果不为0必须断开客户端连接
	a := assert.New(t)

	b := []byte{16, 12, 0, 4, 'M', 'Q', 'T', 'T', 05, 01, 00, 02, 31, 32}
	buf := bytes.NewBuffer(b)
	r := NewReader(buf)
	r.SetVersion(Version5)
	connectPacket, err := r.ReadPacket()
	a.Nil(connectPacket)
	a.Error(codes.ErrMalformed, err)

}
func TestReadConnectPacketErr_V311(t *testing.T) {
	//[MQTT-3.1.2-3],服务端必须验证CONNECT控制报文的保留标志位（第0位）是否为0，如果不为0必须断开客户端连接
	a := assert.New(t)
	b := []byte{16, 12, 0, 4, 'M', 'Q', 'T', 'T', 04, 01, 00, 02, 31, 32}
	buf := bytes.NewBuffer(b)
	connectPacket, err := NewReader(buf).ReadPacket()
	a.Nil(connectPacket)
	a.Error(codes.ErrMalformed, err)
}