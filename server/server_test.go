package server

import (
	"net"
	"testing"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"context"

)

func TestHooks(t *testing.T) {
	srv := newTestServer()
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

	srv.OnClose = func(client *Client) {
		hooks += "OnClose"

	}
	srv.OnStop = func() {
		hooks += "OnStop"
	}
	ln := srv.tcpListener[0].(*testListener)
	closec := make(chan struct{})
	conn := &rwTestConn{
		closec:    closec,
		readChan:  make(chan []byte, 1024),
		writeChan: make(chan []byte, 1024),
	}
	ln.conn.PushBack(conn)
	srv.Run()
	ln.acceptReady <- struct{}{}
	writePacket(conn, defaultConnectPacket())
	readPacket(conn) //connack
	sub := &packets.Subscribe{
		PacketId:10,
		Topics:[]packets.Topic{
			{Name:"name",Qos:packets.QOS_1},
		},
	}
	writePacket(conn,sub)


	readPacket(conn) //suback



	pub := &packets.Publish{
		Dup:false,
		Qos:packets.QOS_1,
		Retain:false,
		TopicName:[]byte("ok"),
		PacketId:10,
		Payload:[]byte("payload"),

	}
	writePacket(conn,pub)
	readPacket(conn) //puback
	srv.Stop(context.Background())
	want := "AcceptOnConnectOnSubscribeOnPublishOnCloseOnStop"
	if hooks != want {
		t.Fatalf("hooks error, want %s, got %s",want, hooks)
	}
}
