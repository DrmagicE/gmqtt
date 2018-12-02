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

func TestServer_db_subscribe_unsubscribe(t *testing.T) {
	srv := NewServer()
	stt := []struct {
		topicName string
		clientId  string
		topic     packets.Topic
	}{
		{topicName: "name0", clientId: "id0", topic: packets.Topic{Name: "name0", Qos: packets.QOS_0}},
		{topicName: "name1", clientId: "id1", topic: packets.Topic{Name: "name1", Qos: packets.QOS_1}},
		{topicName: "name2", clientId: "id2", topic: packets.Topic{Name: "name2", Qos: packets.QOS_2}},
		{topicName: "name3", clientId: "id0", topic: packets.Topic{Name: "name3", Qos: packets.QOS_2}},
	}
	utt := []struct{
		topicName string
		clientId string
	}{
		{topicName:"name0",clientId:"id0"},{topicName:"name1",clientId:"id1"},
	}
	ugot := []struct {
		topicName string
		clientId  string
		topic     packets.Topic
	}{
		{topicName: "name2", clientId: "id2", topic: packets.Topic{Name: "name2", Qos: packets.QOS_2}},
		{topicName: "name3", clientId: "id0", topic: packets.Topic{Name: "name3", Qos: packets.QOS_2}},
	}

	srv.subscriptionsDB.Lock()
	defer srv.subscriptionsDB.Unlock()
	for _, v := range stt {
		srv.subscriptionsDB.init(v.clientId, v.topicName)
		srv.subscribe(v.clientId, v.topic)
	}
	for _, v := range stt {
		if got := srv.subscriptionsDB.topicsByName[v.topicName][v.clientId]; got != v.topic {
			t.Fatalf("subscriptionsDB.topicsByName[%s][%s] error, want %v, got %v", v.topicName, v.clientId, v.topic, got)
		}
		if got := srv.subscriptionsDB.topicsById[v.clientId][v.topicName]; got != v.topic {
			t.Fatalf("subscriptionsDB.topicsById[%s][%s] error, want %v, got %v", v.clientId, v.topicName, v.topic, got)
		}
		if !srv.subscriptionsDB.exist(v.clientId, v.topicName) {
			t.Fatalf("exist() error")
		}
	}
	if len(srv.subscriptionsDB.topicsByName) != 4 || len(srv.subscriptionsDB.topicsById) != 3{
		t.Fatalf("len error,got %d, %d", len(srv.subscriptionsDB.topicsByName), len(srv.subscriptionsDB.topicsById) )
	}

	for _, v := range utt {
		srv.unsubscribe(v.clientId, v.topicName)
	}

	for _, v := range ugot {
		if got := srv.subscriptionsDB.topicsByName[v.topicName][v.clientId]; got != v.topic {
			t.Fatalf("subscriptionsDB.topicsByName[%s][%s] error, want %v, got %v", v.topicName, v.clientId, v.topic, got)
		}
		if got := srv.subscriptionsDB.topicsById[v.clientId][v.topicName]; got != v.topic {
			t.Fatalf("subscriptionsDB.topicsById[%s][%s] error, want %v, got %v", v.clientId, v.topicName, v.topic, got)
		}
		if !srv.subscriptionsDB.exist(v.clientId, v.topicName) {
			t.Fatalf("exist() error")
		}
	}
	if len(srv.subscriptionsDB.topicsByName) != 2 || len(srv.subscriptionsDB.topicsById) != 2{
		t.Fatalf("len error,got %d, %d", len(srv.subscriptionsDB.topicsByName), len(srv.subscriptionsDB.topicsById) )
	}
}


func TestServer_removeClientSubscriptions(t *testing.T) {
	srv := NewServer()
	stt := []struct {
		topicName string
		clientId  string
		topic     packets.Topic
	}{
		{topicName: "name0", clientId: "id0", topic: packets.Topic{Name: "name0", Qos: packets.QOS_0}},
		{topicName: "name1", clientId: "id1", topic: packets.Topic{Name: "name1", Qos: packets.QOS_1}},
		{topicName: "name2", clientId: "id2", topic: packets.Topic{Name: "name2", Qos: packets.QOS_2}},
		{topicName: "name3", clientId: "id0", topic: packets.Topic{Name: "name3", Qos: packets.QOS_2}},
	}

	srv.subscriptionsDB.Lock()
	defer srv.subscriptionsDB.Unlock()
	for _, v := range stt {
		srv.subscriptionsDB.init(v.clientId, v.topicName)
		srv.subscribe(v.clientId, v.topic)
	}
	removedCid := "id0"
	srv.removeClientSubscriptions(removedCid)
	for _, v := range stt {
		if v.clientId == removedCid {
			if srv.subscriptionsDB.exist(v.clientId, v.topicName){
				t.Fatalf("exist() error")
			}
			continue
		}
		if got := srv.subscriptionsDB.topicsByName[v.topicName][v.clientId]; got != v.topic {
			t.Fatalf("subscriptionsDB.topicsByName[%s][%s] error, want %v, got %v", v.topicName, v.clientId, v.topic, got)
		}
		if got := srv.subscriptionsDB.topicsById[v.clientId][v.topicName]; got != v.topic {
			t.Fatalf("subscriptionsDB.topicsById[%s][%s] error, want %v, got %v", v.clientId, v.topicName, v.topic, got)
		}
		if !srv.subscriptionsDB.exist(v.clientId, v.topicName) {
			t.Fatalf("exist() error")
		}
	}
	if len(srv.subscriptionsDB.topicsByName) != 2 || len(srv.subscriptionsDB.topicsById) != 2{
		t.Fatalf("len error,got %d, %d", len(srv.subscriptionsDB.topicsByName), len(srv.subscriptionsDB.topicsById) )
	}

}

func BenchmarkServer_subscribe(b *testing.B) {
	b.StopTimer()
	s := NewServer()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		id := strconv.Itoa(i)
		s.subscribe(id, packets.Topic{
			Qos: packets.QOS_1, Name: id,
		})
	}
	for i := 0; i < b.N; i++ {
		id := strconv.Itoa(i)
		s.unsubscribe(id, id)
	}
	b.StopTimer()
}
