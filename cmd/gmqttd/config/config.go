package config

import (
	"crypto/tls"
	"io/ioutil"
	"net"
	"net/http"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/yaml.v2"

	"github.com/DrmagicE/gmqtt/plugin/prometheus"
	"github.com/DrmagicE/gmqtt/server"
)

// DefaultConfig is the default configration. If config file is not provided, gmqttd will start with DefaultConfig.
// Command-line flags will override the configuration.
var DefaultConfig = Config{
	Listeners: DefaultListeners,
	MQTT:      server.DefaultConfig,
	Log: LogConfig{
		Level: "info",
	},
	PidFile: getDefaultPidFile(),
	Plugins: PluginsConfig{
		Prometheus: prometheus.DefaultConfig,
	},
}
var DefaultListeners = []*ListenerConfig{
	{
		Address:    "0.0.0.0:1883",
		TLSOptions: nil,
		Websocket:  nil,
	},
}

// LogConfig is use to configure the log behaviors.
type LogConfig struct {
	Level string
}

// Config is the configration for gmqttd.
type Config struct {
	Listeners []*ListenerConfig `yaml:"listeners"`
	MQTT      server.Config     `yaml:"mqtt"`
	Log       LogConfig         `yaml:"log"`
	PidFile   string            `yaml:"pid_file"`
	Plugins   PluginsConfig     `yaml:"plugins"`
}
type PluginsConfig struct {
	Prometheus prometheus.Config `yaml:"prometheus"`
}
type TLSOptions struct {
	CertFile string `yaml:"cert_file" validate:"required"`
	KeyFile  string `yaml:"key_file" validate:"required"`
}
type ListenerConfig struct {
	Address string `yaml:"address" validate:"required,hostname_port"`
	*TLSOptions
	Websocket *WebsocketOptions `yaml:"websocket"`
}

type WebsocketOptions struct {
	Path string `yaml:"path" validate:"required"`
}

func (c *Config) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type config Config
	raw := config(DefaultConfig)
	if err := unmarshal(&raw); err != nil {
		return err
	}
	*c = Config(raw)
	return nil
}

func (c Config) Validate() error {
	return c.MQTT.Validate()
}

func ParseConfig(filePath string) (c Config, err error) {
	if filePath == "" {
		return DefaultConfig, nil
	}
	b, err := ioutil.ReadFile(filePath)
	if err != nil {
		return c, err
	}
	err = yaml.Unmarshal(b, &c)
	if err != nil {
		return c, err
	}
	err = c.Validate()
	if err != nil {
		return Config{}, err
	}
	return c, err
}

func (c Config) GetListeners() (tcpListeners []net.Listener, websockets []*server.WsServer, err error) {
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

func (c Config) GetLogger(config LogConfig) (l *zap.Logger, err error) {
	var logLevel zapcore.Level
	err = logLevel.UnmarshalText([]byte(config.Level))
	if err != nil {
		return
	}
	lc := zap.NewDevelopmentConfig()
	lc.Level = zap.NewAtomicLevelAt(logLevel)
	l, err = lc.Build()
	return
}
