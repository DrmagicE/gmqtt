package server

import (
	"bytes"
	"container/list"
	"context"
	"errors"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"io"
	"net"
	"os"
	"reflect"
	"testing"
	"time"
)

const test_redelivery_internal = 10 * time.Second

type dummyAddr string

type testListener struct {
	conn        list.List
	acceptReady chan struct{}
}

var srv *Server

func (l *testListener) Accept() (c net.Conn, err error) {
	<-l.acceptReady
	if l.conn.Len() != 0 {
		e := l.conn.Front()
		c = e.Value.(net.Conn)
		err = nil
		l.conn.Remove(e)
	} else {
		c = nil
		err = io.EOF
	}
	return
}

func (l *testListener) Close() error {
	return nil
}

func (l *testListener) Addr() net.Addr {
	return dummyAddr("test-address")
}

func (a dummyAddr) Network() string {
	return string(a)
}

func (a dummyAddr) String() string {
	return string(a)
}

type noopConn struct{}

func (noopConn) LocalAddr() net.Addr { return dummyAddr("local-addr") }

func (noopConn) SetDeadline(t time.Time) error      { return nil }
func (noopConn) SetReadDeadline(t time.Time) error  { return nil }
func (noopConn) SetWriteDeadline(t time.Time) error { return nil }

type rwTestConn struct {
	io.Reader
	io.Writer
	noopConn
	closeFunc func() error // called if non-nil
	closec    chan struct{}
	readChan  chan []byte
	writeChan chan []byte
	netAddr   string
}

func (c *rwTestConn) RemoteAddr() net.Addr {

	if c.netAddr != "" {
		return dummyAddr( c.netAddr)
	} else {
		return dummyAddr("remote-addr")
	}

}

func (c *rwTestConn) Read(p []byte) (int, error) {
	select {
	case <-c.closec:
		return 0, io.EOF
	case b := <-c.readChan:
		l := len(b)
		copy(p, b)
		return l, nil

	}
}

func (c *rwTestConn) Write(p []byte) (int, error) {
	b := make([]byte, len(p))
	copy(b, p)
	select {
	case <-c.closec:
		return 0, io.EOF
	case c.writeChan <- b:
		return len(p), nil
	}
}

func (c *rwTestConn) Close() error {
	if c.closeFunc != nil {
		return c.closeFunc()
	}
	select {
	case <-c.closec:
	default:
		close(c.closec)
	}
	return nil
}

func newTestServer() *Server {
	var s *Server
	if srv != nil {
		s = srv
	} else {
		s = NewServer()
		s.SetDeliveryRetryInterval(test_redelivery_internal)
	}
	//SetLogger(logger.NewLogger(os.Stderr, "", log2.LstdFlags))
	ln := &testListener{acceptReady: make(chan struct{})}
	s.AddTCPListenner(ln)
	return s
}

func defaultConnectPacket() *packets.Connect {
	return &packets.Connect{
		ProtocolLevel: 0x04,
		UsernameFlag:  true,
		Username:      []byte{116, 101, 115, 116, 117, 115, 101, 114},
		ProtocolName:  []byte{77, 81, 84, 84},
		PasswordFlag:  true,
		Password:      []byte{116, 101, 115, 116, 112, 97, 115, 115},
		WillRetain:    false,
		WillFlag:      true,
		WillTopic:     []byte{116, 101, 115, 116},
		WillMsg:       []byte{84, 101, 115, 116, 32, 80, 97, 121, 108, 111, 97, 100},
		WillQos:       packets.QOS_1,
		CleanSession:  true,
		KeepAlive:     30,
		ClientId:      []byte{77, 81, 84, 84},
	}
}

func connectedServer(connect *packets.Connect) (*Server, net.Conn) {
	srv := newTestServer()
	ln := srv.tcpListener[0].(*testListener)
	if connect == nil {
		connect = defaultConnectPacket()
	}
	closec := make(chan struct{})
	conn := &rwTestConn{
		closec:    closec,
		readChan:  make(chan []byte, 1024),
		writeChan: make(chan []byte, 1024),
	}
	ln.conn.PushBack(conn)
	srv.Run()
	ln.acceptReady <- struct{}{}
	writePacket(conn, connect)
	readPacket(conn)
	return srv, conn
}

