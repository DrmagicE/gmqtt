package run

import (
	"crypto/tls"
	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/logger"
	"log"
	"net"
	"net/http"
	"os"
	"time"
)

// NewServer creates a new  gmqtt.Server instance by the given Config param.
func NewServer(config *Config) (*gmqtt.Server, error) {
	startProfile(config.ProfileConfig.CPUProfile, config.ProfileConfig.MemProfile)
	srv := gmqtt.NewServer()
	srv.SetDeliveryRetryInterval(time.Second * time.Duration(config.DeliveryRetryInterval))
	srv.SetMaxInflightMessages(config.MaxInflightMessages)
	srv.SetQueueQos0Messages(config.QueueQos0Messages)
	srv.SetMaxQueueMessages(config.MaxMsgQueueMessages)
	srv.SetMaxQueueMessages(0) //unlimited
	var l net.Listener
	var ws *gmqtt.WsServer
	var err error
	for _, v := range config.Listener {
		if v.Protocol == ProtocolMQTT {
			if v.KeyFile == "" {
				l, err = net.Listen("tcp", v.Addr)
				if err != nil {
					return nil, err
				}
			} else {
				crt, err := tls.LoadX509KeyPair(v.CertFile, v.KeyFile)
				if err != nil {
					return nil, err
				}
				tlsConfig := &tls.Config{}
				tlsConfig.Certificates = []tls.Certificate{crt}
				l, _ = tls.Listen("tcp", v.Addr, tlsConfig)
			}
			srv.AddTCPListenner(l)
		} else {
			if v.KeyFile == "" {
				ws = &gmqtt.WsServer{
					Server: &http.Server{Addr: v.Addr},
				}
			} else {
				ws = &gmqtt.WsServer{
					Server:   &http.Server{Addr: v.Addr},
					CertFile: v.CertFile,
					KeyFile:  v.KeyFile,
				}
			}
			srv.AddWebSocketServer(ws)
		}
	}
	if config.Logging {
		gmqtt.SetLogger(logger.NewLogger(os.Stderr, "", log.LstdFlags))
	}

	return srv, nil
}
