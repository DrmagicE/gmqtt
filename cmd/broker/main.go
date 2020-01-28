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
	"github.com/DrmagicE/gmqtt/plugin/management"
	"github.com/DrmagicE/gmqtt/plugin/prometheus"
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
	if err != nil {
		panic(err)
	}

	l, _ := zap.NewProduction()
	//l, _ := zap.NewDevelopment()
	s := gmqtt.NewServer(
		gmqtt.WithTCPListener(ln),
		gmqtt.WithWebsocketServer(ws),
		gmqtt.WithPlugin(management.New(":8081", nil)),
		gmqtt.WithPlugin(prometheus.New(&http.Server{
			Addr: ":8082",
		}, "/metrics")),
		gmqtt.WithLogger(l),
	)
	s.Run()
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	<-signalCh
	s.Stop(context.Background())

}
