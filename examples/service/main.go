package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/DrmagicE/gmqtt"
	_ "github.com/DrmagicE/gmqtt/persistence"
	"github.com/DrmagicE/gmqtt/persistence/subscription"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/DrmagicE/gmqtt/server"
	_ "github.com/DrmagicE/gmqtt/topicalias/fifo"
)

func main() {
	go func() {
		http.ListenAndServe(":6060", nil)
	}()
	ln, err := net.Listen("tcp", ":1883")
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	cfg := zap.NewDevelopmentConfig()
	cfg.Level.SetLevel(zap.InfoLevel)
	l, _ := cfg.Build()
	srv := server.New(
		server.WithTCPListener(ln),
		server.WithLogger(l),
	)

	var subService server.SubscriptionService
	err = srv.Init(server.WithHook(server.Hooks{
		OnConnected: func(ctx context.Context, client server.Client) {
			// add subscription for a client when it is connected
			subService = srv.SubscriptionService()
			subService.Subscribe(client.ClientOptions().ClientID, &gmqtt.Subscription{
				TopicFilter: "topic",
				QoS:         packets.Qos0,
			})
		},
	}))

	if err != nil {
		fmt.Println(err.Error())
		return
	}

	// retained service
	retainedService := srv.RetainedService()

	// publish service
	pub := srv.Publisher()

	// add a retained message
	retainedService.AddOrReplace(&gmqtt.Message{
		QoS:      packets.Qos1,
		Retained: true,
		Topic:    "a/b/c",
		Payload:  []byte("retained message"),
	})

	// publish service

	srv.Run()

	go func() {
		for {
			<-time.NewTimer(5 * time.Second).C
			// iterate all topics
			subService.Iterate(func(clientID string, sub *gmqtt.Subscription) bool {
				fmt.Printf("client id: %s, topic: %v \n", clientID, sub.TopicFilter)
				return true
			}, subscription.IterationOptions{
				Type: subscription.TypeAll,
			})
			// publish a message to the broker
			pub.Publish(&gmqtt.Message{
				Topic:   "topic",
				Payload: []byte("abc"),
				QoS:     packets.Qos1,
			})

		}

	}()
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	<-signalCh
	srv.Stop(context.Background())
}
