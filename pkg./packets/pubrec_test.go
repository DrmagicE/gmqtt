package packets

import (
	"reflect"
	"bytes"
	"io"
	"testing"
)

func TestWritePubrecPacket(t *testing.T) {

	b := make([]byte, 0, 2048)
	buf := bytes.NewBuffer(b)
	var pid uint16
	pid = 65535
	pubrec := &Pubrec{
		PacketId:pid,
	}
	err := NewWriter(buf).WritePacket(pubrec)
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}
	packet,err := NewReader(buf).ReadPacket()
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}
	n, err := buf.ReadByte()
	if err != io.EOF {
		t.Fatalf("ReadByte() error,want io.EOF,got %s and %n bytes",err,n)
	}
	if p,ok := packet.(*Pubrec);ok {
		if p.PacketId != pid {
			t.Fatalf("PacketId error,want %d, got %d",pid,p.PacketId)
		}
	} else {
		t.Fatalf("Packet type error,want %v,got %v",reflect.TypeOf(&Pubrec{}),reflect.TypeOf(packet))
	}

}

func TestReadPubrecPacket(t *testing.T) {
	pubrecBytes := bytes.NewBuffer([]byte{0x50,2,0,1})
	packet, err := NewReader(pubrecBytes).ReadPacket()
	if err != nil {
		t.Fatalf("Unexpected error: %s", err.Error())
	}
	if p,ok := packet.(*Pubrec);ok {
		if p.PacketId != 1 {
			t.Fatalf("PacketId  error,want %d,got %d",1,p.PacketId)
		}
	} else {
		t.Fatalf("Packet Type error,want %v,got %v",reflect.TypeOf(&Pubrec{}),reflect.TypeOf(packet))
	}
}

func TestPubrec_NewPubrel(t *testing.T) {
	var pid uint16
	pid = 10
	pubrec := &Pubrec{
		PacketId:pid,
	}
	pubrel := pubrec.NewPubrel()
	if pubrel.PacketId != pid {
		t.Fatalf("PacketId  error,want %d,got %d",1,pubrel.PacketId)
	}

}