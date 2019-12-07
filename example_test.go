package gmqtt

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/DrmagicE/gmqtt/pkg/packets"
)

//see /examples for more details.
func Example() {
	ln, err := net.Listen("tcp", ":1883")
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	ws := &WsServer{
		Server: &http.Server{Addr: ":8080"},
		Path:   "/",
	}
	srv := NewServer(
		TCPListener(ln),
		WebsocketServer(ws),
		Hook(Hooks{
			OnConnect: func(ctx context.Context, client Client) (code uint8) {
				return packets.CodeAccepted
			},
			OnSubscribe: func(ctx context.Context, client Client, topic packets.Topic) (qos uint8) {
				fmt.Println("register onSubscribe callback")
				return packets.QOS_1
			},
		}),
	)

	srv.Run()
	fmt.Println("started...")
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	<-signalCh
	srv.Stop(context.Background())
	fmt.Println("stopped")
}
