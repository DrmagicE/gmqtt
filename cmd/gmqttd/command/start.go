package command

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/DrmagicE/gmqtt/config"
	"github.com/DrmagicE/gmqtt/pkg/pidfile"
	"github.com/DrmagicE/gmqtt/server"
)

var (
	ConfigFile string
	logger     *zap.Logger
)

func must(err error) {
	if err != nil {
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}
}

func installSignal(srv server.Server) {
	// reload
	reloadSignalCh := make(chan os.Signal, 1)
	signal.Notify(reloadSignalCh, syscall.SIGHUP)

	// stop
	stopSignalCh := make(chan os.Signal, 1)
	signal.Notify(stopSignalCh, os.Interrupt, syscall.SIGTERM)

	for {
		select {
		case <-reloadSignalCh:
			var c config.Config
			var err error
			c, err = config.ParseConfig(ConfigFile)
			if err != nil {
				logger.Error("reload error", zap.Error(err))
				return
			}
			srv.ApplyConfig(c)
			logger.Info("gmqtt reloaded")
		case <-stopSignalCh:
			err := srv.Stop(context.Background())
			if err != nil {
				fmt.Fprint(os.Stderr, err.Error())
			}
		}
	}

}

func GetListeners(c config.Config) (tcpListeners []net.Listener, websockets []*server.WsServer, err error) {
	for _, v := range c.Listeners {
		var ln net.Listener
		if v.Websocket != nil {
			ws := &server.WsServer{
				Server: &http.Server{Addr: v.Address},
				Path:   v.Websocket.Path,
			}
			if v.TLSOptions != nil {
				ws.KeyFile = v.Key
				ws.CertFile = v.Cert
			}
			websockets = append(websockets, ws)
			continue
		}
		if v.TLSOptions != nil {
			var cert tls.Certificate
			cert, err = tls.LoadX509KeyPair(v.Cert, v.Key)
			if err != nil {
				return
			}
			ln, err = tls.Listen("tcp", v.Address, &tls.Config{
				Certificates: []tls.Certificate{cert},
			})
		} else {
			ln, err = net.Listen("tcp", v.Address)
		}
		tcpListeners = append(tcpListeners, ln)
	}
	return
}

// NewStartCmd creates a *cobra.Command object for start command.
func NewStartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start gmqtt broker",
		Run: func(cmd *cobra.Command, args []string) {
			var err error
			must(err)
			c, err := config.ParseConfig(ConfigFile)
			if os.IsNotExist(err) {
				must(err)
			} else {
				must(err)
			}
			if c.PidFile != "" {
				pid, err := pidfile.New(c.PidFile)
				if err != nil {
					must(fmt.Errorf("open pid file failed: %s", err))
				}
				defer pid.Remove()
			}

			tcpListeners, websockets, err := GetListeners(c)
			must(err)
			l, err := c.GetLogger(c.Log)
			must(err)
			logger = l

			s := server.New(
				server.WithConfig(c),
				server.WithTCPListener(tcpListeners...),
				server.WithWebsocketServer(websockets...),
				server.WithLogger(l),
			)

			err = s.Init()
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
				return
			}
			go installSignal(s)
			err = s.Run()
			if err != nil {
				fmt.Fprint(os.Stderr, err.Error())
				os.Exit(1)
				return
			}
		},
	}
	return cmd
}
