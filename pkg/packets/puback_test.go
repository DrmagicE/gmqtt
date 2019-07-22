package packets

import (
	"bytes"
	"io"
	"reflect"
	"testing"
)

func TestWritePubackPacket(t *testing.T) {
	b := make([]byte, 0, 2048)
	buf := bytes.NewBuffer(b)
	pid := uint16(65535)
	puback := &Puback{
		PacketID: pid,
	}
	err := NewWriter(buf).WriteAndFlush(puback)
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}
	packet, err := NewReader(buf).ReadPacket()
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}
	n, err := buf.ReadByte()
	if err != io.EOF {
		t.Fatalf("ReadByte() error,want io.EOF,got %s and %v bytes", err, n)
	}
	if p, ok := packet.(*Puback); ok {
		if p.PacketID != pid {
			t.Fatalf("PacketID error,want %d, got %d", pid, p.PacketID)
		}
	} else {
		t.Fatalf("Packet type error,want %v,got %v", reflect.TypeOf(&Puback{}), reflect.TypeOf(packet))
	}

}

func TestReadPubackPacket(t *testing.T) {

	pubackBytes := bytes.NewBuffer([]byte{64, 2, 0, 1})
	packet, err := NewReader(pubackBytes).ReadPacket()
	if err != nil {
		t.Fatalf("Unexpected error: %s", err.Error())
	}
	if p, ok := packet.(*Puback); ok {
		if p.PacketID != 1 {
			t.Fatalf("PacketID   error,want %d,got %d", 1, p.PacketID)
		}
	} else {
		t.Fatalf("Packet Type error,want %v,got %v", reflect.TypeOf(&Puback{}), reflect.TypeOf(packet))
	}
}
