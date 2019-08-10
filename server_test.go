package gmqtt

import (
	"context"

	"net"

	"testing"

	"io"
	"reflect"

	"github.com/DrmagicE/gmqtt/pkg/packets"
)

func TestHooks(t *testing.T) {
	srv := DefaultServer()
	defer srv.Stop(context.Background())
	ln, err := net.Listen("tcp", "127.0.0.1:1883")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	srv.AddTCPListenner(ln)
	var hooks string
	srv.RegisterOnAccept(func(cs ChainStore, conn net.Conn) bool {
		hooks += "Accept"
		return true
	})

	srv.RegisterOnConnect(func(cs ChainStore, client Client) (code uint8) {
		hooks += "OnConnect"
		return packets.CodeAccepted
	})

	srv.RegisterOnConnected(func(cs ChainStore, client Client) {
		hooks += "OnConnected"
	})

	srv.RegisterOnSessionCreated(func(cs ChainStore, client Client) {
		hooks += "OnSessionCreated"
	})

	srv.RegisterOnSubscribe(func(cs ChainStore, client Client, topic packets.Topic) (qos uint8) {
		hooks += "OnSubscribe"
		return packets.QOS_1
	})

	srv.RegisterOnMsgArrived(func(cs ChainStore, client Client, msg Message) (valid bool) {
		hooks += "OnMsgArrived"
		return true
	})

	srv.RegisterOnSessionTerminated(func(cs ChainStore, client Client, reason SessionTerminatedReason) {
		hooks += "OnSessionTerminated"
	})

	srv.RegisterOnClose(func(cs ChainStore, client Client, err error) {
		hooks += "OnClose"
	})
	srv.RegisterOnStop(func(cs ChainStore) {
		hooks += "OnStop"
	})

	srv.Run()

	c, err := net.Dial("tcp", "127.0.0.1:1883")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	w := packets.NewWriter(c)
	r := packets.NewReader(c)
	w.WriteAndFlush(defaultConnectPacket())
	r.ReadPacket()

	sub := &packets.Subscribe{
		PacketID: 10,
		Topics: []packets.Topic{
			{Name: "name", Qos: packets.QOS_1},
		},
	}
	w.WriteAndFlush(sub)
	r.ReadPacket() //suback

	pub := &packets.Publish{
		Dup:       false,
		Qos:       packets.QOS_1,
		Retain:    false,
		TopicName: []byte("ok"),
		PacketID:  10,
		Payload:   []byte("payload"),
	}
	w.WriteAndFlush(pub)
	r.ReadPacket() //puback
	srv.Stop(context.Background())
	want := "AcceptOnConnectOnConnectedOnSessionCreatedOnSubscribeOnMsgArrivedOnSessionTerminatedOnCloseOnStop"
	if hooks != want {
		t.Fatalf("hooks error, want %s, got %s", want, hooks)
	}
}

func TestConnackInvalidCode(t *testing.T) {
	srv := DefaultServer()
	defer srv.Stop(context.Background())
	ln, err := net.Listen("tcp", "127.0.0.1:1883")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	srv.AddTCPListenner(ln)
	srv.Run()
	c, err := net.Dial("tcp", "127.0.0.1:1883")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	w := packets.NewWriter(c)
	r := packets.NewReader(c)
	connect := defaultConnectPacket()
	connect.ProtocolLevel = 0x01
	w.WriteAndFlush(connect)
	p, err := r.ReadPacket()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if ack, ok := p.(*packets.Connack); ok {
		if ack.Code != packets.CodeUnacceptableProtocolVersion {
			t.Fatalf("connack.Code error, want %d, but got %d", packets.CodeUnacceptableProtocolVersion, ack.Code)
		}
	} else {
		t.Fatalf("invalid type, want %v, got %v", reflect.TypeOf(&packets.Connack{}), reflect.TypeOf(p))
	}

	_, err = r.ReadPacket()
	if err != io.EOF {
		t.Fatalf("err error, want %s, but got nil", io.EOF)
	}

}

