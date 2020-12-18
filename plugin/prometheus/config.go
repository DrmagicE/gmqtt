package prometheus

import (
	"errors"
	"net"
)

// Config is the configuration for the prometheus plugin.
type Config struct {
	// ListenAddress is the address that the exporter will listen on.
	ListenAddress string `yaml:"listen_address"`
	// Path is the exporter url path.
	Path string `yaml:"path"`
}

// Validate validates the configuration, and return an error if it is invalid.
func (c *Config) Validate() error {
	_, _, err := net.SplitHostPort(c.ListenAddress)
	if err != nil {
		return errors.New("invalid listen_address")
	}
	return nil
}

// DefaultConfig is the default configuration.
var DefaultConfig = Config{
	ListenAddress: ":8082",
	Path:          "/metrics",
}

func (c *Config) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type cfg Config
	var v = &struct {
		Prometheus cfg `yaml:"prometheus"`
	}{
		Prometheus: cfg(DefaultConfig),
	}
	if err := unmarshal(v); err != nil {
		return err
	}
	empty := cfg(Config{})
	if v.Prometheus == empty {
		v.Prometheus = cfg(DefaultConfig)
	}
	*c = Config(v.Prometheus)
	return nil
}
