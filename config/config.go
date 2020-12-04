package config

import (
	"io/ioutil"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/yaml.v2"
)

// DefaultConfig is the default configration. If config file is not provided, gmqttd will start with DefaultConfig.
// Command-line flags will override the configuration.
var DefaultConfig = Config{
	Listeners: DefaultListeners,
	MQTT:      DefaultMQTTConfig,
	Log: LogConfig{
		Level: "info",
	},
	PidFile: getDefaultPidFile(),
	//Plugins: PluginsConfig{
	//	Prometheus: prometheus.DefaultConfig,
	//},
	Persistence:       DefaultPersistenceConfig,
	TopicAliasManager: DefaultTopicAliasManager,
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
	MQTT      MQTT              `yaml:"mqtt"`
	Log       LogConfig         `yaml:"log"`
	PidFile   string            `yaml:"pid_file"`
	//Plugins     PluginsConfig     `yaml:"plugins"`
	Persistence       Persistence       `yaml:"persistence"`
	TopicAliasManager TopicAliasManager `yaml:"topic_alias_manager"`
}

//type PluginsConfig struct {
//	Prometheus prometheus.Config `yaml:"prometheus"`
//}
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
	err := c.MQTT.Validate()
	if err != nil {
		return err
	}
	return c.Persistence.Validate()
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
