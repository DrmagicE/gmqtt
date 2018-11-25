package server

import (
	"net"
	"testing"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"context"

)

func TestHooks(t *testing.T) {
	srv := NewServer()
	ln, err := net.Listen("tcp","127.0.0.1:1883")
	if err != nil {
		t.Fatalf("unexpected error: %s",err)
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

	srv.OnClose = func(client *Client,err error) {
		hooks += "OnClose"

	}
	srv.OnStop = func() {
		hooks += "OnStop"
	}

	srv.Run()

	c, err := net.Dial("tcp","127.0.0.1:1883")
	if err != nil {
		t.Fatalf("unexpected error: %s",err)
	}

	w := packets.NewWriter(c)
	r := packets.NewReader(c)
	w.WriteAndFlush(defaultConnectPacket())
	r.ReadPacket()

	sub := &packets.Subscribe{
		PacketId:10,
		Topics:[]packets.Topic{
			{Name:"name",Qos:packets.QOS_1},
		},
	}
	w.WriteAndFlush(sub)
	r.ReadPacket()//suback

	pub := &packets.Publish{
		Dup:false,
		Qos:packets.QOS_1,
		Retain:false,
		TopicName:[]byte("ok"),
		PacketId:10,
		Payload:[]byte("payload"),

	}
	w.WriteAndFlush(pub)
	r.ReadPacket() //puback
	srv.Stop(context.Background())
	want := "AcceptOnConnectOnSubscribeOnPublishOnCloseOnStop"
	if hooks != want {
		t.Fatalf("hooks error, want %s, got %s",want, hooks)
	}
}





