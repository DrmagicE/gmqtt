package server

import (
	"time"
	"io"
	"sync"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"testing"
)

const test_max_len = 20

//mock client,only for session_test.go
func mockClient() *Client {
	b := &Server{
		config : &config{
			maxInflightMessages:test_max_len,
		},
	}
	c := b.newClient(nil)
	c.opts.CleanSession = true
	c.newSession()
	return c
}

//mock publish packet
type mockPublishPacket struct {
}

func (p *mockPublishPacket) Pack(w io.Writer) error {
	return nil
}

func (p *mockPublishPacket) Unpack(r io.Reader) error {
	return nil
}

func (p *mockPublishPacket) String() string {
	return "mock"
}

func fullInflightSession() *Client {
	c := mockClient()
	pub := new(mockPublishPacket)
	setWg := &sync.WaitGroup{}
	for i := 1; i <= 20; i++ {
		inflightElem := &inflightElem{
			pid:    packets.PacketId(i),
			packet: pub,
		}
		setWg.Add(1)
		go func() {
			defer setWg.Done()
			c.setInflight(inflightElem)
		}()
	}
	setWg.Wait()
	return c
}

func TestUnsetInflight(t *testing.T) {
	client := fullInflightSession()
	session := client.session
	for i := 1; i <= test_max_len; i++ {
		pub := new(mockPublishPacket)
		inflightElem := &inflightElem{
			pid:    packets.PacketId(i),
			packet: pub,
		}
		client.unsetInflight(inflightElem)
	}
	if len := session.inflight.Len(); len != 0 {
		t.Fatalf("len error, want %d, but %d", 0, len)
	}
	setWg := &sync.WaitGroup{}
	for i := 1; i <= test_max_len; i++ {
		pub := new(mockPublishPacket)
		inflightElem := &inflightElem{
			pid:    packets.PacketId(i),
			packet: pub,
		}
		setWg.Add(1)
		go func() {
			defer setWg.Done()
			client.setInflight(inflightElem)
		}()
	}
	setWg.Wait()
	if len := session.inflight.Len(); len != test_max_len {
		t.Fatalf("len error, want %d, but %d", test_max_len, len)
	}


	for i := 1; i <= test_max_len; i++ {
		pub := new(mockPublishPacket)
		inflightElem := &inflightElem{
			pid:    packets.PacketId(i),
			packet: pub,
		}
		client.unsetInflight(inflightElem)
		if len := session.inflight.Len(); len != test_max_len - 1 {
			t.Fatalf("len error , want %d, but %d", test_max_len - 1, len)
		}
		client.setInflight(inflightElem)
		if len := session.inflight.Len(); len != test_max_len {
			t.Fatalf("len error , want %d, but %d", test_max_len, len)
		}
	}
	if len := session.inflight.Len(); len != test_max_len {
		t.Fatalf("len error , want %d, but %d", test_max_len, len)
	}
}

func TestSetInflight(t *testing.T) {
	session := fullInflightSession()
	pub := new(mockPublishPacket)
	inflightElem := &inflightElem{
		pid:    test_max_len + 1,
		packet: pub,
	}
	token := make(chan struct{})
	go func() {
		session.setInflight(inflightElem)
		token <- struct{}{}
	}()
	select {
	case <-token:
		t.Fatal("SetInflight error, cannot push packets into full inflight queue")
	case <-time.After(1 * time.Second):

	}
}
