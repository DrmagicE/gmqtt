package main

import (
	"context"
	"crypto/tls"
	"log"
	"net"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/DrmagicE/gmqtt/server"
	_ "github.com/DrmagicE/gmqtt/topicalias" // set default topicalias manager
)

func main() {
	ln, err := net.Listen("tcp", ":1883")
	if err != nil {
		log.Fatalln(err.Error())
		return
	}
	crt, err := tls.LoadX509KeyPair("../testcerts/server.crt", "../testcerts/server.key")
	if err != nil {
		log.Fatalln(err.Error())
		return
	}
	tlsConfig := &tls.Config{}
	tlsConfig.Certificates = []tls.Certificate{crt}
	tlsln, err := tls.Listen("tcp", ":8883", tlsConfig)

	if err != nil {
		log.Fatalln(err.Error())
		return
	}
	l, _ := zap.NewDevelopment()
	s := server.New(
		server.WithTCPListener(ln, tlsln),
		server.WithLogger(l),
	)
	s.Run()
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	<-signalCh
	s.Stop(context.Background())
}
