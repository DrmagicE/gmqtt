package gmqtt

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/DrmagicE/gmqtt/plugin/prometheus"
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
	l, _ := zap.NewProduction()
	srv := NewServer(
		WithTCPListener(ln),
		WithWebsocketServer(ws),

		// add config
		WithConfig(DefaultConfig),
		// add plugins
		WithPlugin(prometheus.New(&http.Server{Addr: ":8082"}, "/metrics")),
		// add Hook
		WithHook(Hooks{
			OnConnect: func(ctx context.Context, client Client) (code uint8) {
				return packets.CodeAccepted
			},
			OnSubscribe: func(ctx context.Context, client Client, topic packets.Topic) (qos uint8) {
				fmt.Println("register onSubscribe callback")
				return packets.QOS_1
			},
		}),
		// add logger
		WithLogger(l),
	)

	srv.Run()
	fmt.Println("started...")
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	<-signalCh
	srv.Stop(context.Background())
	fmt.Println("stopped")
}

func ExampleServer_SubscriptionStore() {
	ln, err := net.Listen("tcp", ":1883")
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	srv := NewServer(
		WithTCPListener(ln),
	)
	store := srv.SubscriptionStore()
	srv.Init(WithHook(Hooks{
		OnConnected: func(ctx context.Context, client Client) {
			// add subscription for a client when it is connected
			store.Subscribe(client.OptionsReader().ClientID(), packets.Topic{
				Qos:  packets.QOS_0,
				Name: "topic1",
			})
		},
	}))
	srv.Run()
	fmt.Println("started...")
	go func() {
		for {
			<-time.NewTimer(5 * time.Second).C
			// iterate all topics
			store.Iterate(func(clientID string, topic packets.Topic) bool {
				fmt.Println("client id: %s, topic: %v", clientID, topic)
				return true
			})
		}

	}()
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	<-signalCh
	srv.Stop(context.Background())
	fmt.Println("stopped")
}

func ExampleServer_RetainedStore() {
	ln, err := net.Listen("tcp", ":1883")
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	srv := NewServer(
		WithTCPListener(ln),
	)
	store := srv.RetainedStore()
	// add a retained message
	store.AddOrReplace(NewMessage("a/b/c", []byte("abc"), packets.QOS_1, Retained(true)))
	srv.Run()
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	<-signalCh
	srv.Stop(context.Background())
	fmt.Println("stopped")
}

func ExampleServer_PublishService() {
	ln, err := net.Listen("tcp", ":1883")
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	srv := NewServer(
		WithTCPListener(ln),
	)
	pub := srv.PublishService()
	go func() {
		<-time.NewTimer(5 * time.Second).C
		// publish a message to the broker
		pub.Publish(NewMessage("topic", []byte("abc"), packets.QOS_1))
	}()
	srv.Run()
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	<-signalCh
	srv.Stop(context.Background())
	fmt.Println("stopped")
}
