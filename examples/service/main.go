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
	"github.com/DrmagicE/gmqtt/subscription"
)

func main() {
	ln, err := net.Listen("tcp", ":1883")
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	l, _ := zap.NewDevelopment()
	srv := gmqtt.NewServer(
		gmqtt.WithTCPListener(ln),
		gmqtt.WithLogger(l),
	)

	// subscription store
	subStore := srv.SubscriptionStore()
	srv.Init(gmqtt.WithHook(gmqtt.Hooks{
		OnConnected: func(ctx context.Context, client gmqtt.Client) {
			// add subscription for a client when it is connected
			subStore.Subscribe(client.ClientOptions().ClientID, subscription.New("topic", packets.Qos0))
		},
	}))

	// retained store
	retainedStore := srv.RetainedStore()
	// add a retained message
	retainedStore.AddOrReplace(gmqtt.NewMessage("a/b/c", []byte("retained message"), packets.Qos1, gmqtt.Retained(true)))

	// publish service
	pub := srv.PublishService()

	srv.Run()

	go func() {
		for {
			<-time.NewTimer(5 * time.Second).C
			// iterate all topics
			subStore.Iterate(func(clientID string, sub subscription.Subscription) bool {
				fmt.Printf("client id: %s, subscription: %v \n", clientID, sub)
				return true
			}, subscription.IterationOptions{
				Type: subscription.TypeAll,
			})
			// publish a message to the broker
			pub.Publish(gmqtt.NewMessage("topic", []byte("abc"), packets.Qos1))
		}

	}()
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	<-signalCh
	srv.Stop(context.Background())
}
