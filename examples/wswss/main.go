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

	"github.com/DrmagicE/gmqtt/server"
	_ "github.com/DrmagicE/gmqtt/topicalias/fifo" // set default topicalias manager
)

func main() {
	ln, err := net.Listen("tcp", ":1883")
	if err != nil {
		log.Fatalln(err.Error())
		return
	}
	ws := &server.WsServer{
		Server: &http.Server{Addr: ":8080"},
		Path:   "/",
	}
	wss := &server.WsServer{
		Server:   &http.Server{Addr: ":8081"},
		Path:     "/",
		CertFile: "../testcerts/server.crt",
		KeyFile:  "../testcerts/server.key",
	}
	l, _ := zap.NewDevelopment()
	s := server.New(
		server.WithTCPListener(ln),
		server.WithWebsocketServer(ws, wss),
		server.WithLogger(l),
	)
	s.Run()
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	<-signalCh
	s.Stop(context.Background())

}
