package config

import (
	"crypto/tls"
	"io/ioutil"
	"net"
	"net/http"

	"github.com/spf13/pflag"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/yaml.v2"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/plugin/prometheus"
)

// DefaultConfig is the default configration. If config file is not provided, gmqttd will start with DefaultConfig.
// Command-line flags will override the configuration.
var DefaultConfig = Config{
	Listeners: DefaultListeners,
	MQTT:      gmqtt.DefaultConfig,
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
	MQTT      gmqtt.Config      `yaml:"mqtt"`
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

func (c *Config) AddFlags(fs *pflag.FlagSet) {
	fs.DurationVar(&c.MQTT.SessionExpiry, "mqtt.session-expiry", c.MQTT.SessionExpiry, "The maximum session timeout duration in seconds")
	fs.DurationVar(&c.MQTT.SessionExpiryCheckInterval, "mqtt.session-expiry-check-interval", c.MQTT.SessionExpiry, "The interval of session expiry check.")
	fs.DurationVar(&c.MQTT.MessageExpiry, "mqtt.message-expiry", c.MQTT.MessageExpiry, "The maximum message expiry duration in seconds")
	fs.Uint32Var(&c.MQTT.MaxPacketSize, "mqtt.max-packet-size", c.MQTT.MaxPacketSize, "The maximum size of any MQTT packet in bytes that will be accepted by the broker. 0 means no limit")
	fs.Uint16Var(&c.MQTT.ReceiveMax, "mqtt.server-receive-maximum", c.MQTT.ReceiveMax, "The maximum amount of PUBLISH messages, which have not yet been acknowledged by the broker")
	fs.Uint16Var(&c.MQTT.MaxKeepAlive, "mqtt.max-keepalive", c.MQTT.MaxKeepAlive, "The maximum value that the broker accepts in the keepAlive field in the CONNECT packet of a client")
	fs.Uint16Var(&c.MQTT.TopicAliasMax, "mqtt.topic-alias-maximum", c.MQTT.TopicAliasMax, "The amount of topic aliases available per client")
	fs.BoolVar(&c.MQTT.SubscriptionIDAvailable, "mqtt.subscription-identifier-available", c.MQTT.SubscriptionIDAvailable, "Enable subscription identifier")
	fs.BoolVar(&c.MQTT.WildcardAvailable, "mqtt.wildcard-subscription-available", c.MQTT.WildcardAvailable, "Enable wildcard subscription")
	fs.BoolVar(&c.MQTT.SharedSubAvailable, "mqtt.shared-subscription-available", c.MQTT.SharedSubAvailable, "Enable shared subscription")
	fs.Uint8Var(&c.MQTT.MaximumQoS, "mqtt.maximum-qos", c.MQTT.MaximumQoS, "The maximum qos of PUBLISH that is allowed by the broker")
	fs.BoolVar(&c.MQTT.RetainAvailable, "mqtt.retain-available", c.MQTT.RetainAvailable, "Enable retain message")
	fs.IntVar(&c.MQTT.MaxQueuedMsg, "mqtt.max-queued-messages", c.MQTT.MaxQueuedMsg, "The maximum length of the message queue")
	fs.IntVar(&c.MQTT.MaxInflight, "mqtt.max-inflight", c.MQTT.MaxInflight, "Inflight window size: The flight window is used to store unanswered QoS 1 and QoS 2 messages")
	fs.IntVar(&c.MQTT.MaxAwaitRel, "mqtt.max-awaiting", c.MQTT.MaxAwaitRel, "The maximum receiving window for QoS 2 messages")
	fs.BoolVar(&c.MQTT.QueueQos0Msg, "mqtt.queued-qos0-messages", c.MQTT.QueueQos0Msg, "Enable the message queue stores QoS 0 messages")
	fs.StringVar(&c.MQTT.DeliveryMode, "mqtt.delivery-mode", "onlyonce", `Set the delivery mode ("onlyonce"|"overlap")`)
	fs.BoolVar(&c.MQTT.AllowZeroLenClientID, "mqtt.allow-zero-length-clientid", c.MQTT.AllowZeroLenClientID, "Whether to allow zero clientId")
	fs.StringVar(&c.Log.Level, "log.level", c.Log.Level, `Set the logging level ("debug"|"info"|"warn"|"error"|"fatal")`)
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

func (c Config) GetListeners() (tcpListeners []net.Listener, websockets []*gmqtt.WsServer, err error) {
	for _, v := range c.Listeners {
		var ln net.Listener
		if v.Websocket != nil {
			ws := &gmqtt.WsServer{
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
	lc := zap.NewProductionConfig()
	lc.Level = zap.NewAtomicLevelAt(logLevel)
	l, err = lc.Build()
	return
}
