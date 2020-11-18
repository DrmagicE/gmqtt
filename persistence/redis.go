package persistence

import (
	"time"

	redigo "github.com/gomodule/redigo/redis"

	"github.com/DrmagicE/gmqtt/persistence/queue"
	redis_queue "github.com/DrmagicE/gmqtt/persistence/queue/redis"
	"github.com/DrmagicE/gmqtt/server"
	"github.com/DrmagicE/gmqtt/subscription"
)

func init() {
	server.RegisterPersistenceFactory("redis", &redisFactory{})
}

type NewQueueStore func(config server.Config, client server.Client) (queue.Store, error)

type redisFactory struct {
	config server.Config
}

func (r *redisFactory) New(config server.Config, hooks server.Hooks) (server.Persistence, error) {
	return &redis{
		onMsgDropped: hooks.OnMsgDropped,
	}, nil
}

type redis struct {
	pool         *redigo.Pool
	onMsgDropped server.OnMsgDropped
}

func newPool(addr string) *redigo.Pool {
	return &redigo.Pool{
		MaxIdle:     3,
		IdleTimeout: 240 * time.Second,
		// Dial or DialContext must be set. When both are set, DialContext takes precedence over Dial.
		Dial: func() (redigo.Conn, error) { return redigo.Dial("tcp", addr) },
	}
}
func (r *redis) Open() error {
	// TODO read from config
	r.pool = newPool(":6379")
	conn := r.pool.Get()
	defer conn.Close()
	// Test the connection
	_, err := conn.Do("PING")
	return err
}
func (r *redis) NewQueueStore(config server.Config, client server.Client) (queue.Store, error) {
	return redis_queue.New(config, client, r.pool, r.onMsgDropped)
}

func (r *redis) NewSubscriptionStore(config server.Config) subscription.Store {
	panic("implement me")
}

func (r *redis) Close() error {
	return r.pool.Close()
}
