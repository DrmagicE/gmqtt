package run

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v2"
	"io/ioutil"
)

// ProtocolMQTT represents the MQTT protocol under TCP server
const ProtocolMQTT = "mqtt"

// ProtocolWebsocket represents the MQTT protocol under websocket server.
const ProtocolWebsocket = "websocket"

//Default configration
const (
	DefaultDeliveryRetryInterval = 20
	DefaultQueueQos0Messages     = true
	DefaultMaxInflightMessages   = 20
	DefaultLogging               = false
	DefaultMaxMsgQueueMessages   = 2048
)

// Config represents the configuration options.
type Config struct {
	DeliveryRetryInterval int64            `yaml:"delivery_retry_interval"`
	QueueQos0Messages     bool             `yaml:"queue_qos0_messages"`
	MaxInflightMessages   int              `yaml:"max_inflight_messages"`
	MaxMsgQueueMessages   int              `yaml:"max_msgqueue_messages"`
	ProfileConfig         ProfileConfig    `yaml:"profile"`
	Listener              []ListenerConfig `yaml:"listener,flow"`
	Logging               bool             `yaml:"logging"`
	HttpServerConfig      HttpServerConfig `yaml:"http_server"`
}

// ProfileConfig represents the server profile configuration.
type ProfileConfig struct {
	CPUProfile string `yaml:"cpu"`
	MemProfile string `yaml:"mem"`
}

// HttpServerConfig represents the REST server configuration.
type HttpServerConfig struct {
	Addr string       `yaml:"addr"`
	User gin.Accounts `yaml:"user"`
}

// ListenerConfig represents the tcp server configuration.
type ListenerConfig struct {
	Protocol string `yaml:"protocol"`
	Addr     string `yaml:"addr"`
	CertFile string `yaml:"certfile"`
	KeyFile  string `yaml:"keyfile"`
}

// Validate validate the Config, returns error if it is invalid.
func (c *Config) Validate() error {
	for _, v := range c.Listener {
		if v.Protocol != ProtocolMQTT && v.Protocol != ProtocolWebsocket {
			return fmt.Errorf("invalid protocol name '%s',expect 'mqtt' or 'websocket'", v.Protocol)
		}
		if v.KeyFile != "" && v.CertFile == "" {
			return fmt.Errorf("invalid tls/ssl configration, 'certfile missing'")
		}
		if v.KeyFile == "" && v.CertFile != "" {
			return fmt.Errorf("invalid tls/ssl configration, 'keyfile' missing")
		}
		if v.Addr == "" {
			return fmt.Errorf("addr missing")
		}
	}
	return nil
}

// NewConfig returns the default Config instance.
func NewConfig() *Config {
	return &Config{
		DeliveryRetryInterval: DefaultDeliveryRetryInterval,
		QueueQos0Messages:     DefaultQueueQos0Messages,
		MaxInflightMessages:   DefaultMaxInflightMessages,
		MaxMsgQueueMessages:   DefaultMaxMsgQueueMessages,
		Logging:               DefaultLogging,
	}
}

// FromConfigFile loads the configuration from a yaml  file
func (c *Config) FromConfigFile(fpath string) error {
	bs, err := ioutil.ReadFile(fpath)
	if err != nil {
		return err
	}
	err = yaml.Unmarshal(bs, c)
	if err != nil {
		return err
	}
	if len(c.Listener) == 0 {
		c.Listener = make([]ListenerConfig, 1)
		c.Listener[0].Protocol = ProtocolMQTT
		c.Listener[0].Addr = ":1883"
	}
	return nil
}
