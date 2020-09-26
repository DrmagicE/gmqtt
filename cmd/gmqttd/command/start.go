package command

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/DrmagicE/gmqtt/cmd/gmqttd/config"
	"github.com/DrmagicE/gmqtt/pkg/pidfile"
	"github.com/DrmagicE/gmqtt/plugin/prometheus"
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
				c = config.DefaultConfig
			} else {
				logger.Error("reload error", zap.Error(err))

			}
			srv.ApplyConfig(c.MQTT)
			logger.Info("gmqtt reloaded")
		case <-stopSignalCh:
			srv.Stop(context.Background())
			return
		}
	}

}

// NewStartCmd creates a *cobra.Command object for start command.
func NewStartCmd() *cobra.Command {
	cfg := &config.DefaultConfig
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
				c = *cfg
				useDefault = true
			} else {
				must(err)
			}
			err = c.Validate()
			must(err)
			pid, err := pidfile.New(c.PidFile)
			must(err)
			defer pid.Remove()
			tcpListeners, websockets, err := c.GetListeners()
			must(err)
			l, err := c.GetLogger(c.Log)
			must(err)
			logger = l
			if useDefault {
				l.Warn("config file not exist, use default configration")
			}
			s := server.New(
				server.WithConfig(c.MQTT),
				server.WithTCPListener(tcpListeners...),
				server.WithWebsocketServer(websockets...),
				//gmqtt.WithPlugin(management.New(":8081", nil)),
				server.WithPlugin(prometheus.New(&http.Server{
					Addr: c.Plugins.Prometheus.ListenAddress,
				}, c.Plugins.Prometheus.Path)),
				server.WithLogger(l),
			)
			s.Run()
			installSignal(s)
		},
	}
	return cmd
}
