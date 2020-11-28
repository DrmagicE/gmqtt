package server

import (
	"github.com/DrmagicE/gmqtt/persistence/queue"
	"github.com/DrmagicE/gmqtt/persistence/session"
	"github.com/DrmagicE/gmqtt/persistence/subscription"
	"github.com/DrmagicE/gmqtt/persistence/unack"
)

type Persistence interface {
	Open() error
	NewQueueStore(config Config, clientID string) (queue.Store, error)
	NewSubscriptionStore(config Config) (subscription.Store, error)
	NewSessionStore(config Config) (session.Store, error)
	NewUnackStore(config Config, clientID string) (unack.Store, error)
	Close() error
}

type PersistenceFactory interface {
	New(config Config, hooks Hooks) (Persistence, error)
}
