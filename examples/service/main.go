package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/pkg/packets"
)

func main() {
	ln, err := net.Listen("tcp", ":1883")
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	srv := gmqtt.NewServer(
		gmqtt.WithTCPListener(ln),
	)

	// subscription store
	subStore := srv.SubscriptionStore()
	srv.Init(gmqtt.WithHook(gmqtt.Hooks{
		OnConnected: func(ctx context.Context, client gmqtt.Client) {
			// add subscription for a client when it is connected
			subStore.Subscribe(client.OptionsReader().ClientID(), packets.Topic{
				Qos:  packets.QOS_0,
				Name: "topic1",
			})
		},
	}))

	// retained store
	retainedStore := srv.RetainedStore()
	// add a retained message
	retainedStore.AddOrReplace(gmqtt.NewMessage("a/b/c", []byte("abc"), packets.QOS_1, gmqtt.Retained(true)))

	// publish service
	pub := srv.PublishService()

	srv.Run()
	fmt.Println("started...")
	go func() {
		for {
			<-time.NewTimer(5 * time.Second).C
			// iterate all topics
			subStore.Iterate(func(clientID string, topic packets.Topic) bool {
				fmt.Printf("client id: %s, topic: %v \n", clientID, topic)
				return true
			})
			// publish a message to the broker
			pub.Publish(gmqtt.NewMessage("topic", []byte("abc"), packets.QOS_1))
		}

	}()
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	<-signalCh
	srv.Stop(context.Background())
	fmt.Println("stopped")
}
