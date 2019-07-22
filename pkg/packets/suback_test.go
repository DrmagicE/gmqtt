package packets

import (
	"bytes"
	"reflect"
	"testing"
)

func TestReadSuback(t *testing.T) {
	subackBytes := bytes.NewBuffer([]byte{0x90, 5, //FixHeader
		0, 10, //packetID
		0, 1, 2, //payload
	})
	packet, err := NewReader(subackBytes).ReadPacket()
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}
	if p, ok := packet.(*Suback); ok {
		if p.PacketID != 10 {
			t.Fatalf("PacketID error,want %d, got %d", 10, p.PacketID)
		}
		if !bytes.Equal(p.Payload, []byte{0, 1, 2}) {
			t.Fatalf("Payload error,want %v,got %v", []byte{0, 1, 2}, p.Payload)
		}

	} else {
		t.Fatalf("Packet Type error,want %v,got %v", reflect.TypeOf(&Suback{}), reflect.TypeOf(packet))
	}
}

func TestWriteSubackWithOneTopic(t *testing.T) {
	subscribeBytes := subscribeOneTopicBuffer()
	packet, err := NewReader(subscribeBytes).ReadPacket()
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}
	p := packet.(*Subscribe)
	suback := p.NewSubBack()
	buf := bytes.NewBuffer(make([]byte, 0, 2048))
	err = NewWriter(buf).WriteAndFlush(suback)
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}
	want := []byte{0x90, 3, 0, 10, 1}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Fatalf("write error,want %v, got %v", want, buf.Bytes())
	}
}

func TestWriteSubackWith3Topics(t *testing.T) {
	subscribeBytes := subscribe3TopicsBuffer()
	packet, err := NewReader(subscribeBytes).ReadPacket()
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}
	p := packet.(*Subscribe)
	suback := p.NewSubBack()
	buf := bytes.NewBuffer(make([]byte, 0, 2048))
	err = NewWriter(buf).WriteAndFlush(suback)
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}
	want := []byte{0x90, 5, 0, 10, 0, 1, 2}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Fatalf("write error,want %v, got %v", want, buf.Bytes())
	}
}
