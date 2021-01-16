package federation

import (
	"errors"
	"fmt"
	"net"
)

// Config is the configuration for the federation plugin.
type Config struct {
	// NodeName is the unique identifier for the node in the federation.
	NodeName string `yaml:"node_name"`
	// FedAddr is the gRPC server listening address for the federation internal communication.
	FedAddr string `yaml:"fed_addr"`
	// GossipAddr is the address that the gossip will listen on, It is used for both UDP and TCP gossip.
	GossipAddr string   `yaml:"gossip_addr"`
	Join       []string `yaml:"join"`
}

// Validate validates the configuration, and return an error if it is invalid.
func (c *Config) Validate() error {
	if c.NodeName == "" {
		return errors.New("node_name must be set")
	}
	_, _, err := net.SplitHostPort(c.FedAddr)
	if err != nil {
		return fmt.Errorf("invalid fed_addr: %s", err)
	}
	_, _, err = net.SplitHostPort(c.GossipAddr)
	if err != nil {
		return fmt.Errorf("invalid gossip_addr: %s", err)
	}
	for _, v := range c.Join {
		_, _, err = net.SplitHostPort(v)
		if err != nil {
			return fmt.Errorf("invalid join: %s", err)
		}
	}
	return nil
}

// DefaultConfig is the default configuration.
var DefaultConfig = Config{}

func (c *Config) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type cfg Config
	var v = &struct {
		Federation cfg `yaml:"federation"`
	}{
		Federation: cfg(DefaultConfig),
	}
	if err := unmarshal(v); err != nil {
		return err
	}
	*c = Config(v.Federation)
	return nil
}