func connectedServerWith2Client(connect ...*packets.Connect) (*Server, net.Conn, net.Conn) {
	srv := newTestServer()
	ln := srv.tcpListener[0].(*testListener)
	var cc []net.Conn
	cc = make([]net.Conn, 2)

	conn := make([]*packets.Connect, 2)
	for k, v := range connect {
		conn[k] = v
	}
	for i := 0; i < 2; i++ {
		closec := make(chan struct{})
		conn := &rwTestConn{
			closec:    closec,
			readChan:  make(chan []byte, 1024),
			writeChan: make(chan []byte, 1024),
		}
		ln.conn.PushBack(conn)
		cc[i] = conn
	}

	srv.Run()

	ln.acceptReady <- struct{}{}
	var conn1, conn2 *packets.Connect
	if conn[0] == nil {
		conn1 = defaultConnectPacket()
		conn1.ClientId = []byte("id1")
	} else {
		conn1 = conn[0]
	}

	if conn[1] == nil {
		conn2 = defaultConnectPacket()
		conn2.ClientId = []byte("id2")
	} else {
		conn2 = conn[1]
	}

	writePacket(cc[0].(*rwTestConn), conn1)
	readPacket(cc[0].(*rwTestConn))
	ln.acceptReady <- struct{}{}
	writePacket(cc[1].(*rwTestConn), conn2)
	readPacket(cc[1].(*rwTestConn))
	return srv, cc[0], cc[1]
}

func TestConnect(t *testing.T) {
	srv := newTestServer()
	defer srv.Stop(context.Background())
	ln := srv.tcpListener[0].(*testListener)

	closec := make(chan struct{})
	conn := &rwTestConn{
		closec:    closec,
		readChan:  make(chan []byte),
		writeChan: make(chan []byte),
	}
	ln.conn.PushBack(conn)
	srv.Run()
	ln.acceptReady <- struct{}{}
	writePacket(conn, defaultConnectPacket())
	packet, err := readPacket(conn)
	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}
	if p, ok := packet.(*packets.Connack); ok {
		if p.SessionPresent != 0 {
			t.Fatalf("SessionPresent error,want 0, got %d", p.SessionPresent)
		}
		if p.Code != packets.CODE_ACCEPTED {
			t.Fatalf("SessionPresent error,want %d, got %d", packets.CODE_ACCEPTED, p.Code)
		}
	} else {
		t.Fatalf("unexpected Packet Type, want %v, got %v", reflect.TypeOf(&packets.Connack{}), packet)
	}
	/*	if se, ok := srv.sessions["MQTT"]; ok {
			opts := se.client.opts

			usernameWant := string([]byte{116, 101, 115, 116, 117, 115, 101, 114})
			if opts.Username != usernameWant {
				t.Fatalf("Username error,want %s, got %s", usernameWant, opts.Username)
			}
			passwordWant := string([]byte{116, 101, 115, 116, 112, 97, 115, 115})
			if opts.Password != passwordWant {
				t.Fatalf("Password error,want %s, got %s", passwordWant, opts.Password)
			}

			if opts.CleanSession != true {
				t.Fatalf("CleanSession error,want true, got %v", opts.CleanSession)
			}

			if opts.ClientId != "MQTT" {
				t.Fatalf("ClientId error,want MQTT, got %s", opts.ClientId)
			}

			if opts.KeepAlive != 30 {
				t.Fatalf("KeepAlive error,want 30, got %d", opts.KeepAlive)
			}

			if opts.WillRetain != false {
				t.Fatalf("WillRetain error,want false, got %v", opts.WillRetain)
			}

			willPayloadWant := []byte{84, 101, 115, 116, 32, 80, 97, 121, 108, 111, 97, 100}
			if !bytes.Equal(opts.WillPayload, willPayloadWant) {
				t.Fatalf("WillPayload error,want %v, got %v", willPayloadWant, opts.WillPayload)
			}

			willTopicWant := string([]byte{116, 101, 115, 116})
			if opts.WillTopic != willTopicWant {
				t.Fatalf("WillTopic error,want %s, got %s", willTopicWant, opts.WillTopic)
			}
			if opts.WillQos != 1 {
				t.Fatalf("WillQos error,want 1, got %d", opts.WillQos)
			}
			if opts.WillFlag != true {
				t.Fatalf("WillFlag error,want true, got %t", opts.WillFlag)
			}
		} else {
			t.Fatalf("session not found")
		}
	*/
	select {
	case <-closec:
		t.Fatalf("unexpected close")
	default:

	}
	//send connect packet again
	writePacket(conn, defaultConnectPacket())
	select {
	case <-closec:
	case <-time.After(1 * time.Second):
		t.Fatalf("conn should be closed")
	}
}

func TestDisconnect(t *testing.T) {
	srv, conn := connectedServer(nil)
	defer srv.Stop(context.Background())
	c := conn.(*rwTestConn)
	disconnect := &packets.Disconnect{}
	err := writePacket(c, disconnect)
	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}
	select {
	case <-c.closec:
	case <-time.After(1 * time.Second):
		t.Fatalf("disconnect error")
	}
}

