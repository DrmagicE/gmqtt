package gmqtt

import (
	"context"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"io"
	"net"
	"reflect"
	"testing"
	"time"
)

func TestHooks(t *testing.T) {
	srv := NewServer()
	defer srv.Stop(context.Background())
	ln, err := net.Listen("tcp", "127.0.0.1:1883")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	srv.AddTCPListenner(ln)
	var hooks string
	srv.RegisterOnAccept(func(conn net.Conn) bool {
		hooks += "Accept"
		return true
	})
	srv.RegisterOnConnect(func(client *Client) (code uint8) {
		hooks += "OnConnect"
		return packets.CodeAccepted
	})

	srv.RegisterOnSubscribe(func(client *Client, topic packets.Topic) uint8 {
		hooks += "OnSubscribe"
		return packets.QOS_1
	})

	srv.RegisterOnPublish(func(client *Client, publish *packets.Publish) bool {
		hooks += "OnPublish"
		return true
	})

	srv.RegisterOnClose(func(client *Client, err error) {
		hooks += "OnClose"
	})
	srv.RegisterOnStop(func() {
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
	want := "AcceptOnConnectOnSubscribeOnPublishOnCloseOnStop"
	if hooks != want {
		t.Fatalf("hooks error, want %s, got %s", want, hooks)
	}
}

func TestConnackInvalidCode(t *testing.T) {
	srv := NewServer()
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
	srv := NewServer()
	defer srv.Stop(context.Background())
	ln, err := net.Listen("tcp", "127.0.0.1:1883")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	srv.AddTCPListenner(ln)
	srv.RegisterOnConnect(func(client *Client) (code uint8) {
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
	srv := NewServer()
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
	if len(srv.Monitor.Clients()) != 2 {
		t.Fatalf("len error, want 2, got %d", len(srv.Monitor.Clients()))
	}

}

func TestServer_db_subscribe_unsubscribe(t *testing.T) {
	srv := NewServer()
	stt := []struct {
		topicName string
		clientID  string
		topic     packets.Topic
	}{
		{topicName: "name0", clientID: "id0", topic: packets.Topic{Name: "name0", Qos: packets.QOS_0}},
		{topicName: "name1", clientID: "id1", topic: packets.Topic{Name: "name1", Qos: packets.QOS_1}},
		{topicName: "name2", clientID: "id2", topic: packets.Topic{Name: "name2", Qos: packets.QOS_2}},
		{topicName: "name3", clientID: "id0", topic: packets.Topic{Name: "name3", Qos: packets.QOS_2}},
	}
	utt := []struct {
		topicName string
		clientID  string
	}{
		{topicName: "name0", clientID: "id0"}, {topicName: "name1", clientID: "id1"},
	}
	ugot := []struct {
		topicName string
		clientID  string
		topic     packets.Topic
	}{
		{topicName: "name2", clientID: "id2", topic: packets.Topic{Name: "name2", Qos: packets.QOS_2}},
		{topicName: "name3", clientID: "id0", topic: packets.Topic{Name: "name3", Qos: packets.QOS_2}},
	}

	srv.subscriptionsDB.Lock()
	defer srv.subscriptionsDB.Unlock()
	for _, v := range stt {
		srv.subscriptionsDB.init(v.clientID, v.topicName)
		srv.subscribe(v.clientID, v.topic)
	}
	for _, v := range stt {
		if got := srv.subscriptionsDB.topicsByName[v.topicName][v.clientID]; got != v.topic {
			t.Fatalf("subscriptionsDB.topicsByName[%s][%s] error, want %v, got %v", v.topicName, v.clientID, v.topic, got)
		}
		if got := srv.subscriptionsDB.topicsByID[v.clientID][v.topicName]; got != v.topic {
			t.Fatalf("subscriptionsDB.topicsByID[%s][%s] error, want %v, got %v", v.clientID, v.topicName, v.topic, got)
		}
		if !srv.subscriptionsDB.exist(v.clientID, v.topicName) {
			t.Fatalf("exist() error")
		}
	}
	if len(srv.subscriptionsDB.topicsByName) != 4 || len(srv.subscriptionsDB.topicsByID) != 3 {
		t.Fatalf("len error,got %d, %d", len(srv.subscriptionsDB.topicsByName), len(srv.subscriptionsDB.topicsByID))
	}

	for _, v := range utt {
		srv.unsubscribe(v.clientID, v.topicName)
	}

	for _, v := range ugot {
		if got := srv.subscriptionsDB.topicsByName[v.topicName][v.clientID]; got != v.topic {
			t.Fatalf("subscriptionsDB.topicsByName[%s][%s] error, want %v, got %v", v.topicName, v.clientID, v.topic, got)
		}
		if got := srv.subscriptionsDB.topicsByID[v.clientID][v.topicName]; got != v.topic {
			t.Fatalf("subscriptionsDB.topicsByID[%s][%s] error, want %v, got %v", v.clientID, v.topicName, v.topic, got)
		}
		if !srv.subscriptionsDB.exist(v.clientID, v.topicName) {
			t.Fatalf("exist() error")
		}
	}
	if len(srv.subscriptionsDB.topicsByName) != 2 || len(srv.subscriptionsDB.topicsByID) != 2 {
		t.Fatalf("len error,got %d, %d", len(srv.subscriptionsDB.topicsByName), len(srv.subscriptionsDB.topicsByID))
	}
}

func TestServer_removeClientSubscriptions(t *testing.T) {
	srv := NewServer()
	stt := []struct {
		topicName string
		clientID  string
		topic     packets.Topic
	}{
		{topicName: "name0", clientID: "id0", topic: packets.Topic{Name: "name0", Qos: packets.QOS_0}},
		{topicName: "name1", clientID: "id1", topic: packets.Topic{Name: "name1", Qos: packets.QOS_1}},
		{topicName: "name2", clientID: "id2", topic: packets.Topic{Name: "name2", Qos: packets.QOS_2}},
		{topicName: "name3", clientID: "id0", topic: packets.Topic{Name: "name3", Qos: packets.QOS_2}},
	}

	srv.subscriptionsDB.Lock()
	defer srv.subscriptionsDB.Unlock()
	for _, v := range stt {
		srv.subscriptionsDB.init(v.clientID, v.topicName)
		srv.subscribe(v.clientID, v.topic)
	}
	removedCid := "id0"
	srv.removeClientSubscriptions(removedCid)
	for _, v := range stt {
		if v.clientID == removedCid {
			if srv.subscriptionsDB.exist(v.clientID, v.topicName) {
				t.Fatalf("exist() error")
			}
			continue
		}
		if got := srv.subscriptionsDB.topicsByName[v.topicName][v.clientID]; got != v.topic {
			t.Fatalf("subscriptionsDB.topicsByName[%s][%s] error, want %v, got %v", v.topicName, v.clientID, v.topic, got)
		}
		if got := srv.subscriptionsDB.topicsByID[v.clientID][v.topicName]; got != v.topic {
			t.Fatalf("subscriptionsDB.topicsByID[%s][%s] error, want %v, got %v", v.clientID, v.topicName, v.topic, got)
		}
		if !srv.subscriptionsDB.exist(v.clientID, v.topicName) {
			t.Fatalf("exist() error")
		}
	}
	if len(srv.subscriptionsDB.topicsByName) != 2 || len(srv.subscriptionsDB.topicsByID) != 2 {
		t.Fatalf("len error,got %d, %d", len(srv.subscriptionsDB.topicsByName), len(srv.subscriptionsDB.topicsByID))
	}

}

func TestServer_RegisterOnAccept(t *testing.T) {
	defer func() {
		if err := recover(); err == nil {
			t.Fatalf("RegisterOnAccept error, want panic")
		}
	}()
	srv := newTestServer()
	srv.Run()
	srv.RegisterOnAccept(nil)
}

func TestServer_RegisterOnSubscribe(t *testing.T) {
	defer func() {
		if err := recover(); err == nil {
			t.Fatalf("RegisterOnSubscribe error, want panic")
		}
	}()
	srv := newTestServer()
	srv.Run()
	srv.RegisterOnSubscribe(nil)
}

func TestServer_RegisterOnConnect(t *testing.T) {
	defer func() {
		if err := recover(); err == nil {
			t.Fatalf("RegisterOnConnect error, want panic")
		}
	}()
	srv := newTestServer()
	srv.Run()
	srv.RegisterOnConnect(nil)
}

func TestServer_RegisterOnPublish(t *testing.T) {
	defer func() {
		if err := recover(); err == nil {
			t.Fatalf("RegisterOnPublish error, want panic")
		}
	}()
	srv := newTestServer()
	srv.Run()
	srv.RegisterOnPublish(nil)
}

func TestServer_RegisterOnClose(t *testing.T) {
	defer func() {
		if err := recover(); err == nil {
			t.Fatalf("RegisterOnClose error, want panic")
		}
	}()
	srv := newTestServer()
	srv.Run()
	srv.RegisterOnClose(nil)
}

func TestServer_RegisterOnStop(t *testing.T) {
	defer func() {
		if err := recover(); err == nil {
			t.Fatalf("RegisterOnClose error, want panic")
		}
	}()
	srv := newTestServer()
	srv.Run()
	srv.RegisterOnStop(nil)
}

func TestServer_SetMaxInflightMessages(t *testing.T) {
	srv := newTestServer()
	srv.SetMaxInflightMessages(65536)
	if srv.config.maxInflightMessages != maxInflightMessages {
		t.Fatalf("SetMaxInflightMessages() error, want %d, got %d", maxInflightMessages, srv.config.maxInflightMessages)
	}
	srv.SetMaxInflightMessages(20)
	if srv.config.maxInflightMessages != 20 {
		t.Fatalf("SetMaxInflightMessages() error, want %d, got %d", 20, srv.config.maxInflightMessages)
	}
}

func TestServer_SetFn(t *testing.T) {

	srv := newTestServer()
	srv.SetMsgRouterLen(100)
	srv.SetMaxInflightMessages(200)
	srv.SetRegisterLen(100)
	srv.SetUnregisterLen(100)
	srv.SetMaxQueueMessages(20)
	srv.SetQueueQos0Messages(false)
	srv.SetDeliveryRetryInterval(25 * time.Second)

	if cap(srv.msgRouter) != 100 {
		t.Fatalf("SetMsgRouterLen() error")
	}
	if cap(srv.register) != 100 {
		t.Fatalf("SetRegisterLen() error")
	}
	if cap(srv.unregister) != 100 {
		t.Fatalf("SetUnregisterLen() error")
	}
	if srv.config.maxInflightMessages != 200 {
		t.Fatalf("SetMaxInflightMessages() error")
	}

	if srv.config.maxQueueMessages != 20 {
		t.Fatalf("SetMaxQueueMessages() error")
	}
	if srv.config.queueQos0Messages != false {
		t.Fatalf("SetQueueQos0Messages() error")
	}
	if srv.config.deliveryRetryInterval != 25*time.Second {
		t.Fatalf("SetDeliveryRetryInterval() error")
	}

}

func TestServer_SetFnPanic(t *testing.T) {
	defer func() {
		if err := recover(); err == nil {
			t.Fatalf("set fn error, want panic")
		}
	}()
	srv := newTestServer()
	srv.Run()
	srv.SetMsgRouterLen(100)
	srv.SetMaxInflightMessages(200)
	srv.SetRegisterLen(100)
	srv.SetUnregisterLen(100)
	srv.SetMaxQueueMessages(20)
	srv.SetQueueQos0Messages(false)
	srv.SetDeliveryRetryInterval(25 * time.Second)

	if cap(srv.msgRouter) != 100 {
		t.Fatalf("SetMsgRouterLen() error")
	}
	if cap(srv.register) != 100 {
		t.Fatalf("SetRegisterLen() error")
	}
	if cap(srv.unregister) != 100 {
		t.Fatalf("SetUnregisterLen() error")
	}
	if srv.config.maxInflightMessages != 200 {
		t.Fatalf("SetMaxInflightMessages() error")
	}

	if srv.config.maxQueueMessages != 20 {
		t.Fatalf("SetMaxQueueMessages() error")
	}
	if srv.config.queueQos0Messages != false {
		t.Fatalf("SetQueueQos0Messages() error")
	}
	if srv.config.deliveryRetryInterval != 25*time.Second {
		t.Fatalf("SetDeliveryRetryInterval() error")
	}

}

func TestSubscriptionDb(t *testing.T) {
	db := &subscriptionsDB{
		topicsByName: make(map[string]map[string]packets.Topic),
		topicsByID:   make(map[string]map[string]packets.Topic),
	}
	db.init("cid", "tpname")

	tpname := "tpname"
	topic := packets.Topic{
		Qos:  packets.QOS_0,
		Name: tpname,
	}

	db.add("cid", tpname, topic)
	if tp, ok := db.topicsByID["cid"][tpname]; !ok || tp != topic {
		t.Fatalf("db.add error, topicsByID want %v, got %v", topic, tp)
	}

	if tp, ok := db.topicsByName[tpname]["cid"]; !ok || tp != topic {
		t.Fatalf("db.add error,topicsByName want %v, got %v", topic, tp)
	}
	if !db.exist("cid", tpname) {
		t.Fatalf("db.exist error, want true, got false")
	}

	db.remove("cid", tpname)
	if db.exist("cid", tpname) {
		t.Fatalf("db.exist error, want false, got true")
	}
	if _, ok := db.topicsByID["cid"][tpname]; ok {
		t.Fatalf("db.add error, want false, got true")
	}

	if _, ok := db.topicsByName[tpname]["cid"]; ok {
		t.Fatalf("db.add error, want false, got true")
	}
}

func TestRandUUID(t *testing.T) {
	var uuids map[string]struct{}
	uuids = make(map[string]struct{})
	for i := 0; i < 100; i++ {
		uuids[getRandomUUID()] = struct{}{}
	}
	if len(uuids) != 100 {
		t.Fatalf("duplicated ID")
	}
}
