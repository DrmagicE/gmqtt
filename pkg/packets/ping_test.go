package packets

import (
	"bytes"
	"reflect"
	"testing"
)

func TestReadPingreq(t *testing.T) {
	b := []byte{0xc0, 0}
	buf := bytes.NewBuffer(b)
	packet, err := NewReader(buf).ReadPacket()
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}
	if _, ok := packet.(*Pingreq); !ok {
		t.Fatalf("Packet Type error,want %v,got %v", reflect.TypeOf(&Pingreq{}), reflect.TypeOf(packet))
	}
}

func TestWritePingreq(t *testing.T) {
	req := &Pingreq{}
	buf := bytes.NewBuffer(make([]byte, 0, 2048))
	err := NewWriter(buf).WriteAndFlush(req)
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}
	want := []byte{0xc0, 0}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Fatalf("write error,want %v, got %v", want, buf.Bytes())
	}
}

func TestReadPingresp(t *testing.T) {
	b := []byte{0xd0, 0}
	buf := bytes.NewBuffer(b)
	packet, err := NewReader(buf).ReadPacket()
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}
	if _, ok := packet.(*Pingresp); !ok {
		t.Fatalf("Packet Type error,want %v,got %v", reflect.TypeOf(&Pingresp{}), reflect.TypeOf(packet))
	}
}

func TestWritePingresp(t *testing.T) {

	resp := &Pingresp{}
	buf := bytes.NewBuffer(make([]byte, 0, 2048))
	err := NewWriter(buf).WriteAndFlush(resp)
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}
	want := []byte{0xd0, 0}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Fatalf("write error,want %v, got %v", want, buf.Bytes())
	}
}
