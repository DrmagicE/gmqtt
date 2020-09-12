package command

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"go.uber.org/zap"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/cmd/gmqttd/config"
	"github.com/DrmagicE/gmqtt/pkg/pidfile"
)

type NoopFlag struct {
}

func (NoopFlag) String() string {
	return ""
}
func (NoopFlag) Set(string) error {
	return nil
}
func (NoopFlag) Type() string {
	return ""
}

var (
	configFile string
	// Global logger
	logger *zap.Logger
)

func must(err error) {
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func rootFlagSet() *pflag.FlagSet {
	fs := pflag.NewFlagSet("root", pflag.ExitOnError)
	d, err := homedir.Dir()
	must(err)
	fs.StringVar(&configFile, "config", d+"/gmqttd.yml", "The configration file path")
	return fs

}

func installSignal(srv gmqtt.Server) {
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
			c, err = config.ParseConfig(configFile)
			if os.IsNotExist(err) {
				// if config file not exist, use default configration.
				c = config.DefaultConfig
			} else {
				// TODO log error
			}
			srv.ApplyConfig(c.MQTT)
			fmt.Println("reload")
		case <-stopSignalCh:
			srv.Stop(context.Background())
			return
		}
	}

}

// NewStartCmd creates a *cobra.Command object for start command.
func NewStartCmd() *cobra.Command {
	cfg := &config.DefaultConfig
	var addresses []string
	rfs := rootFlagSet()
	startFlagSet := pflag.NewFlagSet("start", pflag.ExitOnError)
	startFlagSet.StringSliceVar(&addresses, "address", []string{"0.0.0.0:1883"}, "The TCP listening address of the broker")
	cfg.AddFlags(startFlagSet)
	rfs.AddFlagSet(startFlagSet)
	cmd := &cobra.Command{
		Use:                "start",
		Short:              "Start gmqtt broker",
		DisableFlagParsing: true,
		Run: func(cmd *cobra.Command, args []string) {
			var err error
			err = rfs.Parse(args)
			must(err)
			c, err := config.ParseConfig(configFile)
			if os.IsNotExist(err) {
				// if config file not exist, use default configration.
				c = *cfg
			} else {
				must(err)
			}
			fs := pflag.NewFlagSet("", pflag.ExitOnError)
			rootFlagSet().VisitAll(func(f *pflag.Flag) {
				fs.VarP(NoopFlag{}, f.Name, f.Shorthand, f.Usage)
			})
			c.AddFlags(fs)
			err = fs.Parse(args)
			must(err)
			err = c.Validate()
			must(err)

			pid, err := pidfile.New(c.PidFile)
			must(err)
			defer pid.Remove()

			tcpListeners, websockets, err := c.GetListeners()
			must(err)
			l, err := c.GetLogger(c.Log)
			must(err)
			s := gmqtt.NewServer(
				gmqtt.WithConfig(c.MQTT),
				gmqtt.WithTCPListener(tcpListeners...),
				gmqtt.WithWebsocketServer(websockets...),
				//gmqtt.WithPlugin(management.New(":8081", nil)),
				//gmqtt.WithPlugin(prometheus.New(&http.Server{
				//	Addr: ":8082",
				//}, "/metrics")),
				gmqtt.WithLogger(l),
			)
			fmt.Println(c.MQTT)
			s.Run()
			installSignal(s)
		},
	}
	return cmd
}
