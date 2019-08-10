package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"context"
	"net"
	"net/http"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/plugin/management"
)

func main() {

	s := gmqtt.DefaultServer()

	// listener
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
		Server: &http.Server{Addr: ":8081"},
	}
	s.AddWebSocketServer(ws, wss)

	// plugin
	s.AddPlugins(management.New(":9090", nil))

	log.Println("started...")
	s.Run()
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	<-signalCh
	s.Stop(context.Background())
	log.Println("stopped")

}
