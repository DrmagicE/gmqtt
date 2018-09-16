package main


import (
	"fmt"
	"github.com/DrmagicE/gmqtt/server"
	"net"
	"log"
	"os"
	"os/signal"
	"syscall"
	"context"
)

func main() {
	s := server.NewServer()
	ln, err := net.Listen("tcp",":1883")
	if err != nil {
		log.Fatalln(err.Error())
		return
	}
	s.AddTCPListenner(ln)
	ws := &server.WsServer{
		Addr:":8080",
	}
	wss := &server.WsServer{
		Addr:":8081",
		CertFile:"../testcerts/server.crt",
		KeyFile:"../testcerts/server.key",
		TLSConfig:nil,
	}
	s.AddWebSocketServer(ws,wss)
	s.Run()
	fmt.Println("started...")
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	<-signalCh
	s.Stop(context.Background())
	fmt.Println("stopped")

}