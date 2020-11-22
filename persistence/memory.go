package persistence

import (
	"github.com/DrmagicE/gmqtt/persistence/queue"
	mem_queue "github.com/DrmagicE/gmqtt/persistence/queue/mem"
	"github.com/DrmagicE/gmqtt/persistence/subscription"
	mem_sub "github.com/DrmagicE/gmqtt/persistence/subscription/mem"
	"github.com/DrmagicE/gmqtt/server"
)

func init() {
	server.RegisterPersistenceFactory("memory", &memoryFactory{})
}

type memoryFactory struct {
	config server.Config
}

func (m *memoryFactory) New(config server.Config, hooks server.Hooks) (server.Persistence, error) {
	return &memory{
		onMsgDropped: hooks.OnMsgDropped,
	}, nil
}

type memory struct {
	onMsgDropped server.OnMsgDropped
}

func (m *memory) Open() error {
	return nil
}
func (m *memory) NewQueueStore(config server.Config, client server.Client) (queue.Store, error) {
	return mem_queue.New(config, client, m.onMsgDropped)
}

func (m *memory) NewSubscriptionStore(config server.Config) (subscription.Store, error) {
	return mem_sub.NewStore(), nil
}

func (m *memory) Close() error {
	return nil
}
