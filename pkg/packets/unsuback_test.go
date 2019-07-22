package packets

import (
	"bytes"
	"reflect"
	"testing"
)

func TestWriteUnSuback(t *testing.T) {

	unsubscribeBytes := unsubscribeOneTopicBuffer()
	packet, err := NewReader(unsubscribeBytes).ReadPacket()
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}
	p := packet.(*Unsubscribe)
	unsuback := p.NewUnSubBack()
	buf := bytes.NewBuffer(make([]byte, 0, 2048))
	err = NewWriter(buf).WriteAndFlush(unsuback)
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}
	want := []byte{0xb0, 2, 0, 10}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Fatalf("write error,want %v, got %v", want, buf.Bytes())
	}
}

func TestReadUnSuback(t *testing.T) {
	unsubackPacketBytes := bytes.NewBuffer([]byte{0xb0, 2, //FixHeader
		0,
		10,
	})
	packet, err := NewReader(unsubackPacketBytes).ReadPacket()
	if err != nil {
		t.Fatalf("Unexpected error: %s", err.Error())
	}
	if up, ok := packet.(*Unsuback); ok {
		if up.PacketID != 10 {
			t.Fatalf("WillRetain error,want %d, got %d", 10, up.PacketID)
		}
	} else {
		t.Fatalf("Packet type error,want %v,got %v", reflect.TypeOf(&Unsuback{}), reflect.TypeOf(packet))
	}
}

func TestWriteUnSubackFixheader(t *testing.T) {
	var tt = []struct {
		unsuback *Unsuback
		want     []byte
	}{
		{unsuback: &Unsuback{PacketID: 10}, want: []byte{0xb0, 2, 0, 10}},
		{unsuback: &Unsuback{PacketID: 266}, want: []byte{0xb0, 2, 1, 10}},
		{unsuback: &Unsuback{PacketID: 522}, want: []byte{0xb0, 2, 2, 10}},
	}
	for _, v := range tt {
		unsuback := v.unsuback
		buf := bytes.NewBuffer(make([]byte, 0, 2048))
		err := NewWriter(buf).WriteAndFlush(unsuback)
		if err != nil {
			t.Fatalf("unexpected error: %s", err.Error())
		}
		if !bytes.Equal(buf.Bytes(), v.want) {
			t.Fatalf("write error,want %v, got %v", v.want, buf.Bytes())
		}
	}
}
