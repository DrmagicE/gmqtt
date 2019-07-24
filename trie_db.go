package gmqtt

import (
	"strings"
	"sync"

	"github.com/DrmagicE/gmqtt/pkg/packets"
)

// trieDB is the data structure for storing topic trie.
type trieDB struct {
	sync.RWMutex
	topicIndex map[string]map[string]*topicNode // [clientID][topicName]
	topicTrie  *topicTrie
}

// newTrieDB create a new trieDB instance
func newTrieDB() *trieDB {
	return &trieDB{
		topicIndex: make(map[string]map[string]*topicNode),
		topicTrie:  newTopicTrie(),
	}
}

// subscribe add a subscription
func (db *trieDB) subscribe(clientID string, topic packets.Topic) {
	node := db.topicTrie.subscribe(clientID, topic)
	if db.topicIndex[clientID] == nil {
		db.topicIndex[clientID] = make(map[string]*topicNode)
	}
	db.topicIndex[clientID][topic.Name] = node
}

// unsubscribe remove a subscription
func (db *trieDB) unsubscribe(clientID string, topicName string) {
	if _, ok := db.topicIndex[clientID]; ok {
		delete(db.topicIndex[clientID], topicName)
	}
	db.topicTrie.unsubscribe(clientID, topicName)
}

// deleteAll delete all subscriptions of the client
func (db *trieDB) deleteAll(clientID string) {
	for topicName, node := range db.topicIndex[clientID] {
		delete(node.clients, clientID)
		if len(node.clients) == 0 && len(node.children) == 0 {
			ss := strings.Split(topicName, "/")
			delete(node.parent.children, ss[len(ss)-1])
		}
	}
	delete(db.topicIndex, clientID)
}

// getMatchedTopicFilter return a map key by clientID that contain all matched topic for the given topicName.
func (db *trieDB) getMatchedTopicFilter(topicName string) map[string][]packets.Topic {
	return db.topicTrie.getMatchedTopicFilter(topicName)
}

// getMatchedTopicFilter
func (db *trieDB) getClientTopicFilter(topicName string) []packets.Topic {
	// todo
	return nil
}
