package gmqtt

import (
	"context"
	"fmt"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

//see /examples for more details.
func Example() {
	s := NewServer()
	ln, err := net.Listen("tcp", ":1883")
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	s.AddTCPListenner(ln)
	ws := &WsServer{
		Server: &http.Server{Addr: ":8080"},
	}
	s.AddWebSocketServer(ws)

	s.RegisterOnConnect(func(client *Client) (code uint8) {
		return packets.CodeAccepted
	})
	s.RegisterOnSubscribe(func(client *Client, topic packets.Topic) uint8 {
		fmt.Println("register onSubscribe callback")
		return packets.QOS_1
	})
	//register other callback before s.Run()

	s.Run()
	fmt.Println("started...")
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	<-signalCh
	s.Stop(context.Background())
	fmt.Println("stopped")
}
