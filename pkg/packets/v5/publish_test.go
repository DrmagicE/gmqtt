package v5

import (
	"bytes"
	"encoding/binary"
	"io"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWritePublishPacket(t *testing.T) {
	a := assert.New(t)
	var tt = []struct {
		topicName  []byte
		dup        bool
		retain     bool
		qos        uint8
		pid        uint16
		payload    []byte
		properties *Properties
	}{
		{
			topicName: []byte("test topic name1"),
			dup:       true,
			retain:    false,
			qos:       QOS_1,
			pid:       10,
			payload:   []byte("test payload1"),
			properties: &Properties{
				PayloadFormat: byteP(1),
			},
		},
		{
			topicName: []byte("test topic name2"),
			dup:       false,
			retain:    true,
			qos:       QOS_0,
			payload:   []byte("test payload2"),
			properties: &Properties{
				MessageExpiry: uint32P(100),
			},
		},
		{
			topicName:  []byte("test topic name3"),
			dup:        false,
			retain:     true,
			qos:        QOS_2,
			pid:        11,
			payload:    []byte("test payload3"),
			properties: &Properties{},
		},

		{
			topicName:  []byte("test topic name4"),
			dup:        false,
			retain:     false,
			qos:        QOS_1,
			pid:        12,
			payload:    []byte(""),
			properties: &Properties{},
		},
	}

	for _, v := range tt {
		b := make([]byte, 0, 2048)
		buf := bytes.NewBuffer(b)
		pub := &Publish{
			Dup:        v.dup,
			Qos:        v.qos,
			Retain:     v.retain,
			TopicName:  v.topicName,
			PacketID:   v.pid,
			Payload:    v.payload,
			Properties: v.properties,
		}
		err := NewWriter(buf).WriteAndFlush(pub)
		a.Nil(err)
		packet, err := NewReader(buf).ReadPacket()
		a.Nil(err)
		_, err = buf.ReadByte()
		a.Equal(io.EOF, err)
		if p, ok := packet.(*Publish); ok {
			a.Equal(pub.TopicName, p.TopicName)
			a.Equal(pub.PacketID, p.PacketID)
			a.Equal(pub.Payload, p.Payload)
			a.Equal(pub.Retain, p.Retain)
			a.Equal(pub.Qos, p.Qos)
			a.Equal(pub.Dup, p.Dup)
			a.Equal(pub.Properties, p.Properties)
		} else {
			t.Fatalf("Packet type error,want %v,got %v", reflect.TypeOf(&Publish{}), reflect.TypeOf(packet))
		}
	}

}

func TestReadPublishPacket(t *testing.T) {
	a := assert.New(t)
	firstByte := byte(0x3d) // dup=1,qos=2,retain =1
	topicName := []byte("test Topic Name")
	topicNameBytes, _, _ := EncodeUTF8String(topicName)
	pid := []byte{0, 10}
	properties := []byte{
		7,
		0x01, 0,
		0x02, 0, 0, 0, 1,
	}
	payload := []byte("test payload")

	pb := appendPacket(firstByte, topicNameBytes, pid, properties, payload)

	publishPacketBytes := bytes.NewBuffer(pb)

	var packet Packet
	var err error
	t.Run("unpack", func(t *testing.T) {
		packet, err = NewReader(publishPacketBytes).ReadPacket()
		a.Nil(err)
		pp := packet.(*Publish)
		a.Equal(QOS_2, pp.Qos)
		a.Equal(true, pp.Retain)
		a.Equal(topicName, pp.TopicName)
		a.Equal(payload, pp.Payload)
		a.Equal(binary.BigEndian.Uint16(pid), pp.PacketID)
	})

	t.Run("pack", func(t *testing.T) {
		bufw := &bytes.Buffer{}
		err = packet.Pack(bufw)
		a.Nil(err)
		a.Equal(pb, bufw.Bytes())
	})
}

func TestPublish_NewPuback(t *testing.T) {
	pid := uint16(123)
	pub := &Publish{
		Qos:      QOS_1,
		PacketID: pid,
	}
	puback := pub.NewPuback()
	if puback.PacketID != pid {
		t.Fatalf("packet id error ,want %d, got %d", pid, puback.PacketID)
	}
}

func TestPublish_NewPubrec(t *testing.T) {
	pid := uint16(123)
	pub := &Publish{
		Qos:      QOS_2,
		PacketID: pid,
	}
	puback := pub.NewPubrec()
	if puback.PacketID != pid {
		t.Fatalf("packet id error ,want %d, got %d", pid, puback.PacketID)
	}
}
