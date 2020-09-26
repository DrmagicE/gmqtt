package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/DrmagicE/gmqtt/server"
	"github.com/DrmagicE/gmqtt/subscription"
	_ "github.com/DrmagicE/gmqtt/topicalias" // set default topicalias manager
)

func main() {
	ln, err := net.Listen("tcp", ":1883")
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	l, _ := zap.NewDevelopment()
	srv := server.New(
		server.WithTCPListener(ln),
		server.WithLogger(l),
	)

	// subscription store
	subStore := srv.SubscriptionStore()
	srv.Init(server.WithHook(server.Hooks{
		OnConnected: func(ctx context.Context, client server.Client) {
			// add subscription for a client when it is connected
			subStore.Subscribe(client.ClientOptions().ClientID, &gmqtt.Subscription{
				TopicFilter: "topic",
				QoS:         packets.Qos0,
			})
		},
	}))

	// retained store
	retainedStore := srv.RetainedStore()
	// add a retained message
	retainedStore.AddOrReplace(&gmqtt.Message{
		QoS:      packets.Qos1,
		Retained: true,
		Topic:    "a/b/c",
		Payload:  []byte("retained message"),
	})

	// publish service
	pub := srv.PublishService()

	srv.Run()

	go func() {
		for {
			<-time.NewTimer(5 * time.Second).C
			// iterate all topics
			subStore.Iterate(func(clientID string, sub *gmqtt.Subscription) bool {
				fmt.Printf("client id: %s, subscription: %v \n", clientID, sub)
				return true
			}, subscription.IterationOptions{
				Type: subscription.TypeAll,
			})
			// publish a message to the broker
			pub.Publish(&gmqtt.Message{
				QoS:     packets.Qos1,
				Topic:   "topic",
				Payload: []byte("abc"),
			})
		}

	}()
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	<-signalCh
	srv.Stop(context.Background())
}
