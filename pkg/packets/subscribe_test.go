package packets

import (
	"bytes"
	"io"
	"reflect"
	"testing"
)

func subscribe3TopicsBuffer() *bytes.Buffer {
	return bytes.NewBuffer([]byte{0x82, 29, //FIxHeader
		0, 10, //pid 10
		0, 5, 97, 47, 98, 47, 99, //Topic Filter :"a/b/c"
		0,                            //qos = 0
		0, 6, 97, 47, 98, 47, 99, 99, //Topic Filter："a/b/cc"
		1,                                //qos = 1
		0, 7, 97, 47, 98, 47, 99, 99, 99, //Topic Filter："a/b/ccc"
		2, //qos = 2
	})
}

func subscribeOneTopicBuffer() *bytes.Buffer {
	return bytes.NewBuffer([]byte{0x82, 10, //FIxHeader
		0, 10, //pid 10
		0, 5, 97, 47, 98, 47, 99, //Topic Filter :"a/b/c"
		1, //qos = 1
	})
}

func TestReadSubscribePacketWithOneTopic(t *testing.T) {
	subscribeBytes := subscribeOneTopicBuffer()
	packet, err := NewReader(subscribeBytes).ReadPacket()
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}
	if p, ok := packet.(*Subscribe); ok {
		if p.PacketID != 10 {
			t.Fatalf("PacketID error,want %d, got %d", 10, p.PacketID)
		}
		if len(p.Topics) != 1 {
			t.Fatalf("len error,want %d, got %d", 1, len(p.Topics))
		}
		if p.Topics[0].Name != "a/b/c" {
			t.Fatalf("p.Topics[0].Name error,want %s, got %s", "a/b/c", p.Topics[0].Name)
		}

		if p.Topics[0].Qos != 1 {
			t.Fatalf("p.Topics[0].Qos error,want %d, got %d", 0, p.Topics[0].Qos)
		}

	} else {
		t.Fatalf("Packet Type error,want %v,got %v", reflect.TypeOf(&Subscribe{}), reflect.TypeOf(packet))
	}
}

func TestReadSubscribePacketWith3Topics(t *testing.T) {
	subscribeBytes := subscribe3TopicsBuffer()
	packet, err := NewReader(subscribeBytes).ReadPacket()
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}
	if p, ok := packet.(*Subscribe); ok {
		if p.PacketID != 10 {
			t.Fatalf("PacketID error,want %d, got %d", 10, p.PacketID)
		}
		if len(p.Topics) != 3 {
			t.Fatalf("len error,want %d, got %d", 3, len(p.Topics))
		}
		if p.Topics[0].Name != "a/b/c" {
			t.Fatalf("p.Topics[0].Name error,want %s, got %s", "a/b/c", p.Topics[0].Name)
		}
		if p.Topics[1].Name != "a/b/cc" {
			t.Fatalf("p.Topics[1].Name error,want %s, got %s", "a/b/cc", p.Topics[1].Name)
		}
		if p.Topics[2].Name != "a/b/ccc" {
			t.Fatalf("p.Topics[2].Name error,want %s, got %s", "a/b/ccc", p.Topics[2].Name)
		}

		if p.Topics[0].Qos != 0 {
			t.Fatalf("p.Topics[0].Qos error,want %d, got %d", 0, p.Topics[0].Qos)
		}
		if p.Topics[1].Qos != 1 {
			t.Fatalf("p.Topics[1].Qos error,want %d, got %d", 1, p.Topics[1].Qos)
		}
		if p.Topics[2].Qos != 2 {
			t.Fatalf("p.Topics[2].Qos error,want %d, got %d", 2, p.Topics[2].Qos)
		}

	} else {
		t.Fatalf("Packet Type error,want %v,got %v", reflect.TypeOf(&Subscribe{}), reflect.TypeOf(packet))
	}
}

