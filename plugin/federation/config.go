package federation

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultFedAddr       = ":8901"
	DefaultGossipAddr    = ":8902"
	DefaultRetryInterval = 5 * time.Second
	DefaultRetryTimeout  = 1 * time.Minute
)

// Config is the configuration for the federation plugin.
type Config struct {
	// NodeName is the unique identifier for the node in the federation. Defaults to hostname.
	NodeName string `yaml:"node_name"`
	// FedAddr is the gRPC server listening address for the federation internal communication.
	// Defaults to :8901
	// If the port is missing, the default federation port (8901) will be used.
	FedAddr string `yaml:"fed_addr"`
	// AdvertiseFedAddr is used to change the federation gRPC server address that we advertise to other nodes in the cluster.
	// Defaults to "FedAddr".However, in some cases, there may be a routable address that cannot be bound.
	// If the port is missing, the default federation port (8901) will be used.
	AdvertiseFedAddr string `yaml:"advertise_fed_addr"`
	// GossipAddr is the address that the gossip will listen on, It is used for both UDP and TCP gossip. Defaults to :8902
	GossipAddr string `yaml:"gossip_addr"`
	// AdvertiseGossipAddr is used to change the gossip server address that we advertise to other nodes in the cluster.
	// Defaults to "GossipAddr".
	// If the port is missing, the default gossip port (8902) will be used.
	AdvertiseGossipAddr string `yaml:"advertise_gossip_addr"`
	// RetryJoin is the address of other nodes to join upon starting up.
	// If port is missing, the default gossip port (8902) will be used.
	RetryJoin []string `yaml:"retry_join"`
	// RetryInterval is the time to wait between join attempts. Defaults to 5s.
	RetryInterval time.Duration `yaml:"retry_interval"`
	// RetryTimeout is the timeout to wait before joining all nodes in RetryJoin successfully.
	// If timeout expires, the server will exit with error. Defaults to 1m.
	RetryTimeout time.Duration `yaml:"retry_timeout"`
	// SnapshotPath will be pass to "SnapshotPath" in serf configuration.
	// When Serf is started with a snapshot,
	// it will attempt to join all the previously known nodes until one
	// succeeds and will also avoid replaying old user events.
	SnapshotPath string `yaml:"snapshot_path"`
	// RejoinAfterLeave will be pass to "RejoinAfterLeave" in serf configuration.
	// It controls our interaction with the snapshot file.
	// When set to false (default), a leave causes a Serf to not rejoin
	// the cluster until an explicit join is received. If this is set to
	// true, we ignore the leave, and rejoin the cluster on start.
	RejoinAfterLeave bool `yaml:"rejoin_after_leave"`
}

func isPortNumber(port string) bool {
	i, err := strconv.Atoi(port)
	if err != nil {
		return false
	}
	if 1 <= i && i <= 65535 {
		return true
	}
	return false
}

// joinHostPort returns a network address of the form "host:port".
// If the addr does not contains "port", the function will add defaultPort to it.
// Note that this function does not guarantee the correctness of the returned address.
func joinHostPort(addr string, defaultPort string) (newAddr string) {
	portIndex := strings.LastIndex(addr, ":")
	if portIndex == -1 {
		return addr + defaultPort
	}
	if len(addr) == portIndex+1 {
		return addr
	}
	// IPv6
	if addr[0] == '[' && !isPortNumber(addr[portIndex+1:]) {
		return addr + defaultPort
	}
	return addr
}

func validAddrAndSet(addr string, defaultPort string, fieldName string, set func(addr string)) error {
	fedAddr := joinHostPort(addr, defaultPort)
	_, port, err := net.SplitHostPort(fedAddr)
	if err != nil {
		return fmt.Errorf("invalid %s: %s", fieldName, err)
	}
	if !isPortNumber(port) {
		return fmt.Errorf("invalid port number: %s", addr)
	}
	set(fedAddr)
	return nil
}

// Validate validates the configuration, and return an error if it is invalid.
func (c *Config) Validate() (err error) {
	if c.NodeName == "" {
		hostName, err := os.Hostname()
		if err != nil {
			return err
		}
		c.NodeName = hostName
	}
	err = validAddrAndSet(c.FedAddr, DefaultFedAddr, "fed_addr", func(addr string) {
		c.FedAddr = addr
	})
	if err != nil {
		return err
	}
	err = validAddrAndSet(c.GossipAddr, DefaultGossipAddr, "gossip_addr", func(addr string) {
		c.GossipAddr = addr
	})
	if err != nil {
		return err
	}
	if c.AdvertiseFedAddr == "" {
		c.AdvertiseFedAddr = c.FedAddr
	}
	err = validAddrAndSet(c.AdvertiseFedAddr, DefaultFedAddr, "advertise_fed_addr", func(addr string) {
		c.AdvertiseFedAddr = addr
	})
	if err != nil {
		return err
	}

	if c.AdvertiseGossipAddr == "" {
		c.AdvertiseGossipAddr = c.GossipAddr
	}
	err = validAddrAndSet(c.AdvertiseGossipAddr, DefaultGossipAddr, "advertise_gossip_addr", func(addr string) {
		c.AdvertiseGossipAddr = addr
	})
	if err != nil {
		return err
	}

	for k, v := range c.RetryJoin {
		err = validAddrAndSet(v, DefaultGossipAddr, "retry_join", func(addr string) {
			c.RetryJoin[k] = addr
		})
		if err != nil {
			return err
		}
	}
	if c.RetryInterval <= 0 {
		return fmt.Errorf("invalid retry_join: %d", c.RetryInterval)
	}

	if c.RetryTimeout <= 0 {
		return fmt.Errorf("invalid retry_timeout: %d", c.RetryTimeout)
	}
	return nil
}

// DefaultConfig is the default configuration.
var DefaultConfig = Config{}

func init() {
	hostName, err := os.Hostname()
	if err != nil {
		panic(err)
	}
	DefaultConfig = Config{
		NodeName:      hostName,
		FedAddr:       DefaultFedAddr,
		GossipAddr:    DefaultGossipAddr,
		RetryJoin:     nil,
		RetryInterval: DefaultRetryInterval,
		RetryTimeout:  DefaultRetryTimeout,
	}
}

func (c *Config) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type cfg Config
	df := cfg(DefaultConfig)
	var v = &struct {
		Federation *cfg `yaml:"federation"`
	}{
		Federation: &df,
	}
	if err := unmarshal(v); err != nil {
		return err
	}
	if v.Federation == nil {
		v.Federation = &df
	}
	*c = Config(*v.Federation)
	return nil
}
