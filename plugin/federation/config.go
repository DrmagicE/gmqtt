package federation

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/go-sockaddr"
)

// Default config.
const (
	DefaultFedPort       = "8901"
	DefaultGossipPort    = "8902"
	DefaultRetryInterval = 5 * time.Second
	DefaultRetryTimeout  = 1 * time.Minute
)

// stub function for testing
var getPrivateIP = sockaddr.GetPrivateIP

// Config is the configuration for the federation plugin.
type Config struct {
	// NodeName is the unique identifier for the node in the federation. Defaults to hostname.
	NodeName string `yaml:"node_name"`
	// FedAddr is the gRPC server listening address for the federation internal communication.
	// Defaults to :8901.
	// If the port is missing, the default federation port (8901) will be used.
	FedAddr string `yaml:"fed_addr"`
	// AdvertiseFedAddr is used to change the federation gRPC server address that we advertise to other nodes in the cluster.
	// Defaults to "FedAddr" or the private IP address of the node if the IP in "FedAddr" is 0.0.0.0.
	// However, in some cases, there may be a routable address that cannot be bound.
	// If the port is missing, the default federation port (8901) will be used.
	AdvertiseFedAddr string `yaml:"advertise_fed_addr"`
	// GossipAddr is the address that the gossip will listen on, It is used for both UDP and TCP gossip. Defaults to :8902
	GossipAddr string `yaml:"gossip_addr"`
	// AdvertiseGossipAddr is used to change the gossip server address that we advertise to other nodes in the cluster.
	// Defaults to "GossipAddr" or the private IP address of the node if the IP in "GossipAddr" is 0.0.0.0.
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

func getAddr(addr string, defaultPort string, fieldName string, usePrivate bool) (string, error) {
	if addr == "" {
		return "", fmt.Errorf("missing %s", fieldName)
	}
	host, port, err := net.SplitHostPort(addr)
	if port == "" {
		port = defaultPort
	}
	if addr[len(addr)-1] == ':' {
		return "", fmt.Errorf("invalid %s", fieldName)
	}
	if err != nil && strings.Contains(err.Error(), "missing port in address") {
		host, port, err = net.SplitHostPort(addr + ":" + defaultPort)
		if err != nil {
			return "", fmt.Errorf("invalid %s: %s", fieldName, err)
		}
	} else if err != nil {
		return "", fmt.Errorf("invalid %s: %s", fieldName, err)
	}
	if usePrivate && (host == "0.0.0.0" || host == "") {
		host, err = getPrivateIP()
		if err != nil {
			return "", err
		}
	}
	if !isPortNumber(port) {
		return "", fmt.Errorf("invalid port number: %s", port)
	}
	return net.JoinHostPort(host, port), nil
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
	c.FedAddr, err = getAddr(c.FedAddr, DefaultFedPort, "fed_addr", false)
	if err != nil {
		return err
	}
	c.GossipAddr, err = getAddr(c.GossipAddr, DefaultGossipPort, "gossip_addr", false)
	if err != nil {
		return err
	}
	if c.AdvertiseFedAddr == "" {
		c.AdvertiseFedAddr = c.FedAddr
	}
	c.AdvertiseFedAddr, err = getAddr(c.AdvertiseFedAddr, DefaultFedPort, "advertise_fed_addr", true)
	if err != nil {
		return err
	}
	if c.AdvertiseGossipAddr == "" {
		c.AdvertiseGossipAddr = c.GossipAddr
	}
	c.AdvertiseGossipAddr, err = getAddr(c.AdvertiseGossipAddr, DefaultGossipPort, "advertise_gossip_addr", true)
	if err != nil {
		return err
	}

	for k, v := range c.RetryJoin {
		c.RetryJoin[k], err = getAddr(v, DefaultGossipPort, "retry_join", false)
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
		FedAddr:       ":" + DefaultFedPort,
		GossipAddr:    ":" + DefaultFedPort,
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
