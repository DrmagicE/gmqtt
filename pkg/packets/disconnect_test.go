package packets

import (
	"bytes"
	"reflect"
	"testing"
)

func TestReadDisconnect(t *testing.T) {

	b := []byte{0xe0, 0}
	buf := bytes.NewBuffer(b)
	packet, err := NewReader(buf).ReadPacket()
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}
	if _, ok := packet.(*Disconnect); !ok {
		t.Fatalf("Packet Type error,want %v,got %v", reflect.TypeOf(&Disconnect{}), reflect.TypeOf(packet))
	}
}

func TestWriteDisconnect(t *testing.T) {
	disconnect := &Disconnect{}
	buf := bytes.NewBuffer(make([]byte, 0, 2048))
	err := NewWriter(buf).WriteAndFlush(disconnect)
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}
	want := []byte{0xe0, 0}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Fatalf("write error,want %v, got %v", want, buf.Bytes())
	}
}