func TestQos0Publish(t *testing.T) {
	srv, conn := connectedServer(nil)
	defer srv.Stop(context.Background())
	c := conn.(*rwTestConn)
	pub := &packets.Publish{
		Dup:       false,
		Qos:       0,
		Retain:    false,
		TopicName: []byte("topic name"),
		PacketId:  10,
		Payload:   []byte("payload"),
	}
	err := writePacket(c, pub)

	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}
	select {
	case <-c.writeChan:
		t.Fatalf("unexpected write")
	case <-time.After(1 * time.Second):
	}
}

func TestQos1Publish(t *testing.T) {
	srv, conn := connectedServer(nil)
	defer srv.Stop(context.Background())
	c := conn.(*rwTestConn)
	pub := &packets.Publish{
		Dup:       false,
		Qos:       1,
		Retain:    false,
		TopicName: []byte("topic name"),
		PacketId:  10,
		Payload:   []byte("payload"),
	}
	err := writePacket(c, pub)
	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}
	packet, err := readPacket(c)
	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}
	if p, ok := packet.(*packets.Puback); ok {
		if p.PacketId != pub.PacketId {
			t.Fatalf("PacketId error, want %d, got %d", pub.PacketId, p.PacketId)
		}
	} else {
		t.Fatalf("unexpected Packet Type, want %v, got %v", reflect.TypeOf(&packets.Puback{}), reflect.TypeOf(packet))
	}

}

func TestQos2Publish(t *testing.T) {
	srv, conn := connectedServer(nil)
	defer srv.Stop(context.Background())
	c := conn.(*rwTestConn)
	var pid packets.PacketId
	pid = 10
	for i := 0; i < 2; i++ { //发送两次相同的packet id
		pub := &packets.Publish{
			Dup:       false,
			Qos:       2,
			Retain:    false,
			TopicName: []byte("topic name"),
			PacketId:  pid,
			Payload:   []byte("payload"),
		}
		err := writePacket(c, pub)
		if err != nil {
			t.Fatalf("unexpected error:%s", err)
		}
		packet, err := readPacket(c)
		if err != nil {
			t.Fatalf("unexpected error:%s", err)
		}
		if p, ok := packet.(*packets.Pubrec); ok {
			if p.PacketId != pub.PacketId {
				t.Fatalf("PacketId error, want %d, got %d", pub.PacketId, p.PacketId)
			}
		} else {
			t.Fatalf("unexpected Packet Type, want %v, got %v", reflect.TypeOf(&packets.Pubrec{}), reflect.TypeOf(packet))
		}
	}

	for i := 0; i < 2; i++ { //发送两次相同的packet id
		pubrel := &packets.Pubrel{
			PacketId: 10,
		}
		err := writePacket(c, pubrel)
		if err != nil {
			t.Fatalf("unexpected error:%s", err)
		}
		packet, err := readPacket(c)
		if err != nil {
			t.Fatalf("unexpected error:%s", err)
		}
		if p, ok := packet.(*packets.Pubcomp); ok {
			if p.PacketId != pid {
				t.Fatalf("PacketId error, want %d, got %d", pid, p.PacketId)
			}
		} else {
			t.Fatalf("unexpected Packet Type, want %v, got %v", reflect.TypeOf(&packets.Pubcomp{}), reflect.TypeOf(packet))
		}

	}
}

func readPacket(c *rwTestConn) (packets.Packet, error) {
	select {
	case <-c.closec:
		return nil, io.EOF
	case b := <-c.writeChan:
		return packets.NewReader(bytes.NewBuffer(b)).ReadPacket()
	}

}

var testErrReadTimeout = errors.New("reade timeout")

func readPacketWithTimeOut(c *rwTestConn, timeout time.Duration) (packets.Packet, error) {
	select {
	case <-c.closec:
		return nil, io.EOF
	case <-time.After(timeout):
		return nil, testErrReadTimeout
	case b := <-c.writeChan:
		return packets.NewReader(bytes.NewBuffer(b)).ReadPacket()
	}

}

func writePacket(c *rwTestConn, packet packets.Packet) error {
	b := &bytes.Buffer{}
	err := packets.NewWriter(b).WriteAndFlush(packet)
	if err != nil {
		return err
	}
	c.readChan <- b.Bytes()
	return nil

}

