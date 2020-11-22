package persistence

import (
	"testing"

	"github.com/stretchr/testify/assert"

	queue_test "github.com/DrmagicE/gmqtt/persistence/queue/test"
	sub_test "github.com/DrmagicE/gmqtt/persistence/subscription/test"
)

func TestMemoryQueue(t *testing.T) {
	a := assert.New(t)
	m := &memoryFactory{}
	p, err := m.New(queue_test.TestServerConfig, queue_test.TestHooks)

	a.Nil(err)
	qs, err := p.NewQueueStore(queue_test.TestServerConfig, queue_test.TestClient)
	a.Nil(err)
	queue_test.TestQueue(t, qs)

	s, err := p.NewSubscriptionStore(queue_test.TestServerConfig)
	a.Nil(err)
	sub_test.TestSuite(t, s)
}
