package packets

import (
	"bytes"
	"testing"
	"reflect"
	"io"
)


func TestWritePubackPacket(t *testing.T) {
	b := make([]byte, 0, 2048)
	buf := bytes.NewBuffer(b)
	var pid uint16
	pid = 65535
	puback := &Puback{
		PacketId:pid,
	}
	err := NewWriter(buf).WriteAndFlush(puback)
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}
	packet,err := NewReader(buf).ReadPacket()
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}
	n, err := buf.ReadByte()
	if err != io.EOF {
		t.Fatalf("ReadByte() error,want io.EOF,got %s and %v bytes",err,n)
	}
	if p,ok := packet.(*Puback);ok {
		if p.PacketId != pid {
			t.Fatalf("PacketId error,want %d, got %d",pid,p.PacketId)
		}
	} else {
		t.Fatalf("Packet type error,want %v,got %v",reflect.TypeOf(&Puback{}),reflect.TypeOf(packet))
	}

}

func TestReadPubackPacket(t *testing.T) {

	pubackBytes := bytes.NewBuffer([]byte{64,2,0,1})
	packet, err := NewReader(pubackBytes).ReadPacket()
	if err != nil {
		t.Fatalf("Unexpected error: %s", err.Error())
	}
	if p,ok := packet.(*Puback);ok {
		if p.PacketId != 1 {
			t.Fatalf("PacketId   error,want %d,got %d",1,p.PacketId)
		}
	} else {
		t.Fatalf("Packet Type error,want %v,got %v",reflect.TypeOf(&Puback{}),reflect.TypeOf(packet))
	}
}