func TestConnackInvalidCodeInHooks(t *testing.T) {
	srv := DefaultServer()
	defer srv.Stop(context.Background())
	ln, err := net.Listen("tcp", "127.0.0.1:1883")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	srv.AddTCPListenner(ln)
	srv.RegisterOnConnect(func(cs ChainStore, client Client) (code uint8) {
		return packets.CodeBadUsernameorPsw
	})
	srv.Run()
	c, err := net.Dial("tcp", "127.0.0.1:1883")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	w := packets.NewWriter(c)
	r := packets.NewReader(c)
	connect := defaultConnectPacket()
	w.WriteAndFlush(connect)
	p, err := r.ReadPacket()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if ack, ok := p.(*packets.Connack); ok {
		if ack.Code != packets.CodeBadUsernameorPsw {
			t.Fatalf("connack.Code error, want %d, but got %d", packets.CodeBadUsernameorPsw, ack.Code)
		}
	} else {
		t.Fatalf("invalid type, want %v, got %v", reflect.TypeOf(&packets.Connack{}), reflect.TypeOf(p))
	}
	_, err = r.ReadPacket()
	if err != io.EOF {
		t.Fatalf("err error, want %s, but got nil", io.EOF)
	}

}

func TestZeroBytesClientId(t *testing.T) {
	srv := DefaultServer()
	defer srv.Stop(context.Background())
	ln, err := net.Listen("tcp", "127.0.0.1:1883")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	srv.AddTCPListenner(ln)
	srv.Run()
	c, err := net.Dial("tcp", "127.0.0.1:1883")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	w := packets.NewWriter(c)
	r := packets.NewReader(c)
	connect := defaultConnectPacket()
	connect.CleanSession = true
	connect.ClientID = make([]byte, 0)
	w.WriteAndFlush(connect)
	p, err := r.ReadPacket()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if ack, ok := p.(*packets.Connack); ok {
		if ack.Code != packets.CodeAccepted {
			t.Fatalf("connack.Code error, want %d, but got %d", packets.CodeAccepted, ack.Code)
		}
	} else {
		t.Fatalf("invalid type, want %v, got %v", reflect.TypeOf(&packets.Connack{}), reflect.TypeOf(p))
	}
	c2, err := net.Dial("tcp", "127.0.0.1:1883")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	w2 := packets.NewWriter(c2)
	r2 := packets.NewReader(c2)
	connect2 := defaultConnectPacket()
	connect2.CleanSession = true
	connect2.ClientID = make([]byte, 0)
	w2.WriteAndFlush(connect2)
	p, err = r2.ReadPacket()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if ack, ok := p.(*packets.Connack); ok {
		if ack.Code != packets.CodeAccepted {
			t.Fatalf("connack.Code error, want %d, but got %d", packets.CodeAccepted, ack.Code)
		}
	} else {
		t.Fatalf("invalid type, want %v, got %v", reflect.TypeOf(&packets.Connack{}), reflect.TypeOf(p))
	}
}

func TestServer_RegisterOnAccept(t *testing.T) {
	defer func() {
		if err := recover(); err == nil {
			t.Fatalf("RegisterOnAccept error, want panic")
		}
	}()
	srv := DefaultServer()
	srv.Run()
	srv.RegisterOnAccept(nil)
}

func TestServer_RegisterOnSubscribe(t *testing.T) {
	defer func() {
		if err := recover(); err == nil {
			t.Fatalf("RegisterOnSubscribe error, want panic")
		}
	}()
	srv := DefaultServer()
	srv.Run()
	srv.RegisterOnSubscribe(nil)
}

func TestServer_RegisterOnConnect(t *testing.T) {
	defer func() {
		if err := recover(); err == nil {
			t.Fatalf("RegisterOnConnect error, want panic")
		}
	}()
	srv := DefaultServer()
	srv.Run()
	srv.RegisterOnConnect(nil)
}

func TestServer_RegisterOnMsgArrived(t *testing.T) {
	defer func() {
		if err := recover(); err == nil {
			t.Fatalf("RegisterOnMsgArrived error, want panic")
		}
	}()
	srv := DefaultServer()
	srv.Run()
	srv.RegisterOnMsgArrived(nil)
}

func TestServer_RegisterOnClose(t *testing.T) {
	defer func() {
		if err := recover(); err == nil {
			t.Fatalf("RegisterOnClose error, want panic")
		}
	}()
	srv := DefaultServer()
	srv.Run()
	srv.RegisterOnClose(nil)
}

func TestServer_RegisterOnStop(t *testing.T) {
	defer func() {
		if err := recover(); err == nil {
			t.Fatalf("RegisterOnClose error, want panic")
		}
	}()
	srv := DefaultServer()
	srv.Run()
	srv.RegisterOnStop(nil)
}

func TestRandUUID(t *testing.T) {
	uuids := make(map[string]struct{})
	for i := 0; i < 100; i++ {
		uuids[getRandomUUID()] = struct{}{}
	}
	if len(uuids) != 100 {
		t.Fatalf("duplicated ID")
	}
}
