package gmqtt

import (
	"bytes"
	"container/list"
	"context"
	"errors"
	"io"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/DrmagicE/gmqtt/pkg/packets"
)

const testRedeliveryInternal = 10 * time.Second

type dummyAddr string

type testListener struct {
	conn        list.List
	acceptReady chan struct{}
}

var srv *server

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
		return dummyAddr(c.netAddr)
	}
	return dummyAddr("remote-addr")
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

func newTestServer() *server {
	var s *server
	if srv != nil {
		s = srv
	} else {
		s = NewServer(WithLogger(zap.NewNop()))
		s.config.RetryInterval = testRedeliveryInternal
		s.config.RetryCheckInterval = testRedeliveryInternal
	}
	ln := &testListener{acceptReady: make(chan struct{})}
	s.tcpListener = append(s.tcpListener, ln)
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
		ClientID:      []byte{77, 81, 84, 84}, //MQTT
	}
}

func doconnect(srv *server, connect *packets.Connect) net.Conn {
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
	return conn
}

func connectedServer(connect *packets.Connect) (*server, net.Conn) {
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

func connectedServerWith2Client(connect ...*packets.Connect) (*server, net.Conn, net.Conn) {
	srv := newTestServer()
	ln := srv.tcpListener[0].(*testListener)
	cc := make([]net.Conn, 2)

	conn := make([]*packets.Connect, 2)
	copy(conn, connect)
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
		conn1.ClientID = []byte("id1")
	} else {
		conn1 = conn[0]
	}

	if conn[1] == nil {
		conn2 = defaultConnectPacket()
		conn2.ClientID = []byte("id2")
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
		if p.Code != packets.CodeAccepted {
			t.Fatalf("SessionPresent error,want %d, got %d", packets.CodeAccepted, p.Code)
		}
	} else {
		t.Fatalf("unexpected Packet Type, want %v, got %v", reflect.TypeOf(&packets.Connack{}), packet)
	}
	if se := srv.Client("MQTT").(*client); se != nil {
		if !se.IsConnected() {
			t.Fatalf("IsConnected() error, want true, got false")
		}
		opts := se.opts
		usernameWant := string([]byte{116, 101, 115, 116, 117, 115, 101, 114})
		if opts.username != usernameWant {
			t.Fatalf("Username error,want %s, got %s", usernameWant, opts.username)
		}
		passwordWant := string([]byte{116, 101, 115, 116, 112, 97, 115, 115})
		if opts.password != passwordWant {
			t.Fatalf("Password error,want %s, got %s", passwordWant, opts.password)
		}

		if opts.cleanSession != true {
			t.Fatalf("CleanSession error,want true, got %v", opts.cleanSession)
		}

		if opts.clientID != "MQTT" {
			t.Fatalf("ClientID error,want MQTT, got %s", opts.clientID)
		}

		if opts.keepAlive != 30 {
			t.Fatalf("KeepAlive error,want 30, got %d", opts.keepAlive)
		}

		if opts.willRetain != false {
			t.Fatalf("WillRetain error,want false, got %v", opts.willRetain)
		}

		willPayloadWant := []byte{84, 101, 115, 116, 32, 80, 97, 121, 108, 111, 97, 100}
		if !bytes.Equal(opts.willPayload, willPayloadWant) {
			t.Fatalf("WillPayload error,want %v, got %v", willPayloadWant, opts.willPayload)
		}

		willTopicWant := string([]byte{116, 101, 115, 116})
		if opts.willTopic != willTopicWant {
			t.Fatalf("WillTopic error,want %s, got %s", willTopicWant, opts.willTopic)
		}
		if opts.willQos != 1 {
			t.Fatalf("WillQos error,want 1, got %d", opts.willQos)
		}
		if opts.willFlag != true {
			t.Fatalf("WillFlag error,want true, got %t", opts.willFlag)
		}
	} else {
		t.Fatalf("session not found")
	}

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
		PacketID:  10,
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
		PacketID:  10,
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
		if p.PacketID != pub.PacketID {
			t.Fatalf("PacketID error, want %d, got %d", pub.PacketID, p.PacketID)
		}
	} else {
		t.Fatalf("unexpected Packet Type, want %v, got %v", reflect.TypeOf(&packets.Puback{}), reflect.TypeOf(packet))
	}

}

