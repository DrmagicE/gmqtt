package config

import (
	"io/ioutil"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/yaml.v2"
)

var (
	defaultPluginConfig = make(map[string]Configuration)
)

type Configuration interface {
	Validate() error
	yaml.Unmarshaler
}
type Validate func(config Configuration) error

func RegisterDefaultPluginConfig(name string, config Configuration) {
	defaultPluginConfig[name] = config
}

// DefaultConfig return the default configuration.
// If config file is not provided, gmqttd will start with DefaultConfig.
// Command-line flags will override the configuration.
func DefaultConfig() Config {
	c := Config{
		Listeners: DefaultListeners,
		MQTT:      DefaultMQTTConfig,
		Log: LogConfig{
			Level: "info",
		},
		PidFile:           getDefaultPidFile(),
		Plugins:           make(PluginConfig),
		Persistence:       DefaultPersistenceConfig,
		TopicAliasManager: DefaultTopicAliasManager,
	}

	for name, v := range defaultPluginConfig {
		c.Plugins[name] = v
	}
	return c
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

type PluginConfig map[string]Configuration

func (p PluginConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	for _, v := range p {
		err := unmarshal(v)
		if err != nil {
			return err
		}
	}
	return nil
}

// Config is the configration for gmqttd.
type Config struct {
	Listeners         []*ListenerConfig `yaml:"listeners"`
	MQTT              MQTT              `yaml:"mqtt,omitempty"`
	Log               LogConfig         `yaml:"log"`
	PidFile           string            `yaml:"pid_file"`
	Plugins           PluginConfig      `yaml:"plugins"`
	Persistence       Persistence       `yaml:"persistence"`
	TopicAliasManager TopicAliasManager `yaml:"topic_alias_manager"`
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
	raw := config(DefaultConfig())
	if err := unmarshal(&raw); err != nil {
		return err
	}
	emptyMQTT := MQTT{}
	if raw.MQTT == emptyMQTT {
		raw.MQTT = DefaultMQTTConfig
	}
	if len(raw.Plugins) == 0 {
		raw.Plugins = make(PluginConfig)
		for name, v := range defaultPluginConfig {
			raw.Plugins[name] = v
		}
	} else {
		for name, v := range raw.Plugins {
			if v == nil {
				raw.Plugins[name] = defaultPluginConfig[name]
			}
		}
	}
	*c = Config(raw)
	return nil
}

func (c Config) Validate() error {
	err := c.MQTT.Validate()
	if err != nil {
		return err
	}
	err = c.Persistence.Validate()
	if err != nil {
		return err
	}
	for _, conf := range c.Plugins {
		err := conf.Validate()
		if err != nil {
			return err
		}
	}
	return nil
}

func ParseConfig(filePath string) (c Config, err error) {
	if filePath == "" {
		return DefaultConfig(), nil
	}
	b, err := ioutil.ReadFile(filePath)
	if err != nil {
		return c, err
	}
	c = DefaultConfig()
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

func (c Config) GetLogger(config LogConfig) (l *zap.Logger, err error) {
	var logLevel zapcore.Level
	err = logLevel.UnmarshalText([]byte(config.Level))
	if err != nil {
		return
	}

	lc := zap.NewDevelopmentConfig()
	lc.Level = zap.NewAtomicLevelAt(logLevel)
	l, err = lc.Build()
	return l, nil
}
