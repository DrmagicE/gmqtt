package packets

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadWriteSubscribe_V5(t *testing.T) {
	a := assert.New(t)
	firstByte := byte(0x82)
	pid := []byte{0, 10}
	properties := []byte{
		2,
		0x0b, 1,
	}
	topicFilter1 := []byte("/topic/A")
	topicFilter1Bytes, _, _ := EncodeUTF8String(topicFilter1)
	option1 := []byte{0x1d} // retain Handling = 1, rap = 1, nl = 1, qos = 1
	topicFilter2 := []byte("/topic/B")
	topicFilter2Bytes, _, _ := EncodeUTF8String(topicFilter2)
	option2 := []byte{0x06} // retain Handling = 0, rap = 0, nl = 1, qos = 2

	pb := appendPacket(firstByte, pid, properties, topicFilter1Bytes, option1, topicFilter2Bytes, option2)

	subBytes := bytes.NewBuffer(pb)

	var packet Packet
	var err error
	t.Run("unpack", func(t *testing.T) {
		r := NewReader(subBytes)
		r.SetVersion(Version5)
		packet, err = r.ReadPacket()
		a.Nil(err)
		if p, ok := packet.(*Subscribe); ok {
			a.Equal(binary.BigEndian.Uint16(pid), p.PacketID)
			a.EqualValues(1, p.Properties.SubscriptionIdentifier[0])
			a.EqualValues(topicFilter1, p.Topics[0].Name)
			a.EqualValues(1, p.Topics[0].RetainHandling)
			a.EqualValues(true, p.Topics[0].RetainAsPublished)
			a.EqualValues(true, p.Topics[0].NoLocal)
			a.EqualValues(1, p.Topics[0].Qos)

			a.EqualValues(0, p.Topics[1].RetainHandling)
			a.EqualValues(false, p.Topics[1].RetainAsPublished)
			a.EqualValues(true, p.Topics[1].NoLocal)
			a.EqualValues(2, p.Topics[1].Qos)

			a.Len(p.Topics, 2)

		} else {
			t.Fatalf("Packet Type error,want %v,got %v", reflect.TypeOf(&Subscribe{}), reflect.TypeOf(packet))
		}
	})

	t.Run("pack", func(t *testing.T) {
		bufw := &bytes.Buffer{}
		err = packet.Pack(bufw)
		a.Nil(err)
		a.Equal(pb, bufw.Bytes())
	})

}

func TestSubscribeNoTopics_V5(t *testing.T) {
	a := assert.New(t)
	firstByte := byte(0x82)
	pid := []byte{0, 10}
	properties := []byte{
		2,
		0x0b, 1,
	}

	pb := appendPacket(firstByte, pid, properties)

	subBytes := bytes.NewBuffer(pb)

	r := NewReader(subBytes)
	r.SetVersion(Version5)
	packet, err := r.ReadPacket()
	a.Nil(packet)
	a.NotNil(err)
}

func TestReadWriteSubscribe_V311(t *testing.T) {
	a := assert.New(t)
	firstByte := byte(0x82)
	pid := []byte{0, 10}
	topicFilter1 := []byte("/topic/A")
	topicFilter1Bytes, _, _ := EncodeUTF8String(topicFilter1)
	qos1 := []byte{0x01}
	topicFilter2 := []byte("/topic/B")
	topicFilter2Bytes, _, _ := EncodeUTF8String(topicFilter2)
	qos2 := []byte{0x02}

	pb := appendPacket(firstByte, pid, topicFilter1Bytes, qos1, topicFilter2Bytes, qos2)

	subBytes := bytes.NewBuffer(pb)

	var packet Packet
	var err error
	t.Run("unpack", func(t *testing.T) {
		r := NewReader(subBytes)
		r.SetVersion(Version311)
		packet, err = r.ReadPacket()
		a.Nil(err)
		if p, ok := packet.(*Subscribe); ok {
			a.Equal(binary.BigEndian.Uint16(pid), p.PacketID)
			a.EqualValues(topicFilter1, p.Topics[0].Name)
			a.EqualValues(1, p.Topics[0].Qos)
			a.EqualValues(2, p.Topics[1].Qos)
			a.Len(p.Topics, 2)

		} else {
			t.Fatalf("Packet Type error,want %v,got %v", reflect.TypeOf(&Subscribe{}), reflect.TypeOf(packet))
		}
	})

	t.Run("pack", func(t *testing.T) {
		bufw := &bytes.Buffer{}
		err = packet.Pack(bufw)
		a.Nil(err)
		a.Equal(pb, bufw.Bytes())
	})

}