func TestQos2Publish(t *testing.T) {
	srv, conn := connectedServer(nil)
	defer srv.Stop(context.Background())
	c := conn.(*rwTestConn)
	pid := packets.PacketID(10)
	for i := 0; i < 2; i++ { //发送两次相同的packet id
		pub := &packets.Publish{
			Dup:       false,
			Qos:       2,
			Retain:    false,
			TopicName: []byte("topic name"),
			PacketID:  pid,
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
			if p.PacketID != pub.PacketID {
				t.Fatalf("PacketID error, want %d, got %d", pub.PacketID, p.PacketID)
			}
		} else {
			t.Fatalf("unexpected Packet Type, want %v, got %v", reflect.TypeOf(&packets.Pubrec{}), reflect.TypeOf(packet))
		}
	}

	for i := 0; i < 2; i++ { //发送两次相同的packet id
		pubrel := &packets.Pubrel{
			PacketID: 10,
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
			if p.PacketID != pid {
				t.Fatalf("PacketID error, want %d, got %d", pid, p.PacketID)
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

var errTestReadTimeout = errors.New("reade timeout")

func readPacketWithTimeOut(c *rwTestConn, timeout time.Duration) (packets.Packet, error) {
	select {
	case <-c.closec:
		return nil, io.EOF
	case <-time.After(timeout):
		return nil, errTestReadTimeout
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
		PacketID: 10,
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
		if p.PacketID != sub.PacketID {
			t.Fatalf("PacketID error, want %d, got %d", sub.PacketID, p.PacketID)
		}
		if !bytes.Equal(p.Payload, []byte{0, 1}) {
			t.Fatalf("Payload error, want %v, got %v", []byte{0, 1}, p.Payload)
		}
	} else {
		t.Fatalf("unexpected Packet Type, want %v, got %v", reflect.TypeOf(&packets.Suback{}), reflect.TypeOf(packet))
	}
	pub := &packets.Publish{
		Dup:       false,
		Qos:       packets.QOS_0,
		Retain:    false,
		TopicName: []byte("/a/b/cc"),
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

func TestServer_Subscribe_UnSubscribe(t *testing.T) {
	srv, conn := connectedServer(nil)
	defer srv.Stop(context.Background())
	var err error
	c := conn.(*rwTestConn)
	tt := []packets.Topic{
		{Qos: packets.QOS_0, Name: "t0"},
		{Qos: packets.QOS_1, Name: "t1"},
		{Qos: packets.QOS_2, Name: "t2"},
	}
	srv.subscriptionsDB.Subscribe("MQTT", tt...)
	pub := &packets.Publish{
		Dup:       false,
		Qos:       packets.QOS_0,
		Retain:    false,
		TopicName: []byte("t0"),
		Payload:   []byte("payload"),
	}
	err = writePacket(c, pub)
	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}
	packet, err := readPacket(c)
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

	srv.subscriptionsDB.Unsubscribe("MQTT", []string{"t0", "t1", "t2"}...)

	for _, topic := range tt {
		m := srv.subscriptionsDB.GetTopicMatched(topic.Name)
		if len(m) != 0 {
			t.Fatalf("UnSubscribe error,  got %v", m)
		}
	}
}

func TestServer_Publish(t *testing.T) {
	srv, conn := connectedServer(nil)
	defer srv.Stop(context.Background())
	var err error
	c := conn.(*rwTestConn)
	tt := []packets.Topic{
		{Qos: packets.QOS_0, Name: "t0"},
		{Qos: packets.QOS_1, Name: "t1"},
		{Qos: packets.QOS_2, Name: "t2"},
	}
	srv.subscriptionsDB.Subscribe("MQTT", tt...)
	pub := &packets.Publish{
		Dup:       false,
		Qos:       packets.QOS_0,
		Retain:    false,
		TopicName: []byte("t0"),
		Payload:   []byte("payload"),
	}
	srv.publishService.Publish(NewMessage(string(pub.TopicName), pub.Payload, pub.Qos, Retained(pub.Retain)))
	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}
	packet, err := readPacket(c)

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

func TestServer_PublishToClientWithMatch(t *testing.T) {
	a := assert.New(t)
	srv, conn := connectedServer(nil)
	defer srv.Stop(context.Background())

	c := conn.(*rwTestConn)
	tt := []packets.Topic{
		{Qos: packets.QOS_0, Name: "t0"},
		{Qos: packets.QOS_1, Name: "t1"},
		{Qos: packets.QOS_2, Name: "t2"},
	}
	srv.subscriptionsDB.Subscribe("MQTT", tt...)

	// create a package for an unsubscribed channel
	pubU := &packets.Publish{
		Dup:       false,
		Qos:       packets.QOS_0,
		Retain:    false,
		TopicName: []byte("tu"),
		Payload:   []byte("payload"),
	}
	srv.publishService.PublishToClient("MQTT",
		NewMessage(string(pubU.TopicName), pubU.Payload, pubU.Qos, Retained(pubU.Retain)),
		true) // publish with match=true, i.e. the pubU package should not be delivered

	pub := &packets.Publish{
		Dup:       false,
		Qos:       packets.QOS_0,
		Retain:    false,
		TopicName: []byte("t0"),
		Payload:   []byte("payload"),
	}
	srv.publishService.PublishToClient("MQTT",
		NewMessage(string(pub.TopicName), pub.Payload, pub.Qos, Retained(pub.Retain)),
		true)
	packet, err := readPacket(c)
	a.Nil(err)

	if p, ok := packet.(*packets.Publish); ok {
		a.NotEqual(string(p.TopicName), string(pubU.TopicName), "Match error, received message for unsubscribed topic")
	} else {
		t.Fatalf("unexpected Packet Type, want %v, got %v", reflect.TypeOf(&packets.Publish{}), reflect.TypeOf(packet))
	}
}

func TestServer_PublishToClientWithoutMatch(t *testing.T) {
	a := assert.New(t)
	srv, conn := connectedServer(nil)
	defer srv.Stop(context.Background())
	c := conn.(*rwTestConn)
	tt := []packets.Topic{
		{Qos: packets.QOS_0, Name: "t0"},
		{Qos: packets.QOS_1, Name: "t1"},
		{Qos: packets.QOS_2, Name: "t2"},
	}
	srv.subscriptionsDB.Subscribe("MQTT", tt...)

	// create a package for an unsubscribed channel
	pubU := &packets.Publish{
		Dup:       false,
		Qos:       packets.QOS_0,
		Retain:    false,
		TopicName: []byte("tu"),
		Payload:   []byte("payload"),
	}
	srv.publishService.PublishToClient("MQTT",
		NewMessage(string(pubU.TopicName), pubU.Payload, pubU.Qos, Retained(pubU.Retain)),
		false) // publish with match=false, i.e. the pubU package should be delivered

	pub := &packets.Publish{
		Dup:       false,
		Qos:       packets.QOS_0,
		Retain:    false,
		TopicName: []byte("t0"),
		Payload:   []byte("payload"),
	}
	srv.publishService.PublishToClient("MQTT",
		NewMessage(string(pub.TopicName), pub.Payload, pub.Qos, Retained(pub.Retain)),
		true)
	packet, err := readPacket(c)
	a.Nil(err)

	if p, ok := packet.(*packets.Publish); ok {
		a.Equal(string(pubU.TopicName), string(p.TopicName), "Match error, did not receive message for unsubscribed topic")
	} else {
		t.Fatalf("unexpected Packet Type, want %v, got %v", reflect.TypeOf(&packets.Publish{}), reflect.TypeOf(packet))
	}
}

func TestUnsubscribe(t *testing.T) {
	srv, conn := connectedServer(nil)
	defer srv.Stop(context.Background())
	c := conn.(*rwTestConn)
	sub := &packets.Subscribe{
		PacketID: 10,
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
		PacketID: 11,
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
		if unsuback.PacketID != unsub.PacketID {
			t.Fatalf("PacketID error, want %d, got %d", sub.PacketID, unsuback.PacketID)
		}
	} else {
		t.Fatalf("unexpected Packet Type, want %v, got %v", reflect.TypeOf(&packets.Unsuback{}), reflect.TypeOf(p))
	}

	pub := &packets.Publish{
		Dup:       false,
		Qos:       packets.QOS_0,
		Retain:    false,
		TopicName: []byte("/a/b/cc"),
		PacketID:  11,
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
	srv := newTestServer()
	srv.hooks.OnSubscribe = func(ctx context.Context, client Client, topic packets.Topic) (qos uint8) {
		if topic.Qos == packets.QOS_0 {
			return packets.QOS_1
		}
		if topic.Name == "/a/b/+" {
			return packets.SUBSCRIBE_FAILURE
		}
		return topic.Qos
	}
	conn := doconnect(srv, nil)
	defer srv.Stop(context.Background())
	c := conn.(*rwTestConn)

	sub := &packets.Subscribe{
		PacketID: 10,
		Topics: []packets.Topic{
			{Name: "/a/b/c", Qos: packets.QOS_0},
			{Name: "/a/b/+", Qos: packets.QOS_1},
		},
	}
	err := writePacket(c, sub)

	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}
	packet, _ := readPacket(c)
	if p, ok := packet.(*packets.Suback); ok {
		if p.PacketID != sub.PacketID {
			t.Fatalf("PacketID error, want %d, got %d", sub.PacketID, p.PacketID)
		}
		if !bytes.Equal(p.Payload, []byte{packets.QOS_1, packets.SUBSCRIBE_FAILURE}) {
			t.Fatalf("Payload error, want %v, got %v", []byte{packets.QOS_1, packets.SUBSCRIBE_FAILURE}, p.Payload)
		}

		m := srv.subscriptionsDB.GetTopicMatched("/a/b/+")
		if len(m) != 0 {
			t.Fatalf("onSubscribe error, got %v", m)
		}
		m = srv.subscriptionsDB.GetTopicMatched("/a/b/c")
		if ts, ok := m["MQTT"]; ok {
			if ts[0].Qos != packets.QOS_1 {
				t.Fatalf("onSubscribe error, got %v", ts)
			}
		}
	} else {
		t.Fatalf("unexpected Packet Type, want %v, got %v", reflect.TypeOf(&packets.Suback{}), reflect.TypeOf(packet))
	}

}

func TestRetainMsg(t *testing.T) {
	a := assert.New(t)
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
		PacketID:  10,
		Payload:   payload,
	}
	err = writePacket(c, pub)
	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}
	readPacket(c) //read puback
	sub := &packets.Subscribe{
		PacketID: 11,
		Topics: []packets.Topic{
			{Name: string(topicName), Qos: packets.QOS_2},
		},
	}
	err = writePacket(c, sub)

	retain := srv.retainedDB.GetRetainedMessage("a/b")
	a.NotNil(retain)

	a.Equal(pub.Qos, retain.Qos())
	a.Equal(string(pub.TopicName), retain.Topic())
	a.Equal(pub.Payload, retain.Payload())

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
		PacketID:  10,
		Payload:   payload,
	}
	writePacket(c, pub0)
	packet, _ := readPacket(c) //publish
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
		PacketID: 10,
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
		PacketID:  11,
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
			originalPid = pub.PacketID
		}
	}
	p, err := readPacketWithTimeOut(c, 2*testRedeliveryInternal)
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
		if pub.PacketID != originalPid {
			t.Fatalf("PacketID error, want %d, got %d", originalPid, pub.PacketID)
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
		PacketID:  senderPid,
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
		if pubrec2.PacketID != pubrec.PacketID {
			t.Fatalf("PacketID error, want %d, got %d", pubrec.PacketID, pubrec2.PacketID)
		}
		p, err = readPacketWithTimeOut(reciver, 10*time.Second)
		if err != errTestReadTimeout {
			t.Fatalf("delivery duplicated messages， %v", reflect.TypeOf(p))
		}
		err = writePacket(sender, pubrec.NewPubrel())
		if err != nil {
			t.Fatalf("unexpected error:%s", err)
		}
		p, _ = readPacket(sender) //pubcomp

		if _, ok := p.(*packets.Pubcomp); !ok {
			t.Fatalf("unexpected Packet Type, want %v, got %v", reflect.TypeOf(&packets.Pubcomp{}), reflect.TypeOf(p))
		}
	} else {
		t.Fatalf("unexpected Packet Type, want %v, got %v", reflect.TypeOf(&packets.Pubrec{}), reflect.TypeOf(pubrec))
	}

	p, _ = readPacket(reciver) //pubrel

	if _, ok := p.(*packets.Pubrel); !ok {
		t.Fatalf("unexpected Packet Type, want %v, got %v", reflect.TypeOf(&packets.Pubrel{}), reflect.TypeOf(p))
	}
	pubrel1 := p.(*packets.Pubrel)

	p, err = readPacketWithTimeOut(reciver, (redeliveryTime+1)*time.Second) //redelivery pubrel
	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}
	if pubrel2, ok := p.(*packets.Pubrel); ok {
		if pubrel1.PacketID != pubrel2.PacketID {
			t.Fatalf("PacketID error, want %d, got %d", pubrel1.PacketID, pubrel2.PacketID)
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
		PacketID: 10,
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
		PacketID:  10,
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
			originalPid = pub.PacketID
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
	readPacket(reConn)         //connack
	p, _ := readPacket(reConn) //redelivery publish
	if pub, ok := p.(*packets.Publish); ok {
		if !bytes.Equal([]byte("test"), pub.TopicName) {
			t.Fatalf("TopicName error, want %v, got %v", []byte("test"), pub.TopicName)
		}
		if !bytes.Equal([]byte("payload"), pub.Payload) {
			t.Fatalf("Payload error, want %v, got %v", []byte("payload"), pub.Payload)
		}
		if pub.PacketID != originalPid {
			t.Fatalf("PacketID error, want %d, got %d", originalPid, pub.PacketID)
		}
		if pub.Dup != true {
			t.Fatalf("Dup error, want %t, got %t", true, false)
		}
	} else {
		t.Fatalf("unexpected Packet Type, want %v, got %v", reflect.TypeOf(&packets.Publish{}), reflect.TypeOf(p))
	}

}

func TestOfflineMessageQueueing(t *testing.T) {
	a := assert.New(t)
	c := DefaultConfig
	c.MaxMsgQueue = 5
	srv = NewServer(WithConfig(c), WithLogger(zap.NewNop()))
	defer func() {
		srv = nil
	}()
	conn1 := defaultConnectPacket()
	conn1.CleanSession = false
	conn1.ClientID = []byte("id1")

	conn2 := defaultConnectPacket()
	conn2.CleanSession = false
	conn2.ClientID = []byte("id2")

	srv, s, r := connectedServerWith2Client(conn1, conn2)
	defer srv.Stop(context.Background())
	var err error
	sender := s.(*rwTestConn)
	reciver := r.(*rwTestConn)
	sub := &packets.Subscribe{
		PacketID: 10,
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

	for i := 0x31; i <= 0x36; i++ { //assic 1 to 6,packet 1 will be dropped
		pub := &packets.Publish{
			Dup:       false,
			Qos:       packets.QOS_1,
			Retain:    false,
			TopicName: []byte{byte(i)},
			PacketID:  uint16(i),
			Payload:   []byte{byte(i), byte(i)},
		}
		err = writePacket(sender, pub)
		if err != nil {
			t.Fatalf("unexpected error:%s", err)
		}
	}

	reConn := &rwTestConn{
		closec:    make(chan struct{}),
		readChan:  make(chan []byte, 1024),
		writeChan: make(chan []byte, 1024),
		netAddr:   "reciver",
	}
	srv.tcpListener[0].(*testListener).conn.PushBack(reConn)
	srv.tcpListener[0].(*testListener).acceptReady <- struct{}{}

	time.Sleep(2 * time.Second)

	cl := srv.Client(string(conn2.ClientID))
	stats := cl.GetSessionStatsManager().GetStats()
	a.EqualValues(1, stats.Qos1.DroppedTotal)
	a.EqualValues(5, stats.QueuedCurrent)

	err = writePacket(reConn, conn2)
	if err != nil {
		t.Fatalf("unexpected error:%s", err)
	}
	readPacket(reConn) //connack

	for i := 0x32; i <= 0x36; i++ { //assic 2 to 6
		p, err := readPacket(reConn)
		if err != nil {
			t.Fatalf("unexpected error:%s", err)
		}
		if pub, ok := p.(*packets.Publish); ok {
			if !bytes.Equal([]byte{byte(i), byte(i)}, pub.Payload) {
				t.Fatalf("[%x]Payload error, want % x, got % x % x", i, []byte{byte(i), byte(i)}, pub.Payload, string(pub.Payload))
			}
			if !bytes.Equal([]byte{byte(i)}, pub.TopicName) {
				t.Fatalf("[%x]TopicName error, want % x, got % x", i, []byte{byte(i)}, pub.TopicName)
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
	cl = srv.Client(string(conn2.ClientID))
	stats = cl.GetSessionStatsManager().GetStats()
	a.EqualValues(1, stats.Qos1.DroppedTotal)
	a.EqualValues(0, stats.QueuedCurrent)
}

func TestWillMsg(t *testing.T) {
	srv, s, r := connectedServerWith2Client()
	defer srv.Stop(context.Background())
	var err error
	sender := s.(*rwTestConn)
	reciver := r.(*rwTestConn)
	sub := &packets.Subscribe{
		PacketID: 10,
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
		PacketID: 10,
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

func TestEmptyClientID(t *testing.T) {
	connect := defaultConnectPacket()
	connect.ClientID = make([]byte, 0)
	connect.CleanSession = true
	srv, _ := connectedServer(connect)
	defer srv.Stop(context.Background())
	if len(srv.clients) != 1 {
		t.Fatalf("len error, want %d, got %d", 1, len(srv.clients))
	}
	for id := range srv.clients {
		if id == "" {
			t.Fatalf("id is empty, should be generated as a uuid")
		}
	}
}
