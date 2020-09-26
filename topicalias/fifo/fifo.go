package fifo

import (
	"container/list"
	"sync"

	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/DrmagicE/gmqtt/server"
)

// Queue is the fifo queue which store all topic alias
type Queue struct {
	mu sync.RWMutex
	// key by client id
	topicAlias map[string]*topicAlias
}
type topicAlias struct {
	max   int
	alias *list.List
	// topic name => alias
	index map[string]uint16
}
type aliasElem struct {
	topic string
	alias uint16
}

func (q *Queue) Create(client server.Client) {
	q.mu.Lock()
	defer q.mu.Unlock()
	opts := client.ClientOptions()
	q.topicAlias[opts.ClientID] = &topicAlias{
		max:   int(opts.ClientTopicAliasMax),
		alias: list.New(),
		index: make(map[string]uint16),
	}
}

func (q *Queue) Delete(client server.Client) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.topicAlias, client.ClientOptions().ClientID)
}

func (q *Queue) Check(client server.Client, publish *packets.Publish) (alias uint16, exist bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	opts := client.ClientOptions()
	al := q.topicAlias[opts.ClientID]
	topicName := string(publish.TopicName)

	// alias exist
	if a, ok := al.index[topicName]; ok {
		return a, true
	}
	l := al.alias.Len()
	// alias has been exhausted
	if l == al.max {
		first := al.alias.Front()
		elem := first.Value.(*aliasElem)
		al.alias.Remove(first)
		delete(al.index, elem.topic)
		alias = elem.alias
	} else {
		alias = uint16(l + 1)
	}

	al.alias.PushBack(&aliasElem{
		topic: topicName,
		alias: alias,
	})
	al.index[topicName] = alias
	return
}

// New is the constructor of Queue.
func New() *Queue {
	return &Queue{
		topicAlias: make(map[string]*topicAlias),
	}
}
