package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/plugin/management"
)

func main() {
	// listener
	ln, err := net.Listen("tcp", ":1883")
	if err != nil {
		log.Fatalln(err.Error())
		return
	}
	ws := &gmqtt.WsServer{
		Server: &http.Server{Addr: ":8080"},
		Path:   "/ws",
	}
	s := gmqtt.NewServer(
		gmqtt.TCPListener(ln),
		gmqtt.WebsocketServer(ws),
		gmqtt.Plugin(management.New(":9090", nil)),
		gmqtt.Mode(gmqtt.ReleaseMode),
	)
	s.Run()
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	<-signalCh
	s.Stop(context.Background())

}
