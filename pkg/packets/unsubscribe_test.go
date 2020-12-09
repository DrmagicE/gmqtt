package packets

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadWriteUnsubscribe_V5(t *testing.T) {
	a := assert.New(t)
	firstByte := byte(0xa2)
	pid := []byte{0, 10}
	properties := []byte{0}
	topicFilter1 := []byte("/topic/A")
	topicFilter1Bytes, _, _ := EncodeUTF8String(topicFilter1)

	topicFilter2 := []byte("/topic/B")
	topicFilter2Bytes, _, _ := EncodeUTF8String(topicFilter2)

	pb := appendPacket(firstByte, pid, properties, topicFilter1Bytes, topicFilter2Bytes)

	unsubBytes := bytes.NewBuffer(pb)

	var packet Packet
	var err error
	t.Run("unpack", func(t *testing.T) {
		r := NewReader(unsubBytes)
		r.SetVersion(Version5)
		packet, err = r.ReadPacket()
		a.Nil(err)
		if p, ok := packet.(*Unsubscribe); ok {
			a.Equal(binary.BigEndian.Uint16(pid), p.PacketID)
			a.EqualValues(topicFilter1, p.Topics[0])
			a.EqualValues(topicFilter2, p.Topics[1])
			a.Len(p.Topics, 2)
		} else {
			t.Fatalf("Packet Type error,want %v,got %v", reflect.TypeOf(&Unsubscribe{}), reflect.TypeOf(packet))
		}
	})

	t.Run("pack", func(t *testing.T) {
		bufw := &bytes.Buffer{}
		err = packet.Pack(bufw)
		a.Nil(err)
		a.Equal(pb, bufw.Bytes())
	})

}

func TestUnsubscribeNoTopics_V5(t *testing.T) {
	a := assert.New(t)
	firstByte := byte(0xa2)
	pid := []byte{0, 10}
	properties := []byte{0}
	pb := appendPacket(firstByte, pid, properties)
	unsubBytes := bytes.NewBuffer(pb)
	r := NewReader(unsubBytes)
	r.SetVersion(Version5)
	packet, err := r.ReadPacket()
	a.NotNil(err)
	a.Nil(packet)
}

func TestReadWriteUnsubscribe_V311(t *testing.T) {
	a := assert.New(t)
	firstByte := byte(0xa2)
	pid := []byte{0, 10}
	topicFilter1 := []byte("/topic/A")
	topicFilter1Bytes, _, _ := EncodeUTF8String(topicFilter1)

	topicFilter2 := []byte("/topic/B")
	topicFilter2Bytes, _, _ := EncodeUTF8String(topicFilter2)

	pb := appendPacket(firstByte, pid, topicFilter1Bytes, topicFilter2Bytes)

	unsubBytes := bytes.NewBuffer(pb)

	var packet Packet
	var err error
	t.Run("unpack", func(t *testing.T) {
		r := NewReader(unsubBytes)
		r.SetVersion(Version311)
		packet, err = r.ReadPacket()
		a.Nil(err)
		if p, ok := packet.(*Unsubscribe); ok {
			a.Equal(binary.BigEndian.Uint16(pid), p.PacketID)
			a.EqualValues(topicFilter1, p.Topics[0])
			a.EqualValues(topicFilter2, p.Topics[1])
			a.Len(p.Topics, 2)
		} else {
			t.Fatalf("Packet Type error,want %v,got %v", reflect.TypeOf(&Unsubscribe{}), reflect.TypeOf(packet))
		}
	})

	t.Run("pack", func(t *testing.T) {
		bufw := &bytes.Buffer{}
		err = packet.Pack(bufw)
		a.Nil(err)
		a.Equal(pb, bufw.Bytes())
	})

}
