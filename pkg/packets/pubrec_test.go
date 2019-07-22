package packets

import (
	"bytes"
	"io"
	"reflect"
	"testing"
)

func TestWritePubrecPacket(t *testing.T) {

	b := make([]byte, 0, 2048)
	buf := bytes.NewBuffer(b)
	pid := uint16(65535)
	pubrec := &Pubrec{
		PacketID: pid,
	}
	err := NewWriter(buf).WriteAndFlush(pubrec)
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
	if p, ok := packet.(*Pubrec); ok {
		if p.PacketID != pid {
			t.Fatalf("PacketID error,want %d, got %d", pid, p.PacketID)
		}
	} else {
		t.Fatalf("Packet type error,want %v,got %v", reflect.TypeOf(&Pubrec{}), reflect.TypeOf(packet))
	}

}

func TestReadPubrecPacket(t *testing.T) {
	pubrecBytes := bytes.NewBuffer([]byte{0x50, 2, 0, 1})
	packet, err := NewReader(pubrecBytes).ReadPacket()
	if err != nil {
		t.Fatalf("Unexpected error: %s", err.Error())
	}
	if p, ok := packet.(*Pubrec); ok {
		if p.PacketID != 1 {
			t.Fatalf("PacketID  error,want %d,got %d", 1, p.PacketID)
		}
	} else {
		t.Fatalf("Packet Type error,want %v,got %v", reflect.TypeOf(&Pubrec{}), reflect.TypeOf(packet))
	}
}

func TestPubrec_NewPubrel(t *testing.T) {
	pid := uint16(10)
	pubrec := &Pubrec{
		PacketID: pid,
	}
	pubrel := pubrec.NewPubrel()
	if pubrel.PacketID != pid {
		t.Fatalf("PacketID  error,want %d,got %d", 1, pubrel.PacketID)
	}

}