func TestSubScribe(t *testing.T) {
	srv, conn := connectedServer(nil)
	defer srv.Stop(context.Background())
	c := conn.(*rwTestConn)
	sub := &packets.Subscribe{
		PacketId: 10,
		Topics: []packets.Topic{
			{Name: "/a/b/c", Qos: packets.QOS_0},
			{Name: "/a/b/+", Qos: packets.QOS_1},
		},
	}
	err := writePacket(c, sub)
	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}
	packet, err := readPacket(c)
	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}
	if p, ok := packet.(*packets.Suback); ok {
		if p.PacketId != sub.PacketId {
			t.Fatalf("PacketId error, want %d, got %d", sub.PacketId, p.PacketId)
		}
		if !bytes.Equal(p.Payload, []byte{0, 1}) {
			t.Fatalf("Payload error, want %v, got %v", []byte{0, 1}, p.Payload)
		}
	} else {
		t.Fatalf("unexpected Packet Type, want %v, got %v", reflect.TypeOf(&packets.Suback{}), reflect.TypeOf(packet))
	}
	var pubpid uint16
	pubpid = 10
	pub := &packets.Publish{
		Dup:       false,
		Qos:       packets.QOS_0,
		Retain:    false,
		TopicName: []byte("/a/b/cc"),
		PacketId:  pubpid,
		Payload:   []byte("payload"),
	}
	err = writePacket(c, pub)
	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}
	packet, err = readPacket(c)
	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}
	if p, ok := packet.(*packets.Publish); ok {
		if p.Dup != false {
			t.Fatalf("Dup error, want false,got %t", p.Dup)
		}
		if p.Qos != packets.QOS_0 {
			t.Fatalf("Qos error, want %d, got %d", packets.QOS_0, p.Qos)
		}
		if !bytes.Equal(p.Payload, pub.Payload) {
			t.Fatalf("Payload error, want %v, got %v", pub.Payload, p.Payload)
		}
		if p.Retain {
			t.Fatalf("Retain error, want false,got %t", p.Retain)
		}
	} else {
		t.Fatalf("unexpected Packet Type, want %v, got %v", reflect.TypeOf(&packets.Publish{}), reflect.TypeOf(packet))
	}

}

func TestUnsubscribe(t *testing.T) {
	srv, conn := connectedServer(nil)
	defer srv.Stop(context.Background())
	c := conn.(*rwTestConn)
	sub := &packets.Subscribe{
		PacketId: 10,
		Topics: []packets.Topic{
			{Name: "/a/b/c", Qos: packets.QOS_0},
			{Name: "/a/b/+", Qos: packets.QOS_1},
		},
	}
	err := writePacket(c, sub)
	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}
	readPacket(c) //suback

	unsub := &packets.Unsubscribe{
		PacketId: 11,
		Topics:   []string{"/a/b/+"},
	}
	err = writePacket(c, unsub)
	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}
	p, err := readPacket(c) //
	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}
	if unsuback, ok := p.(*packets.Unsuback); ok {
		if unsuback.PacketId != unsub.PacketId {
			t.Fatalf("PacketId error, want %d, got %d", sub.PacketId, unsuback.PacketId)
		}
	} else {
		t.Fatalf("unexpected Packet Type, want %v, got %v", reflect.TypeOf(&packets.Unsuback{}), reflect.TypeOf(p))
	}

	srv.mu.RLock()
	client := srv.clients["MQTT"]
	se := client.session
	srv.mu.RUnlock()
	se.topicsMu.Lock()
	if _, ok := se.subTopics["/a/b/+"]; ok {
		t.Fatalf("subTopics error,the topic dose not delete from map")
	}
	se.topicsMu.Unlock()

	pub := &packets.Publish{
		Dup:       false,
		Qos:       packets.QOS_0,
		Retain:    false,
		TopicName: []byte("/a/b/cc"),
		PacketId:  11,
		Payload:   []byte("payload"),
	}
	err = writePacket(c, pub)
	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}
	p, err = readPacketWithTimeOut(c, 1*time.Second)
	if err == nil {
		t.Fatalf("delivering message to unsubscribed topic:%v", reflect.TypeOf(p))
	}
}

func TestOnSubscribe(t *testing.T) {
	srv, conn := connectedServer(nil)
	defer srv.Stop(context.Background())
	c := conn.(*rwTestConn)
	srv.OnSubscribe = func(client *Client, topic packets.Topic) uint8 {
		if topic.Qos == packets.QOS_0 {
			return packets.QOS_1
		}
		if topic.Name == "/a/b/+" {
			return packets.SUBSCRIBE_FAILURE
		}
		return topic.Qos
	}
	sub := &packets.Subscribe{
		PacketId: 10,
		Topics: []packets.Topic{
			{Name: "/a/b/c", Qos: packets.QOS_0},
			{Name: "/a/b/+", Qos: packets.QOS_1},
		},
	}
	err := writePacket(c, sub)
	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}
	packet, err := readPacket(c)
	if p, ok := packet.(*packets.Suback); ok {
		if p.PacketId != sub.PacketId {
			t.Fatalf("PacketId error, want %d, got %d", sub.PacketId, p.PacketId)
		}
		if !bytes.Equal(p.Payload, []byte{packets.QOS_1, packets.SUBSCRIBE_FAILURE}) {
			t.Fatalf("Payload error, want %v, got %v", []byte{packets.QOS_1, packets.SUBSCRIBE_FAILURE}, p.Payload)
		}
		srv.mu.RLock()
		srv.clients["MQTT"].session.topicsMu.Lock()
		if len(srv.clients["MQTT"].session.subTopics) != 1 {
			t.Fatalf("len(subTopics) error, want 1, got %d", len(srv.clients["MQTT"].session.subTopics))
		}
		srv.clients["MQTT"].session.topicsMu.Unlock()
		srv.mu.RUnlock()
	} else {
		t.Fatalf("unexpected Packet Type, want %v, got %v", reflect.TypeOf(&packets.Suback{}), reflect.TypeOf(packet))
	}

}

