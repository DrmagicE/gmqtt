package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/pkg/packets"
)

func main() {
	go func() {
		http.ListenAndServe("127.0.0.1:6060", nil)
	}()
	s := gmqtt.DefaultServer()
	ln, err := net.Listen("tcp", ":1883")
	if err != nil {
		log.Fatalln(err.Error())
		return
	}
	crt, err := tls.LoadX509KeyPair("../testcerts/server.crt", "../testcerts/server.key")
	if err != nil {
		log.Fatalln(err.Error())
		return
	}
	tlsConfig := &tls.Config{}
	tlsConfig.Certificates = []tls.Certificate{crt}
	tlsln, err := tls.Listen("tcp", ":8883", tlsConfig)

	if err != nil {
		log.Fatalln(err.Error())
		return
	}
	s.AddTCPListenner(ln)
	s.AddTCPListenner(tlsln)

	s.RegisterOnSubscribe(func(cs gmqtt.ChainStore, client gmqtt.Client, topic packets.Topic) (qos uint8) {
		if topic.Name == "test/nosubscribe" {
			return packets.SUBSCRIBE_FAILURE
		}
		return topic.Qos
	})

	//server.SetLogger(logger.NewLogger(os.Stderr, "", log.LstdFlags))
	s.Run()
	fmt.Println("started...")
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	<-signalCh
	s.Stop(context.Background())
	fmt.Println("stopped")
}
