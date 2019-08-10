package gmqtt

import (
	"strings"
	"sync"

	"github.com/DrmagicE/gmqtt/pkg/packets"
)

// trieDB is the trie data structure for storing topics.
type trieDB struct {
	mu         sync.RWMutex
	topicIndex map[string]map[string]*topicNode // [clientID][topicName]
	topicTrie  *topicTrie

	// system topic which begin with "$"
	systemIndex map[string]map[string]*topicNode // [clientID][topicName]
	systemTrie  *topicTrie
}

// newTrieDB create a new trieDB instance
func newTrieDB() *trieDB {
	return &trieDB{
		topicIndex: make(map[string]map[string]*topicNode),
		topicTrie:  newTopicTrie(),

		systemIndex: make(map[string]map[string]*topicNode),
		systemTrie:  newTopicTrie(),
	}
}

// subscribe add a subscription
func (db *trieDB) subscribe(clientID string, topic packets.Topic) {
	db.mu.Lock()
	defer db.mu.Unlock()
	if isSystemTopic(topic.Name) {
		node := db.systemTrie.subscribe(clientID, topic)
		if db.systemIndex[clientID] == nil {
			db.systemIndex[clientID] = make(map[string]*topicNode)
		}
		db.systemIndex[clientID][topic.Name] = node
	} else {
		node := db.topicTrie.subscribe(clientID, topic)
		if db.topicIndex[clientID] == nil {
			db.topicIndex[clientID] = make(map[string]*topicNode)
		}
		db.topicIndex[clientID][topic.Name] = node
	}

}

// unsubscribe remove a subscription
func (db *trieDB) unsubscribe(clientID string, topicName string) {
	db.mu.Lock()
	defer db.mu.Unlock()
	if isSystemTopic(topicName) {
		if _, ok := db.systemIndex[clientID]; ok {
			delete(db.systemIndex[clientID], topicName)
		}
		db.systemTrie.unsubscribe(clientID, topicName)
	} else {
		if _, ok := db.topicIndex[clientID]; ok {
			delete(db.topicIndex[clientID], topicName)
		}
		db.topicTrie.unsubscribe(clientID, topicName)
	}

}

// deleteAll delete all subscriptions of the client
func (db *trieDB) deleteAll(clientID string) {
	db.mu.Lock()
	defer db.mu.Unlock()
	// user topics
	for topicName, node := range db.topicIndex[clientID] {
		delete(node.clients, clientID)
		if len(node.clients) == 0 && len(node.children) == 0 {
			ss := strings.Split(topicName, "/")
			delete(node.parent.children, ss[len(ss)-1])
		}
	}
	delete(db.topicIndex, clientID)
	// system topics
	for topicName, node := range db.systemIndex[clientID] {
		delete(node.clients, clientID)
		if len(node.clients) == 0 && len(node.children) == 0 {
			ss := strings.Split(topicName, "/")
			delete(node.parent.children, ss[len(ss)-1])
		}
	}
	delete(db.systemIndex, clientID)
}

// getMatchedTopicFilter return a map key by clientID that contain all matched topic for the given topicName.
func (db *trieDB) getMatchedTopicFilter(topicName string) map[string][]packets.Topic {
	db.mu.RLock()
	defer db.mu.RUnlock()
	// system topic
	if isSystemTopic(topicName) {
		return db.systemTrie.getMatchedTopicFilter(topicName)
	}
	return db.topicTrie.getMatchedTopicFilter(topicName)
}
