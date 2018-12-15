package main

import (
	"context"
	"fmt"
	"github.com/DrmagicE/gmqtt"
	"log"
	"net"
	//_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
)

func main() {
/*	go func() {
		http.ListenAndServe("127.0.0.1:6060", nil)
	}()*/
	s := gmqtt.NewServer()
	s.SetMaxInflightMessages(65535)
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
	s.SetMaxInflightMessages(5)
	fmt.Println("started...")
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	<-signalCh
	s.Stop(context.Background())
	fmt.Println("stopped")
}