func TestRetainMsg(t *testing.T) {
	srv, conn := connectedServer(nil)
	defer srv.Stop(context.Background())
	var err error
	c := conn.(*rwTestConn)
	topicName := []byte("a/b")
	payload := []byte("Payload")
	pub := &packets.Publish{
		Dup:       true,
		Qos:       packets.QOS_1,
		Retain:    true,
		TopicName: topicName,
		PacketId:  10,
		Payload:   payload,
	}
	err = writePacket(c, pub)
	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}
	readPacket(c) //read puback
	sub := &packets.Subscribe{
		PacketId: 11,
		Topics: []packets.Topic{
			{Name: string(topicName), Qos: packets.QOS_2},
		},
	}
	err = writePacket(c, sub)
	srv.retainedMsgMu.Lock()
	if retain, ok := srv.retainedMsg["a/b"]; ok {
		if retain.Qos != pub.Qos {
			t.Fatalf("Qos error, want %d, got %d", pub.Qos, retain.Qos)
		}
		if !bytes.Equal(retain.TopicName, pub.TopicName) {
			t.Fatalf("TopicName error, want %v, got %v", pub.TopicName, retain.TopicName)
		}
		if !bytes.Equal(retain.Payload, pub.Payload) {
			t.Fatalf("Payload error, want %v, got %v", pub.Payload, retain.Payload)
		}
	} else {
		t.Fatalf("retained Msg error")
	}
	srv.retainedMsgMu.Unlock()
	var pp []packets.Packet
	for i := 0; i < 2; i++ { //read suback & publish
		p, err := readPacket(c)
		if err != nil {
			t.Fatalf("unexpected error:%s", err)
		}
		pp = append(pp, p)
	}
	for _, v := range pp {
		switch v.(type) {
		case *packets.Suback:

		case *packets.Publish:
			pub := v.(*packets.Publish)

			if !bytes.Equal(pub.TopicName, topicName) {
				t.Fatalf("TopicName error, want %v, got %v", topicName, pub.TopicName)
			}
			if pub.Dup != false {
				t.Fatalf("Dup error, want %t, got %t", false, true)
			}
			if pub.Qos != packets.QOS_1 {
				t.Fatalf("Qos error, want %d, got %d", packets.QOS_1, pub.Qos)
			}

			if !pub.Retain {
				t.Fatalf("Retain error, want %t, got %t", true, false)
			}
		default:
			t.Fatalf("unexpected type:%v", reflect.TypeOf(v))
		}
	}
	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}
	pub0 := &packets.Publish{
		Dup:       false,
		Qos:       packets.QOS_0,
		Retain:    true,
		TopicName: topicName,
		PacketId:  10,
		Payload:   payload,
	}
	err = writePacket(c, pub0)
	packet, err := readPacket(c) //publish
	if p, ok := packet.(*packets.Publish); ok {
		if !bytes.Equal(p.TopicName, topicName) {
			t.Fatalf("TopicName error, want %v, got %v", topicName, p.TopicName)
		}
		if p.Dup != false {
			t.Fatalf("Dup error, want %t, got %t", false, true)
		}
		if p.Qos != packets.QOS_0 {
			t.Fatalf("Qos error, want %d, got %d", packets.QOS_0, p.Qos)
		}

		if p.Retain {
			t.Fatalf("Retain error, want %t, got %t", false, true)
		}
	} else {
		t.Fatalf("unexpected Packet Type, want %v, got %v", reflect.TypeOf(&packets.Publish{}), reflect.TypeOf(packet))
	}

}

func TestPingPong(t *testing.T) {
	srv, conn := connectedServer(nil)
	defer srv.Stop(context.Background())
	c := conn.(*rwTestConn)
	ping := &packets.Pingreq{}
	err := writePacket(c, ping)
	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}
	packet, err := readPacket(c)
	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}
	if _, ok := packet.(*packets.Pingresp); !ok {
		t.Fatalf("unexpected Packet Type, want %v, got %v", reflect.TypeOf(&packets.Pingresp{}), reflect.TypeOf(packet))
	}
}

