package config

import (
	"net"
	"time"

	"github.com/pkg/errors"
)

type PersistenceType = string

const (
	PersistenceTypeMemory PersistenceType = "memory"
	PersistenceTypeRedis  PersistenceType = "redis"
)

var (
	defaultMaxActive = uint(0)
	defaultMaxIdle   = uint(1000)
	// DefaultPersistenceConfig is the default value of Persistence
	DefaultPersistenceConfig = Persistence{
		Type: PersistenceTypeMemory,
		Redis: RedisPersistence{
			Addr:        "127.0.0.1:6379",
			Password:    "",
			Database:    0,
			MaxIdle:     &defaultMaxIdle,
			MaxActive:   &defaultMaxActive,
			IdleTimeout: 240 * time.Second,
		},
	}
)

// Persistence is the config of backend persistence.
type Persistence struct {
	// Type is the persistence type.
	// If empty, use "memory" as default.
	Type PersistenceType `yaml:"type"`
	// Redis is the redis configuration and must be set when Type ==  "redis".
	Redis RedisPersistence `yaml:"redis"`
}

// RedisPersistence is the configuration of redis persistence.
type RedisPersistence struct {
	// Addr is the redis server address.
	// If empty, use "127.0.0.1:6379" as default.
	Addr string `yaml:"addr"`
	// Password is the redis password.
	Password string `yaml:"password"`
	// Database is the number of the redis database to be connected.
	Database uint `yaml:"database"`
	// MaxIdle is the maximum number of idle connections in the pool.
	// If nil, use 1000 as default.
	// This value will pass to redis.Pool.MaxIde.
	MaxIdle *uint `yaml:"max_idle"`
	// MaxActive is the maximum number of connections allocated by the pool at a given time.
	// If nil, use 0 as default.
	// If zero, there is no limit on the number of connections in the pool.
	// This value will pass to redis.Pool.MaxActive.
	MaxActive *uint `yaml:"max_active"`
	// Close connections after remaining idle for this duration. If the value
	// is zero, then idle connections are not closed. Applications should set
	// the timeout to a value less than the server's timeout.
	// Ff zero, use 240 * time.Second as default.
	// This value will pass to redis.Pool.IdleTimeout.
	IdleTimeout time.Duration `yaml:"idle_timeout"`
}

func (p *Persistence) Validate() error {
	if p.Type != PersistenceTypeMemory && p.Type != PersistenceTypeRedis {
		return errors.New("invalid persistence type")
	}
	_, _, err := net.SplitHostPort(p.Redis.Addr)
	if err != nil {
		return err
	}
	if p.Redis.Database < 0 {
		return errors.New("invalid redis database number")
	}
	return nil
}
