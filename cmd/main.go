package main

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

var f mqtt.MessageHandler = func(client mqtt.Client, msg mqtt.Message) {
	fmt.Printf("TOPIC: %s\n", msg.Topic())
	fmt.Printf("MSG: %s\n", msg.Payload())
}

func main() {
	opts := mqtt.NewClientOptions().AddBroker("tcp://127.0.0.1:1883").SetClientID("gotrivial")
	opts.SetKeepAlive(2 * time.Second)
	opts.SetPingTimeout(1 * time.Second)

	c := mqtt.NewClient(opts)
	if token := c.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	var s int
	mu := sync.Mutex{}
	if token := c.Subscribe("#", 1, func(client mqtt.Client, message mqtt.Message) {
		mu.Lock()
		defer mu.Unlock()
		s++
	}); token.Wait() && token.Error() != nil {
		fmt.Println(token.Error())
		os.Exit(1)
	}

	go func() {
		t := time.NewTicker(5 * time.Second)
		for {
			<-t.C
			mu.Lock()
			log.Println(s)
			mu.Unlock()
		}
	}()
	select {}
}
