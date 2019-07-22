package packets

import (
	"bytes"
	"io"
	"reflect"
	"testing"
)

func TestWritePublishPacket(t *testing.T) {

	var tt = []struct {
		topicName []byte
		dup       bool
		retain    bool
		qos       uint8
		pid       uint16
		payload   []byte
	}{
		{topicName: []byte("test topic name1"), dup: true, retain: false, qos: QOS_1, pid: 10, payload: []byte("test payload1")},
		{topicName: []byte("test topic name2"), dup: false, retain: true, qos: QOS_0, payload: []byte("test payload2")},
		{topicName: []byte("test topic name3"), dup: false, retain: true, qos: QOS_2, pid: 11, payload: []byte("test payload3")},
		{topicName: []byte("test topic name4"), dup: false, retain: false, qos: QOS_1, pid: 12, payload: []byte("")},
	}

	for _, v := range tt {
		b := make([]byte, 0, 2048)
		buf := bytes.NewBuffer(b)
		pub := &Publish{
			Dup:       v.dup,
			Qos:       v.qos,
			Retain:    v.retain,
			TopicName: v.topicName,
			PacketID:  v.pid,
			Payload:   v.payload,
		}
		err := NewWriter(buf).WriteAndFlush(pub)
		if err != nil {
			t.Fatalf("unexpected error: %s,%v", err.Error(), string(v.topicName))
		}
		packet, err := NewReader(buf).ReadPacket()
		if err != nil {
			t.Fatalf("unexpected error: %s,%v", err.Error(), string(v.topicName))
		}
		n, err := buf.ReadByte()
		if err != io.EOF {
			t.Fatalf("ReadByte() error,want io.EOF,got %s and %v bytes", err, n)
		}

		if p, ok := packet.(*Publish); ok {
			if !bytes.Equal(p.TopicName, pub.TopicName) {
				t.Fatalf("TopicName error,want %v, got %v", pub.TopicName, p.TopicName)
			}
			if p.PacketID != pub.PacketID {
				t.Fatalf("PacketID error,want %v, got %v", pub.PacketID, p.PacketID)
			}
			if !bytes.Equal(p.Payload, pub.Payload) {
				t.Fatalf("Payload error,want %v, got %v", pub.Payload, p.Payload)
			}
			if p.Retain != pub.Retain {
				t.Fatalf("Retain error,want %v, got %v", pub.Retain, p.Retain)
			}
			if p.Qos != pub.Qos {
				t.Fatalf("Qos error,want %v, got %v", pub.Qos, p.Qos)
			}
			if p.Dup != pub.Dup {
				t.Fatalf("Dup error,want %v, got %v", pub.Dup, p.Dup)
			}

		} else {
			t.Fatalf("Packet type error,want %v,got %v", reflect.TypeOf(&Publish{}), reflect.TypeOf(packet))
		}
	}

}

func TestReadPublishPacket(t *testing.T) {
	publishPacketBytes := bytes.NewBuffer([]byte{0x3d, 31, //FIxHeader，dup=1,qos=2,retain =1
		0, 15, 116, 101, 115, 116, 32, 84, 111, 112, 105, 99, 32, 78, 97, 109, 101, //"test Topic Name"
		0, 10, //pid 10
		116, 101, 115, 116, 32, 112, 97, 121, 108, 111, 97, 100, //"test payload"
	})
	topicName := []byte{116, 101, 115, 116, 32, 84, 111, 112, 105, 99, 32, 78, 97, 109, 101}
	pid := uint16(10)
	payload := []byte{116, 101, 115, 116, 32, 112, 97, 121, 108, 111, 97, 100}

	packet, err := NewReader(publishPacketBytes).ReadPacket()
	if err != nil {
		t.Fatalf("Unexpected error: %s", err.Error())
	}
	pp := packet.(*Publish)
	if pp.Qos != QOS_2 {
		t.Fatalf("Qos error,want %d, got %d", QOS_2, pp.Qos)
	}
	if !pp.Retain {
		t.Fatalf("Retain error,want %t, got %t", true, pp.Retain)
	}
	if !bytes.Equal(pp.TopicName, topicName) {
		t.Fatalf("TopicName error,want %v, got %v", topicName, pp.TopicName)
	}

	if pp.PacketID != pid {
		t.Fatalf("PacketID error,want %d, got %d", pid, pp.PacketID)
	}

	if !bytes.Equal(pp.Payload, payload) {
		t.Fatalf("Payload error,want %v, got %v", payload, pp.Payload)
	}
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
