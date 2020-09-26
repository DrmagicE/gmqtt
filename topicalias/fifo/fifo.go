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
	if q.topicAlias[opts.ClientID] == nil {
		q.topicAlias[opts.ClientID] = &topicAlias{
			max:   int(opts.ClientTopicAliasMax),
			alias: list.New(),
			index: make(map[string]uint16),
		}
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

	if _, ok := al.index[topicName]; ok {
		exist = true
	}
	// alias has been exhausted
	if al.alias.Len() == al.max {
		first := al.alias.Front()
		elem := first.Value.(*aliasElem)
		al.alias.Remove(first)
		delete(al.index, elem.topic)
	}
	alias = uint16(al.alias.Len()) + 1
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
