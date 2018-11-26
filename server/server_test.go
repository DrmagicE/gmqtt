package server

import (
	"context"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"net"
	"strconv"
	"testing"
)

func TestHooks(t *testing.T) {
	srv := NewServer()
	ln, err := net.Listen("tcp", "127.0.0.1:1883")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	srv.AddTCPListenner(ln)
	var hooks string
	srv.OnAccept = func(conn net.Conn) bool {
		hooks += "Accept"
		return true
	}
	srv.OnConnect = func(client *Client) (code uint8) {
		hooks += "OnConnect"
		return packets.CODE_ACCEPTED
	}
	srv.OnSubscribe = func(client *Client, topic packets.Topic) uint8 {
		hooks += "OnSubscribe"
		return packets.QOS_1
	}

	srv.OnPublish = func(client *Client, publish *packets.Publish) bool {
		hooks += "OnPublish"
		return true
	}

	srv.OnClose = func(client *Client, err error) {
		hooks += "OnClose"

	}
	srv.OnStop = func() {
		hooks += "OnStop"
	}

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
		PacketId: 10,
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
		PacketId:  10,
		Payload:   []byte("payload"),
	}
	w.WriteAndFlush(pub)
	r.ReadPacket() //puback
	srv.Stop(context.Background())
	want := "AcceptOnConnectOnSubscribeOnPublishOnCloseOnStop"
	if hooks != want {
		t.Fatalf("hooks error, want %s, got %s", want, hooks)
	}
}
func TestServer_Subscribe_Unsubscribe(t *testing.T) {
	srv := NewServer()
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
	w.WriteAndFlush(defaultConnectPacket()) //connect
	r.ReadPacket()                          //ack

	cid := string(defaultConnectPacket().ClientId)

	srv.Subscribe(cid, []packets.Topic{
		{2, "qos2"},
		{1, "qos1"},
	})
	srv.topicsMu.Lock()
	want1 := packets.Topic{2, "qos2"}
	if srv.topics[cid]["qos2"] != want1 {
		t.Fatalf("Subscribe() error, want %v, but %v", want1, srv.topics[cid]["qos2"])
	}
	want2 := packets.Topic{1, "qos1"}
	if (srv.topics[cid]["qos1"] != packets.Topic{1, "qos1"}) {
		t.Fatalf("Subscribe() error, want %v, but %v", want2, srv.topics[cid]["qos1"])
	}
	srv.topicsMu.Unlock()
	if len(srv.Monitor.ClientSubscriptions(cid)) != 2 {
		t.Fatalf("lenth of Monitor.ClientSubscriptions() error, want %d, but %d", 2, len(srv.Monitor.ClientSubscriptions(cid)))
	}
	srv.UnSubscribe(cid, []string{"qos2"})
	srv.topicsMu.Lock()
	if _, ok := srv.topics[cid]["qos2"]; ok {
		t.Fatalf("unexpected behaviour, topic qos2 should be removed but not")
	}
	if _, ok := srv.topics[cid]["qos1"]; !ok {
		t.Fatalf("unexpected behaviour, topic qos1 should not be removed but removed")
	}
	srv.topicsMu.Unlock()

	if len(srv.Monitor.ClientSubscriptions(cid)) != 1 {
		t.Fatalf("lenth of Monitor.ClientSubscriptions() error, want %d, but %d", 1, len(srv.Monitor.ClientSubscriptions(cid)))
	}
}
func TestServer_Publish(t *testing.T) {
	s := NewServer()
	pub := &packets.Publish{
		TopicName: []byte{0x31, 0x32},
		Qos:       2,
		PacketId:  3,
	}
	s.Publish(pub, "1", "2", "3")
	msgRouter := <-s.msgRouter

	if msgRouter.forceBroadcast != false {
		t.Fatalf("msgRouter.forceBroadcast error, want false,but got true")
	}
	for i := 0; i < 3; i++ {
		want := strconv.Itoa(i + 1)
		if msgRouter.clientIds[i] != want {
			t.Fatalf("msgRouter.clientIds[%d] error, want %s,but got %s", i, want, msgRouter.clientIds[i])
		}
	}
	if msgRouter.pub != pub {
		t.Fatalf("msgRouter.pub error, want %s, but got %s", pub, msgRouter.pub)
	}

}
func TestServer_Broadcast(t *testing.T) {
	s := NewServer()
	pub := &packets.Publish{
		TopicName: []byte{0x31, 0x32},
		Qos:       2,
		PacketId:  3,
	}
	s.Broadcast(pub, "1", "2", "3")
	msgRouter := <-s.msgRouter
	if msgRouter.forceBroadcast != true {
		t.Fatalf("msgRouter.forceBroadcast error, want true,but got false")
	}
	for i := 0; i < 3; i++ {
		want := strconv.Itoa(i + 1)
		if msgRouter.clientIds[i] != want {
			t.Fatalf("msgRouter.clientIds[%d] error, want %s,but got %s", i, want, msgRouter.clientIds[i])
		}
	}
	if msgRouter.pub != pub {
		t.Fatalf("msgRouter.pub error, want %s, but got %s", pub, msgRouter.pub)
	}

}

func TestServer_registerHandlerOnError1(t *testing.T) {
	srv := newTestServer()
	c := srv.newClient(nil)
	conn := defaultConnectPacket()
	errCode := uint8(packets.CODE_SERVER_UNAVAILABLE)
	conn.AckCode = errCode
	register := &register{
		client:  c,
		connect: conn,
	}
	srv.registerHandler(register)
	ack := <-c.out
	connack := ack.(*packets.Connack)
	if connack.Code != errCode {
		t.Fatalf("connack.Code error, want %d, but got %d", errCode, connack.Code)
	}
	select {
	case <-c.error:
	default:
		t.Fatalf("unexpected error")
	}
	if register.error == nil {
		t.Fatalf("register.error should not be nil")
	}
}
func TestServer_registerHandlerOnError2(t *testing.T) {
	srv := newTestServer()
	errCode := uint8(packets.CODE_BAD_USERNAME_OR_PSW)
	srv.OnConnect = func(client *Client) (code uint8) {
		return errCode
	}
	c := srv.newClient(nil)
	conn := defaultConnectPacket()
	register := &register{
		client:  c,
		connect: conn,
	}
	srv.registerHandler(register)
	ack := <-c.out
	connack := ack.(*packets.Connack)
	if connack.Code != errCode {
		t.Fatalf("connack.Code error, want %d, but got %d", errCode, connack.Code)
	}
	select {
	case <-c.error:
	default:
		t.Fatalf("unexpected error")
	}
	if register.error == nil {
		t.Fatalf("register.error should not be nil")
	}
}
