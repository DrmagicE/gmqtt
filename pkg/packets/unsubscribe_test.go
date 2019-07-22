package packets

import (
	"bytes"
	"io"
	"reflect"
	"testing"
)

func unsubscribe3TopicsBuffer() *bytes.Buffer {
	return bytes.NewBuffer([]byte{0xa2, 26, //FIxHeader
		0, 10, //pid 10
		0, 5, 97, 47, 98, 47, 99, //Topic Filter :"a/b/c"
		0, 6, 97, 47, 98, 47, 99, 99, //Topic Filter："a/b/cc"
		0, 7, 97, 47, 98, 47, 99, 99, 99, //Topic Filter："a/b/ccc"
	})
}

func unsubscribeOneTopicBuffer() *bytes.Buffer {
	return bytes.NewBuffer([]byte{0xa2, 9, //FIxHeader
		0, 10, //pid 10
		0, 5, 97, 47, 98, 47, 99, //Topic Filter :"a/b/c"
	})
}

func TestReadUnSubscribePacketWithOneTopic(t *testing.T) {

	unsubscribeBytes := unsubscribeOneTopicBuffer()
	packet, err := NewReader(unsubscribeBytes).ReadPacket()
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}
	if p, ok := packet.(*Unsubscribe); ok {
		if p.PacketID != 10 {
			t.Fatalf("PacketID error,want %d, got %d", 10, p.PacketID)
		}
		if len(p.Topics) != 1 {
			t.Fatalf("len error,want %d, got %d", 1, len(p.Topics))
		}
		if p.Topics[0] != "a/b/c" {
			t.Fatalf("p.Topics[0] error,want %s, got %s", "a/b/c", p.Topics[0])
		}

	} else {
		t.Fatalf("Packet Type error,want %v,got %v", reflect.TypeOf(&Unsubscribe{}), reflect.TypeOf(packet))
	}
}

func TestReadUnSubscribePacketWith3Topics(t *testing.T) {
	unsubscribeBytes := unsubscribe3TopicsBuffer()
	packet, err := NewReader(unsubscribeBytes).ReadPacket()
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}
	if p, ok := packet.(*Unsubscribe); ok {
		if p.PacketID != 10 {
			t.Fatalf("PacketID error,want %d, got %d", 10, p.PacketID)
		}
		if len(p.Topics) != 3 {
			t.Fatalf("len error,want %d, got %d", 3, len(p.Topics))
		}
		if p.Topics[0] != "a/b/c" {
			t.Fatalf("p.Topics[0] error,want %s, got %s", "a/b/c", p.Topics[0])
		}
		if p.Topics[1] != "a/b/cc" {
			t.Fatalf("p.Topics[1] error,want %s, got %s", "a/b/cc", p.Topics[1])
		}
		if p.Topics[2] != "a/b/ccc" {
			t.Fatalf("p.Topics[2] error,want %s, got %s", "a/b/ccc", p.Topics[2])
		}

	} else {
		t.Fatalf("Packet Type error,want %v,got %v", reflect.TypeOf(&Subscribe{}), reflect.TypeOf(packet))
	}
}

func TestWriteUnSubscribePacket(t *testing.T) {
	var tt = []struct {
		packetID PacketID
		topics   []string
	}{
		{packetID: 10, topics: []string{"a/b/c"}},
		{packetID: 266, topics: []string{"a/b/c/d"}},
		{packetID: 522, topics: []string{"a/b/c/d/e"}},
		{packetID: 10, topics: []string{"a/b/c", "a/b/cc", "a/b/ccc"}},
		{packetID: 266, topics: []string{"a/b/c/d", "a/b/c/dd", "a/b/c/ddd"}},
		{packetID: 522, topics: []string{"a/b/c/d/e", "a/b/c/d/ee", "a/b/c/d/eee"}},
	}

	for _, v := range tt {
		b := make([]byte, 0, 2048)
		buf := bytes.NewBuffer(b)
		uns := &Unsubscribe{
			PacketID: v.packetID,
			Topics:   v.topics,
		}
		err := NewWriter(buf).WriteAndFlush(uns)
		if err != nil {
			t.Fatalf("unexpected error: %s,%v", err.Error(), string(v.packetID))
		}
		packet, err := NewReader(buf).ReadPacket()
		if err != nil {
			t.Fatalf("unexpected error: %s,%v", err.Error(), string(v.packetID))
		}
		n, err := buf.ReadByte()
		if err != io.EOF {
			t.Fatalf("ReadByte() error,want io.EOF,got %s and %v bytes", err, n)
		}
		if p, ok := packet.(*Unsubscribe); ok {
			if len(p.Topics) == len(uns.Topics) {
				for i := 0; i < len(p.Topics); i++ {
					if p.Topics[i] != uns.Topics[i] {
						t.Fatalf("p.Topics[%v] error,want %s, got %s", i, uns.Topics[i], p.Topics[i])
					}
				}
			} else {
				t.Fatalf("Topics slice length error,want %v, got %v", len(uns.Topics), len(p.Topics))
			}
			if p.PacketID != uns.PacketID {
				t.Fatalf("PacketID error,want %v, got %v", uns.PacketID, p.PacketID)
			}

		} else {
			t.Fatalf("Packet type error,want %v,got %v", reflect.TypeOf(&Unsubscribe{}), reflect.TypeOf(packet))
		}
	}
}

func TestUnsubscribe_NewUnSubBack(t *testing.T) {
	unsubscribeBytes := unsubscribeOneTopicBuffer()
	packet, err := NewReader(unsubscribeBytes).ReadPacket()
	if err != nil {
		t.Fatalf("Unexpected error: %s", err.Error())
	}
	p := packet.(*Unsubscribe)
	unsuback := p.NewUnSubBack()

	if unsuback.FixHeader.PacketType != UNSUBACK {
		t.Fatalf("FixHeader.PacketType error,want %d, got %d", UNSUBACK, unsuback.FixHeader.PacketType)
	}

	if unsuback.FixHeader.RemainLength != 2 {
		t.Fatalf("FixHeader.RemainLength error,want %d, got %d", 2, unsuback.FixHeader.RemainLength)
	}
	if unsuback.PacketID != p.PacketID {
		t.Fatalf("PacketID error,want %d, got %d", p.PacketID, unsuback.PacketID)
	}
}

/*
func TestSubscribe_NewSubBackWith3Topics(t *testing.T) {
	subscribeBytes := subscribe3TopicsBuffer()
	packet, err := NewReader(subscribeBytes).ReadPacket()
	if err != nil {
		t.Fatalf("Unexpected error: %s", err.Error())
	}
	var p *Subscribe
	p = packet.(*Subscribe)
	suback := p.NewSubBack()
	if suback.FixHeader.PacketType != SUBACK {
		t.Fatalf("FixHeader.PacketType error,want %d, got %d",SUBACK,suback.FixHeader.PacketType)
	}

	if suback.FixHeader.RemainLength != 5 {
		t.Fatalf("FixHeader.RemainLength error,want %d, got %d",5,suback.FixHeader.RemainLength)
	}

	if suback.PacketID != p.PacketID {
		t.Fatalf("PacketID error,want %d, got %d",p.PacketID,suback.PacketID)
	}
	if !bytes.Equal(suback.Payload,[]byte{0, 1, 2}) {
		t.Fatalf("Payload error,want %v, got %v",suback.Payload,[]byte{0, 1, 2})
	}
}*/
