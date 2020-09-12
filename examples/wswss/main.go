package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/DrmagicE/gmqtt"
)

func main() {
	ln, err := net.Listen("tcp", ":1883")
	if err != nil {
		log.Fatalln(err.Error())
		return
	}
	ws := &gmqtt.WsServer{
		Server: &http.Server{Addr: ":8080"},
		Path:   "/",
	}
	wss := &gmqtt.WsServer{
		Server:   &http.Server{Addr: ":8081"},
		Path:     "/",
		CertFile: "../testcerts/server.crt",
		KeyFile:  "../testcerts/server.key",
	}
	l, _ := zap.NewDevelopment()
	s := gmqtt.NewServer(
		gmqtt.WithTCPListener(ln),
		gmqtt.WithWebsocketServer(ws, wss),
		gmqtt.WithLogger(l),
	)
	s.Run()
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	<-signalCh
	s.Stop(context.Background())

}
