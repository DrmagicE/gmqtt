package packets

import (
	"bytes"
	"encoding/binary"
	"io"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DrmagicE/gmqtt/pkg/codes"
)

func TestReadWritePublishPacket_V5(t *testing.T) {
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
			qos:       Qos1,
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
			qos:       Qos0,
			payload:   []byte("test payload2"),
			properties: &Properties{
				MessageExpiry: uint32P(100),
			},
		},
		{
			topicName:  []byte("test topic name3"),
			dup:        false,
			retain:     true,
			qos:        Qos2,
			pid:        11,
			payload:    []byte("test payload3"),
			properties: &Properties{},
		},

		{
			topicName:  []byte("test topic name4"),
			dup:        false,
			retain:     false,
			qos:        Qos1,
			pid:        12,
			payload:    []byte(""),
			properties: &Properties{},
		},
	}

	for _, v := range tt {
		b := make([]byte, 0, 2048)
		buf := bytes.NewBuffer(b)
		pub := &Publish{
			Version:    Version5,
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
		bufr := bytes.NewBuffer(buf.Bytes())
		r := NewReader(bufr)
		r.SetVersion(Version5)
		p, err := r.ReadPacket()
		a.Nil(err)
		_, err = bufr.ReadByte()
		a.Equal(io.EOF, err)
		if p, ok := p.(*Publish); ok {
			a.Equal(pub.TopicName, p.TopicName)
			a.Equal(pub.PacketID, p.PacketID)
			a.Equal(pub.Payload, p.Payload)
			a.Equal(pub.Retain, p.Retain)
			a.Equal(pub.Qos, p.Qos)
			a.Equal(pub.Dup, p.Dup)
			a.Equal(pub.Properties, p.Properties)
		} else {
			t.Fatalf("Packet type error,want %v,got %v", reflect.TypeOf(&Publish{}), reflect.TypeOf(p))
		}
	}

}

func TestReadPublishPacket_V5(t *testing.T) {
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
		r := NewReader(publishPacketBytes)
		r.SetVersion(Version5)
		packet, err = r.ReadPacket()
		a.Nil(err)
		pp := packet.(*Publish)
		a.Equal(Qos2, pp.Qos)
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

func TestReadWritePublishPacket_V311(t *testing.T) {
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
			topicName: []byte("abc"),
			dup:       true,
			retain:    false,
			qos:       Qos1, pid: 10,
			payload: []byte("a")},
		{
			topicName: []byte("test topic name2"),
			dup:       false,
			retain:    true,
			qos:       Qos0,
			payload:   []byte("test payload2")},
		{
			topicName: []byte("test topic name3"),
			dup:       false,
			retain:    true,
			qos:       Qos2,
			pid:       11,
			payload:   []byte("test payload3"),
		},
	}

	for _, v := range tt {
		b := make([]byte, 0, 2048)
		buf := bytes.NewBuffer(b)
		pub := &Publish{
			Version:    Version311,
			Dup:        v.dup,
			Qos:        v.qos,
			Retain:     v.retain,
			TopicName:  v.topicName,
			PacketID:   v.pid,
			Payload:    v.payload,
			Properties: v.properties,
		}
		a.Nil(NewWriter(buf).WriteAndFlush(pub))
		packet, err := NewReader(buf).ReadPacket()
		a.Nil(err, string(v.topicName))
		_, err = buf.ReadByte()
		a.Equal(io.EOF, err, string(v.topicName))
		if p, ok := packet.(*Publish); ok {
			a.Equal(p.TopicName, pub.TopicName)
			a.Equal(p.PacketID, pub.PacketID)
			a.Equal(p.Payload, pub.Payload)
			a.Equal(p.Retain, pub.Retain)
			a.Equal(p.Qos, pub.Qos)
			a.Equal(p.Dup, pub.Dup)
			a.Equal(p.Properties, pub.Properties)
		} else {
			t.Fatalf("Packet type error,want %v,got %v", reflect.TypeOf(&Publish{}), reflect.TypeOf(packet))
		}
	}

}

func TestPublish_NewPuback(t *testing.T) {
	pid := uint16(123)
	pub := &Publish{
		Qos:      Qos1,
		PacketID: pid,
	}
	puback := pub.NewPuback(codes.Success, nil)
	if puback.PacketID != pid {
		t.Fatalf("packet id error ,want %d, got %d", pid, puback.PacketID)
	}
}

func TestPublish_NewPubrec(t *testing.T) {
	pid := uint16(123)
	pub := &Publish{
		Qos:      Qos2,
		PacketID: pid,
	}
	puback := pub.NewPubrec(codes.Success, nil)
	if puback.PacketID != pid {
		t.Fatalf("packet id error ,want %d, got %d", pid, puback.PacketID)
	}
}
