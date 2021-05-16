package persistence

import (
	redigo "github.com/gomodule/redigo/redis"

	"github.com/DrmagicE/gmqtt/config"
	"github.com/DrmagicE/gmqtt/persistence/queue"
	redis_queue "github.com/DrmagicE/gmqtt/persistence/queue/redis"
	"github.com/DrmagicE/gmqtt/persistence/session"
	redis_sess "github.com/DrmagicE/gmqtt/persistence/session/redis"
	"github.com/DrmagicE/gmqtt/persistence/subscription"
	redis_sub "github.com/DrmagicE/gmqtt/persistence/subscription/redis"
	"github.com/DrmagicE/gmqtt/persistence/unack"
	redis_unack "github.com/DrmagicE/gmqtt/persistence/unack/redis"
	"github.com/DrmagicE/gmqtt/server"
)

func init() {
	server.RegisterPersistenceFactory("redis", NewRedis)
}

func NewRedis(config config.Config) (server.Persistence, error) {
	return &redis{
		config: config,
	}, nil
}

type redis struct {
	pool         *redigo.Pool
	config       config.Config
	onMsgDropped server.OnMsgDropped
}

func (r *redis) NewUnackStore(config config.Config, clientID string) (unack.Store, error) {
	return redis_unack.New(redis_unack.Options{
		ClientID: clientID,
		Pool:     r.pool,
	}), nil
}

func (r *redis) NewSessionStore(config config.Config) (session.Store, error) {
	return redis_sess.New(r.pool), nil
}

func newPool(config config.Config) *redigo.Pool {
	return &redigo.Pool{
		// Dial or DialContext must be set. When both are set, DialContext takes precedence over Dial.
		Dial: func() (redigo.Conn, error) {
			c, err := redigo.Dial("tcp", config.Persistence.Redis.Addr)
			if err != nil {
				return nil, err
			}
			if pswd := config.Persistence.Redis.Password; pswd != "" {
				if _, err := c.Do("AUTH", pswd); err != nil {
					c.Close()
					return nil, err
				}
			}
			if _, err := c.Do("SELECT", config.Persistence.Redis.Database); err != nil {
				c.Close()
				return nil, err
			}
			return c, nil
		},
	}
}
func (r *redis) Open() error {
	r.pool = newPool(r.config)
	r.pool.MaxIdle = int(*r.config.Persistence.Redis.MaxIdle)
	r.pool.MaxActive = int(*r.config.Persistence.Redis.MaxActive)
	r.pool.IdleTimeout = r.config.Persistence.Redis.IdleTimeout
	conn := r.pool.Get()
	defer conn.Close()
	// Test the connection
	_, err := conn.Do("PING")

	return err
}

func (r *redis) NewQueueStore(config config.Config, defaultNotifier queue.Notifier, clientID string) (queue.Store, error) {
	return redis_queue.New(redis_queue.Options{
		MaxQueuedMsg:    config.MQTT.MaxQueuedMsg,
		InflightExpiry:  config.MQTT.InflightExpiry,
		ClientID:        clientID,
		Pool:            r.pool,
		DefaultNotifier: defaultNotifier,
	})
}

func (r *redis) NewSubscriptionStore(config config.Config) (subscription.Store, error) {
	return redis_sub.New(r.pool), nil
}

func (r *redis) Close() error {
	return r.pool.Close()
}
