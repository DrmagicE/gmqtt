package main

import (
	"context"
	"fmt"
	"github.com/DrmagicE/gmqtt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	s := gmqtt.NewServer()
	ln, err := net.Listen("tcp", ":1883")
	if err != nil {
		log.Fatalln(err.Error())
		return
	}
	s.AddTCPListenner(ln)
	ws := &gmqtt.WsServer{
		Server: &http.Server{Addr: ":8080"},
	}
	wss := &gmqtt.WsServer{
		Server:   &http.Server{Addr: ":8081"},
		CertFile: "../testcerts/server.crt",
		KeyFile:  "../testcerts/server.key",
	}
	s.AddWebSocketServer(ws, wss)
	s.Run()
	fmt.Println("started...")
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	<-signalCh
	s.Stop(context.Background())
	fmt.Println("stopped")

}
