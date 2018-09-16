package main

import (
	"fmt"
	"github.com/DrmagicE/gmqtt/server"
	"net"
	"os"
	"os/signal"
	"syscall"
	"context"
)

func main() {

	var err error
	s := server.NewServer()
	l, err := net.Listen("tcp",":1883")
	if err != nil {
		fmt.Println(err)
		return
	}
	s.AddTCPListenner(l)
	s.Run()
	fmt.Println("started..")
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	<-signalCh
	s.Stop(context.Background())
	fmt.Println("stopped..")
}


