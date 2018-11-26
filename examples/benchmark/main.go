package main

import (
	"context"
	"fmt"
	"github.com/DrmagicE/gmqtt/server"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	go func() {
		http.ListenAndServe("127.0.0.1:6060", nil)
	}()
	s := server.NewServer()
	s.SetMsgRouterLen(5000000)
	s.SetRegisterLen(10000)
	s.SetUnregisterLen(10000)
	s.SetMaxQueueMessages(0) //unlimited
	ln, err := net.Listen("tcp", ":1883")
	if err != nil {
		log.Fatalln(err.Error())
		return
	}

	s.AddTCPListenner(ln)
	s.Run()
	fmt.Println("started...")
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	<-signalCh
	s.Stop(context.Background())
	fmt.Println("stopped")
}
