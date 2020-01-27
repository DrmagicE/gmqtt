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
	"github.com/DrmagicE/gmqtt/pkg/packets"
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
	//cfg := zap.Config{
	//	Level:            zap.NewAtomicLevelAt(zap.DebugLevel),
	//	Development:      true,
	//	Encoding:         "console",
	//	EncoderConfig:    zap.NewDevelopmentEncoderConfig(),
	//	OutputPaths:      []string{"stderr"},
	//	ErrorOutputPaths: []string{"stderr"},
	//}
	//
	//l, err := cfg.Build()
	if err != nil {
		panic(err)
	}
	s := gmqtt.NewServer(
		gmqtt.WithTCPListener(ln),
		gmqtt.WithWebsocketServer(ws),
		//gmqtt.WithPlugin(management.New(":9090", nil)),
		gmqtt.WithPlugin(prometheus.New(&http.Server{
			Addr: ":8081",
		}, "/metrics")),
		gmqtt.WithLogger(zap.NewNop()),
		gmqtt.WithHook(gmqtt.Hooks{
			OnSubscribe: func(ctx context.Context, client gmqtt.Client, topic packets.Topic) (qos uint8) {
				if topic.Name == "test/nosubscribe" {
					return packets.SUBSCRIBE_FAILURE
				}
				return topic.Qos
			},
		}),
	)

	s.Run()
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	<-signalCh
	s.Stop(context.Background())

}