func TestQos1Redelivery(t *testing.T) {
	srv, conn := connectedServer(nil)
	defer srv.Stop(context.Background())
	c := conn.(*rwTestConn)
	topicName := []byte("a/b")
	payload := []byte("payload")
	sub := &packets.Subscribe{
		PacketId: 10,
		Topics: []packets.Topic{
			{Name: "a/b", Qos: packets.QOS_2},
		},
	}
	err := writePacket(c, sub)
	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}
	readPacket(c) //suback
	//test Qos1

	pub1 := &packets.Publish{
		Dup:       false,
		Qos:       packets.QOS_1,
		Retain:    false,
		TopicName: topicName,
		PacketId:  11,
		Payload:   payload,
	}
	err = writePacket(c, pub1)
	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}
	var originalPid uint16
	for i := 0; i < 2; i++ { //read puback & publish
		p, err := readPacket(c)
		if err != nil {
			t.Fatalf("unexpected error:%s", err)
		}
		if pub, ok := p.(*packets.Publish); ok {
			originalPid = pub.PacketId
		}
	}
	p, err := readPacketWithTimeOut(c, test_redelivery_internal+ 1 * time.Second)
	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}
	if pub, ok := p.(*packets.Publish); ok {
		if pub.Dup != true {
			t.Fatalf("Dup error, want %t, got %t", true, false)
		}
		if !bytes.Equal(pub.TopicName, topicName) {
			t.Fatalf("TopicName error, want %v, got %v", topicName, pub.TopicName)
		}
		if !bytes.Equal(pub.Payload, payload) {
			t.Fatalf("Payload error, want %v, got %v", payload, pub.Payload)
		}
		if pub.Qos != packets.QOS_1 {
			t.Fatalf("Qos error, want %d, got %d", packets.QOS_1, pub.Qos)
		}
		if pub.PacketId != originalPid {
			t.Fatalf("PacketId error, want %d, got %d", originalPid, pub.PacketId)
		}
	}
	puback := p.(*packets.Publish).NewPuback()
	err = writePacket(c, puback)
	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}

}

func TestQos2Redelivery(t *testing.T) {
	srv, s, r := connectedServerWith2Client()
	defer srv.Stop(context.Background())
	sender := s.(*rwTestConn)
	reciver := r.(*rwTestConn)
	topicName := []byte("b/c")
	payload := []byte("payload")
	var err error
	//test Qos2
	var senderPid uint16
	sub := &packets.Subscribe{
		Topics: []packets.Topic{
			{Name: string(topicName), Qos: packets.QOS_2},
		},
	}
	err = writePacket(reciver, sub)
	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}
	readPacket(reciver) //suback
	senderPid = 10
	pub2 := &packets.Publish{
		Dup:       false,
		Qos:       packets.QOS_2,
		Retain:    false,
		TopicName: topicName,
		PacketId:  senderPid,
		Payload:   payload,
	}
	err = writePacket(sender, pub2)
	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}
	p, err := readPacket(sender) //pubrec
	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}
	if pubrec, ok := p.(*packets.Pubrec); ok {
		p, err := readPacket(reciver)
		if err != nil {
			t.Fatalf("unexpected error:%s", err)
		}
		pub := p.(*packets.Publish)
		if pub.Qos != packets.QOS_2 {
			if pub.Dup != false {
				t.Fatalf("Dup error, want %t, got %t", false, true)
			}
			if !bytes.Equal(pub.TopicName, topicName) {
				t.Fatalf("TopicName error, want %v, got %v", topicName, pub.TopicName)
			}
			if !bytes.Equal(pub.Payload, payload) {
				t.Fatalf("Payload error, want %v, got %v", payload, pub.Payload)
			}
			if pub.Qos != packets.QOS_2 {
				t.Fatalf("Qos error, want %d, got %d", packets.QOS_2, pub.Qos)
			}
		}
		err = writePacket(reciver, pub.NewPubrec())
		readPacket(reciver) //pubrel
		if err != nil {
			t.Fatalf("unexpected error:%s", err)
		}
		err = writePacket(sender, pub2) //再发一次相同的publish包
		if err != nil {
			t.Fatalf("unexpected error:%s", err)
		}
		p, err = readPacket(sender) //pubrec
		if err != nil {
			t.Fatalf("unexpected error:%s", err)
		}
		pubrec2 := p.(*packets.Pubrec)
		if pubrec2.PacketId != pubrec.PacketId {
			t.Fatalf("PacketId error, want %d, got %d", pubrec.PacketId, pubrec2.PacketId)
		}
		p, err = readPacketWithTimeOut(reciver, 1*time.Second)
		if err != testErrReadTimeout {
			t.Fatalf("delivery duplicated messages， %v", reflect.TypeOf(p))
		}
		err = writePacket(sender, pubrec.NewPubrel())
		if err != nil {
			t.Fatalf("unexpected error:%s", err)
		}
		p, err = readPacket(sender) //pubcomp
		if _, ok := p.(*packets.Pubcomp); !ok {
			t.Fatalf("unexpected Packet Type, want %v, got %v", reflect.TypeOf(&packets.Pubcomp{}), reflect.TypeOf(p))
		}
	} else {
		t.Fatalf("unexpected Packet Type, want %v, got %v", reflect.TypeOf(&packets.Pubrec{}), reflect.TypeOf(pubrec))
	}
	p, err = readPacket(reciver) //pubrel
	if _, ok := p.(*packets.Pubrel); !ok {
		t.Fatalf("unexpected Packet Type, want %v, got %v", reflect.TypeOf(&packets.Pubrel{}), reflect.TypeOf(p))
	}
	pubrel1 := p.(*packets.Pubrel)

	p, err = readPacketWithTimeOut(reciver, (REDELIVER_TIME+1)*time.Second) //redelivery pubrel
	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}
	if pubrel2, ok := p.(*packets.Pubrel); ok {
		if pubrel1.PacketId != pubrel2.PacketId {
			t.Fatalf("PacketId error, want %d, got %d", pubrel1.PacketId, pubrel2.PacketId)
		}
	} else {
		t.Fatalf("unexpected Packet Type, want %v, got %v", reflect.TypeOf(&packets.Pubrel{}), reflect.TypeOf(p))
	}
}

