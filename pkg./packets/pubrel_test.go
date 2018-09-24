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
	var pid uint16
	pid = 65535
	pubrel := &Pubrel{
		PacketId:pid,
	}
	err := NewWriter(buf).WritePacket(pubrel)
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
	if p,ok := packet.(*Pubrel);ok {
		if p.PacketId != pid {
			t.Fatalf("PacketId error,want %d, got %d",pid,p.PacketId)
		}
	} else {
		t.Fatalf("Packet type error,want %v,got %v",reflect.TypeOf(&Pubrec{}),reflect.TypeOf(packet))
	}

}

func TestReadPubrelPacket(t *testing.T) {
	pubrelBytes := bytes.NewBuffer([]byte{0x62,2,0,1})
	packet, err := NewReader(pubrelBytes).ReadPacket()
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}
	if p,ok := packet.(*Pubrel);ok {
		if p.PacketId != 1 {
			t.Fatalf("PacketId  error,want %d,got %d",1,p.PacketId)
		}
	} else {
		t.Fatalf("Packet Type error,want %v,got %v",reflect.TypeOf(&Pubrel{}),reflect.TypeOf(packet))
	}
}


func TestPubrel_NewPubcomp(t *testing.T) {
	var pid uint16
	pid = 10
	pubrel := &Pubrel{
		PacketId:pid,
	}
	pubcomp := pubrel.NewPubcomp()
	if pubcomp.PacketId != pid {
		t.Fatalf("PacketId  error,want %d,got %d",1,pubcomp.PacketId)
	}
}