func TestSubscribe_NewSubBackWithOneTopic(t *testing.T) {
	subscribeBytes := subscribeOneTopicBuffer()
	packet, err := NewReader(subscribeBytes).ReadPacket()
	if err != nil {
		t.Fatalf("Unexpected error: %s", err.Error())
	}
	p := packet.(*Subscribe)
	suback := p.NewSubBack()

	if suback.FixHeader.PacketType != SUBACK {
		t.Fatalf("FixHeader.PacketType error,want %d, got %d", SUBACK, suback.FixHeader.PacketType)
	}

	if suback.FixHeader.RemainLength != 3 {
		t.Fatalf("FixHeader.RemainLength error,want %d, got %d", 3, suback.FixHeader.RemainLength)
	}
	if suback.PacketID != p.PacketID {
		t.Fatalf("PacketID error,want %d, got %d", p.PacketID, suback.PacketID)
	}
	if !bytes.Equal(suback.Payload, []byte{1}) {
		t.Fatalf("Payload error,want %v, got %v", suback.Payload, []byte{1})
	}
}

func TestSubscribe_NewSubBackWith3Topics(t *testing.T) {
	subscribeBytes := subscribe3TopicsBuffer()
	packet, err := NewReader(subscribeBytes).ReadPacket()
	if err != nil {
		t.Fatalf("Unexpected error: %s", err.Error())
	}
	p := packet.(*Subscribe)
	suback := p.NewSubBack()
	if suback.FixHeader.PacketType != SUBACK {
		t.Fatalf("FixHeader.PacketType error,want %d, got %d", SUBACK, suback.FixHeader.PacketType)
	}

	if suback.FixHeader.RemainLength != 5 {
		t.Fatalf("FixHeader.RemainLength error,want %d, got %d", 5, suback.FixHeader.RemainLength)
	}

	if suback.PacketID != p.PacketID {
		t.Fatalf("PacketID error,want %d, got %d", p.PacketID, suback.PacketID)
	}
	if !bytes.Equal(suback.Payload, []byte{0, 1, 2}) {
		t.Fatalf("Payload error,want %v, got %v", suback.Payload, []byte{0, 1, 2})
	}
}

func TestWriteSubscribePacket(t *testing.T) {
	var tt = []struct {
		pid    uint16
		topics []Topic
	}{
		{pid: 10, topics: []Topic{
			{Name: "T0", Qos: 0},
			{Name: "T1", Qos: 1},
			{Name: "T2", Qos: 2},
		}},
		{pid: 11, topics: []Topic{
			{Name: "T0", Qos: 0},
		}},
	}

	for _, v := range tt {
		b := make([]byte, 0, 2048)
		buf := bytes.NewBuffer(b)
		sub := &Subscribe{
			PacketID: v.pid,
			Topics:   v.topics,
		}
		err := NewWriter(buf).WriteAndFlush(sub)
		if err != nil {
			t.Fatalf("unexpected error: %s, pid :%d", err.Error(), v.pid)
		}

		packet, err := NewReader(buf).ReadPacket()
		if err != nil {
			t.Fatalf("unexpected error: %s, pid :%d", err.Error(), v.pid)
		}
		n, err := buf.ReadByte()
		if err != io.EOF {
			t.Fatalf("ReadByte() error,want io.EOF,got %s and %v bytes", err, n)
		}

		if p, ok := packet.(*Subscribe); ok {
			if len(p.Topics) != len(sub.Topics) {
				t.Fatalf("len(p.Topics) error,want %d, got %v", len(sub.Topics), p.Topics)
			}
			for k, v := range p.Topics {
				if v != sub.Topics[k] {
					t.Fatalf("Topics error,want %v, got %v", sub.Topics[k], v)
				}
			}

			if p.PacketID != sub.PacketID {
				t.Fatalf("PacketID error,want %v, got %v", sub.PacketID, p.PacketID)
			}

		} else {
			t.Fatalf("Packet type error,want %v,got %v", reflect.TypeOf(&Publish{}), reflect.TypeOf(packet))
		}
	}

}
