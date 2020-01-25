package main

import (
	"context"
	"fmt"
	"log"
	"net"

	"github.com/DrmagicE/gmqtt"

	//_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	/*	go func() {
		http.ListenAndServe("127.0.0.1:6060", nil)
	}()*/
	config := gmqtt.DefaultConfig
	config.MaxInflight = 65535
	config.MaxMsgQueue = 0 //unlimited
	config.MaxAwaitRel = 0 //unlimited
	config.RegisterLen = 10000
	config.UnregisterLen = 10000

	ln, err := net.Listen("tcp", ":1883")
	if err != nil {
		log.Fatalln(err.Error())
		return
	}
	s := gmqtt.NewServer(
		gmqtt.WithConfigure(config),
		gmqtt.WithTCPListener(ln),
	)
	s.Run()
	fmt.Println("started...")
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	<-signalCh
	s.Stop(context.Background())
	fmt.Println("stopped")
}