func TestRedeliveryOnReconnect(t *testing.T) {
	connect := defaultConnectPacket()
	connect.CleanSession = false
	srv, conn := connectedServer(connect)
	defer srv.Stop(context.Background())
	var err error
	c := conn.(*rwTestConn)
	//ln := srv.tcpListener[0].(*testListener)
	sub := &packets.Subscribe{
		PacketId: 10,
		Topics: []packets.Topic{
			{Name: string("#"), Qos: packets.QOS_1},
		},
	}
	err = writePacket(c, sub)
	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}
	readPacket(c) //suback
	pub := &packets.Publish{
		Dup:       false,
		Qos:       1,
		Retain:    false,
		TopicName: []byte("test"),
		PacketId:  10,
		Payload:   []byte("payload"),
	}
	err = writePacket(c, pub)
	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}
	var originalPid uint16
	for i := 0; i < 2; i++ { //read puback & publish
		p, err := readPacket(c)
		if err != nil {
			t.Fatalf("unexpected error:%s", err)
		}
		if pub, ok := p.(*packets.Publish); ok {
			originalPid = pub.PacketId
		}
	}
	c.Close()
	//reconnect
	reConn := &rwTestConn{
		closec:    make(chan struct{}),
		readChan:  make(chan []byte, 1024),
		writeChan: make(chan []byte, 1024),
	}
	srv.tcpListener[0].(*testListener).conn.PushBack(reConn)
	srv.tcpListener[0].(*testListener).acceptReady <- struct{}{}
	err = writePacket(reConn, connect)
	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}
	readPacket(reConn)           //connack
	p, err := readPacket(reConn) //redelivery publish
	if pub, ok := p.(*packets.Publish); ok {
		if !bytes.Equal([]byte("test"), pub.TopicName) {
			t.Fatalf("TopicName error, want %v, got %v", []byte("test"), pub.TopicName)
		}
		if !bytes.Equal([]byte("payload"), pub.Payload) {
			t.Fatalf("Payload error, want %v, got %v", []byte("payload"), pub.Payload)
		}
		if pub.PacketId != originalPid {
			t.Fatalf("PacketId error, want %d, got %d", originalPid, pub.PacketId)
		}
		if pub.Dup != true {
			t.Fatalf("Dup error, want %t, got %t", true, false)
		}
	} else {
		t.Fatalf("unexpected Packet Type, want %v, got %v", reflect.TypeOf(&packets.Publish{}), reflect.TypeOf(p))
	}

}

