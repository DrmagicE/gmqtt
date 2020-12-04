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
	"github.com/DrmagicE/gmqtt/pkg/codes"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/DrmagicE/gmqtt/pkg/pidfile"
	"github.com/DrmagicE/gmqtt/server"
)

var (
	ConfigFile string
	logger     *zap.Logger
)

func must(err error) {
	if err != nil {
		fmt.Println(err)
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
			if os.IsNotExist(err) {
				// if config file not exist, use default configration.
				c = config.DefaultConfig()
			} else {
				logger.Error("reload error", zap.Error(err))

			}
			srv.ApplyConfig(c)
			logger.Info("gmqtt reloaded")
		case <-stopSignalCh:
			srv.Stop(context.Background())
			return
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
				ws.KeyFile = v.KeyFile
				ws.CertFile = v.CertFile
			}
			websockets = append(websockets, ws)
			continue
		}
		if v.TLSOptions != nil {
			var cert tls.Certificate
			cert, err = tls.LoadX509KeyPair(v.CertFile, v.KeyFile)
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
	cfg := config.DefaultConfig()
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start gmqtt broker",
		Run: func(cmd *cobra.Command, args []string) {
			var err error
			must(err)
			c, err := config.ParseConfig(ConfigFile)
			var useDefault bool
			if os.IsNotExist(err) {
				// if config file not exist, use default configration.
				c = cfg
				useDefault = true
			} else {
				must(err)
			}
			err = c.Validate()
			must(err)
			pid, err := pidfile.New(c.PidFile)
			must(err)
			defer pid.Remove()
			tcpListeners, websockets, err := GetListeners(c)
			must(err)
			l, err := c.GetLogger(c.Log)
			must(err)
			logger = l
			if useDefault {
				l.Warn("config file not exist, use default configration")
			}
			// subscribe failure test
			// TODO remove this
			h := server.Hooks{
				OnSubscribe: func(ctx context.Context, client server.Client, subReq *server.SubscribeRequest) error {
					if sub := subReq.Subscriptions["test/nosubscribe"]; sub != nil {
						sub.Error = codes.NewError(packets.SubscribeFailure)
					}
					return nil

				},
			}
			s := server.New(
				server.WithConfig(c),
				server.WithTCPListener(tcpListeners...),
				server.WithWebsocketServer(websockets...),
				//gmqtt.WithPlugin(management.New(":8081", nil)),
				//server.WithPlugin(prometheus.New(&http.Server{
				//	Addr: c.Plugins.Prometheus.ListenAddress,
				//}, c.Plugins.Prometheus.Path)),
				server.WithLogger(l),
				server.WithHook(h),
			)
			err = s.Run()
			if err != nil {
				fmt.Println(err)
				return
			}
			installSignal(s)
		},
	}
	return cmd
}
