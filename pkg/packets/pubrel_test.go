package packets

import (
	"bytes"
	"io"
	"reflect"
	"testing"
)

func TestWritePubrelPacket(t *testing.T) {
	b := make([]byte, 0, 2048)
	buf := bytes.NewBuffer(b)
	pid := uint16(65535)
	pubrel := &Pubrel{
		PacketID: pid,
	}
	err := NewWriter(buf).WriteAndFlush(pubrel)
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
	if p, ok := packet.(*Pubrel); ok {
		if p.PacketID != pid {
			t.Fatalf("PacketID error,want %d, got %d", pid, p.PacketID)
		}
	} else {
		t.Fatalf("Packet type error,want %v,got %v", reflect.TypeOf(&Pubrec{}), reflect.TypeOf(packet))
	}

}

func TestReadPubrelPacket(t *testing.T) {
	pubrelBytes := bytes.NewBuffer([]byte{0x62, 2, 0, 1})
	packet, err := NewReader(pubrelBytes).ReadPacket()
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}
	if p, ok := packet.(*Pubrel); ok {
		if p.PacketID != 1 {
			t.Fatalf("PacketID  error,want %d,got %d", 1, p.PacketID)
		}
	} else {
		t.Fatalf("Packet Type error,want %v,got %v", reflect.TypeOf(&Pubrel{}), reflect.TypeOf(packet))
	}
}

func TestPubrel_NewPubcomp(t *testing.T) {
	pid := uint16(10)
	pubrel := &Pubrel{
		PacketID: pid,
	}
	pubcomp := pubrel.NewPubcomp()
	if pubcomp.PacketID != pid {
		t.Fatalf("PacketID  error,want %d,got %d", 1, pubcomp.PacketID)
	}
}