func TestOfflineMessageQueueing(t *testing.T) {
	srv = NewServer()
	srv.SetMaxOfflineMsg(3)
	srv.Store = &FileStore{Path:"testdata/clientTest"}
	defer func() {
		srv = nil
		os.RemoveAll("testdata/clientTest")
	}()

	conn1 := defaultConnectPacket()
	conn1.CleanSession = false
	conn1.ClientId = []byte("id1")

	conn2 := defaultConnectPacket()
	conn2.CleanSession = false
	conn2.ClientId = []byte("id2")
	srv, s, r := connectedServerWith2Client(conn1, conn2)
	defer srv.Stop(context.Background())
	var err error
	sender := s.(*rwTestConn)
	reciver := r.(*rwTestConn)
	sub := &packets.Subscribe{
		PacketId: 10,
		Topics: []packets.Topic{
			{Name: string("#"), Qos: packets.QOS_1},
		},
	}
	err = writePacket(reciver, sub)
	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}
	readPacket(reciver) //suback
	disconnect := &packets.Disconnect{}
	writePacket(reciver, disconnect)

	readPacket(reciver) //close()

	for i := 0x31; i <= 0x35; i++ { //assic 1 to 5
		pub := &packets.Publish{
			Dup:       false,
			Qos:       packets.QOS_1,
			Retain:    false,
			TopicName: []byte{byte(i)},
			PacketId:  uint16(i),
			Payload:   []byte{byte(i), byte(i)},
		}
		err = writePacket(sender, pub)
		if err != nil {
			t.Fatalf("unexpected error:%s", err)
		}
	}
	time.Sleep(2 * time.Second)
	reConn := &rwTestConn{
		closec:    make(chan struct{}),
		readChan:  make(chan []byte, 1024),
		writeChan: make(chan []byte, 1024),
		netAddr:"reciver",
	}
	srv.tcpListener[0].(*testListener).conn.PushBack(reConn)
	srv.tcpListener[0].(*testListener).acceptReady <- struct{}{}
	err = writePacket(reConn, conn2)
	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}
	readPacket(reConn) //connack

	for i := 0x31; i <= 0x35; i++ { //assic 1 to 5
		p, err := readPacket(reConn)
		if err != nil {
			t.Fatalf("unexpected error:%s", err)
		}
		if pub, ok := p.(*packets.Publish); ok {
			if !bytes.Equal([]byte{byte(i), byte(i)}, pub.Payload) {
				t.Fatalf("[%x]Payload error, want %v, got %v %s", i, []byte{byte(i), byte(i)}, pub.Payload,string(pub.Payload))
			}
			if !bytes.Equal([]byte{byte(i)}, pub.TopicName) {
				t.Fatalf("[%x]TopicName error, want %v, got %v", i, []byte{byte(i)}, pub.TopicName)
			}
			if pub.Dup != false {
				t.Fatalf("[%x]Dup error, want %t, got %t", i, false, true)
			}
			if pub.Qos != packets.QOS_1 {
				t.Fatalf("[%x]Qos error, want %d, got %d", i, packets.QOS_1, pub.Qos)
			}
		} else {
			t.Fatalf("unexpected Packet Type, want %v, got %v", reflect.TypeOf(&packets.Publish{}), reflect.TypeOf(p))
		}
	}

}

func TestWillMsg(t *testing.T) {
	srv, s, r := connectedServerWith2Client()
	defer srv.Stop(context.Background())
	var err error
	sender := s.(*rwTestConn)
	reciver := r.(*rwTestConn)
	sub := &packets.Subscribe{
		PacketId: 10,
		Topics: []packets.Topic{
			{Name: "#", Qos: packets.QOS_1},
		},
	}
	err = writePacket(reciver, sub)
	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}
	readPacket(reciver) //suback
	sender.Close()      //
	p, err := readPacketWithTimeOut(reciver, 1*time.Second)
	if err != nil {
		t.Fatalf("missing Will Message, %s", err)
	}
	connect := defaultConnectPacket()
	if pub, ok := p.(*packets.Publish); ok {
		if !bytes.Equal(connect.WillMsg, pub.Payload) {
			t.Fatalf("Payload error, want %v, got %v", connect.WillMsg, pub.Payload)
		}
		if !bytes.Equal(connect.WillTopic, pub.TopicName) {
			t.Fatalf("TopicName error, want %v, got %v", connect.WillTopic, pub.TopicName)
		}
	} else {
		t.Fatalf("unexpected Packet Type, want %v, got %v", reflect.TypeOf(&packets.Publish{}), reflect.TypeOf(p))
	}
}

func TestRemoveWillMsg(t *testing.T) {
	srv, s, r := connectedServerWith2Client()
	defer srv.Stop(context.Background())
	var err error
	sender := s.(*rwTestConn)
	reciver := r.(*rwTestConn)
	sub := &packets.Subscribe{
		PacketId: 10,
		Topics: []packets.Topic{
			{Name: "topicname", Qos: packets.QOS_1},
		},
	}
	err = writePacket(reciver, sub)
	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}
	readPacket(reciver)                        //suback
	writePacket(sender, &packets.Disconnect{}) //remove will msg
	sender.Close()                             //
	_, err = readPacketWithTimeOut(reciver, 1*time.Second)
	if err == nil {
		t.Fatalf("delivering removed will message")
	}
}
