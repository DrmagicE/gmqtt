package trie

import (
	"errors"
	"strings"
	"sync"

	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/DrmagicE/gmqtt/subscription"
)

// trieDB implement the subscription.Interface, it use trie tree  to store topics.
type trieDB struct {
	sync.RWMutex
	userIndex map[string]map[string]*topicNode // [clientID][topicName]
	userTrie  *topicTrie

	// system topic which begin with "$"
	systemIndex map[string]map[string]*topicNode // [clientID][topicName]
	systemTrie  *topicTrie

	// statistics of the server and each client
	stats       subscription.Stats
	clientStats map[string]*subscription.Stats // [clientID]

}

func (t *trieDB) getTrie(topicName string) *topicTrie {
	if isSystemTopic(topicName) {
		return t.systemTrie
	}
	return t.userTrie
}

func (db *trieDB) GetClientSubscriptions(clientID string) []packets.Topic {
	db.RLock()
	defer db.RUnlock()
	var rs []packets.Topic
	for topicName, v := range db.userIndex[clientID] {
		rs = append(rs, packets.Topic{
			Qos:  v.clients[clientID],
			Name: topicName,
		})
	}
	for topicName, v := range db.systemIndex[clientID] {
		rs = append(rs, packets.Topic{
			Qos:  v.clients[clientID],
			Name: topicName,
		})
	}
	return rs
}

func (db *trieDB) Iterate(fn subscription.IterateFn) {
	db.RLock()
	defer db.RUnlock()
	if !db.userTrie.preOrderTraverse(fn) {
		return
	}
	db.systemTrie.preOrderTraverse(fn)
}

func (db *trieDB) GetStats() subscription.Stats {
	db.RLock()
	defer db.RUnlock()
	return db.stats
}

func (db *trieDB) GetClientStats(clientID string) (subscription.Stats, error) {
	db.RLock()
	defer db.RUnlock()
	if stats, ok := db.clientStats[clientID]; !ok {
		return subscription.Stats{}, errors.New("client not exists")
	} else {
		return *stats, nil
	}
}

func (db *trieDB) Get(topicFilter string) subscription.ClientTopics {
	db.RLock()
	defer db.RUnlock()
	node := db.getTrie(topicFilter).find(topicFilter)
	if node != nil {
		rs := make(subscription.ClientTopics)
		for clientID, qos := range node.clients {
			rs[clientID] = append(rs[clientID], packets.Topic{
				Qos:  qos,
				Name: node.topicName,
			})
		}
		return rs
	}
	return nil
}

func (db *trieDB) GetTopicMatched(topicName string) subscription.ClientTopics {
	db.RLock()
	defer db.RUnlock()
	return db.getTrie(topicName).getMatchedTopicFilter(topicName)
}

// NewStore create a new trieDB instance
func NewStore() *trieDB {
	return &trieDB{
		userIndex: make(map[string]map[string]*topicNode),
		userTrie:  newTopicTrie(),

		systemIndex: make(map[string]map[string]*topicNode),
		systemTrie:  newTopicTrie(),

		clientStats: make(map[string]*subscription.Stats),
	}
}

// Subscribe add subscriptions
func (db *trieDB) Subscribe(clientID string, topics ...packets.Topic) subscription.SubscribeResult {
	db.Lock()
	defer db.Unlock()
	var node *topicNode
	var index map[string]map[string]*topicNode
	rs := make(subscription.SubscribeResult, len(topics))
	for k, topic := range topics {
		rs[k].Topic = topic
		if isSystemTopic(topic.Name) {
			node = db.systemTrie.subscribe(clientID, topic)
			index = db.systemIndex
		} else {
			node = db.userTrie.subscribe(clientID, topic)
			index = db.userIndex
		}
		if index[clientID] == nil {
			index[clientID] = make(map[string]*topicNode)
			db.clientStats[clientID] = &subscription.Stats{}
		}
		if _, ok := index[clientID][topic.Name]; !ok {
			db.stats.SubscriptionsTotal++
			db.stats.SubscriptionsCurrent++
			db.clientStats[clientID].SubscriptionsTotal++
			db.clientStats[clientID].SubscriptionsCurrent++
		} else {
			rs[k].AlreadyExisted = true
		}
		index[clientID][topic.Name] = node
	}
	return rs
}

// Unsubscribe remove  subscriptions
func (db *trieDB) Unsubscribe(clientID string, topics ...string) {
	db.Lock()
	defer db.Unlock()
	var index map[string]map[string]*topicNode
	for _, topic := range topics {
		if isSystemTopic(topic) {
			index = db.systemIndex
		} else {
			index = db.userIndex
		}
		if _, ok := index[clientID]; ok {
			if _, ok := index[clientID][topic]; ok {
				db.stats.SubscriptionsCurrent--
				db.clientStats[clientID].SubscriptionsCurrent--
			}
			delete(index[clientID], topic)
		}
		db.getTrie(topic).unsubscribe(clientID, topic)
	}

}

func (db *trieDB) unsubscribeAll(index map[string]map[string]*topicNode, clientID string) {
	db.stats.SubscriptionsCurrent -= uint64(len(index[clientID]))
	if db.clientStats[clientID] != nil {
		db.clientStats[clientID].SubscriptionsCurrent -= uint64(len(index[clientID]))
	}
	for topicName, node := range index[clientID] {
		delete(node.clients, clientID)
		if len(node.clients) == 0 && len(node.children) == 0 {
			ss := strings.Split(topicName, "/")
			delete(node.parent.children, ss[len(ss)-1])
		}
	}
	delete(index, clientID)
}

// UnsubscribeAll delete all subscriptions of the client
func (db *trieDB) UnsubscribeAll(clientID string) {
	db.Lock()
	defer db.Unlock()
	// user topics
	db.unsubscribeAll(db.userIndex, clientID)
	db.unsubscribeAll(db.systemIndex, clientID)
}

// getMatchedTopicFilter return a map key by clientID that contain all matched topic for the given topicName.
func (db *trieDB) getMatchedTopicFilter(topicName string) map[string][]packets.Topic {
	// system topic
	if isSystemTopic(topicName) {
		return db.systemTrie.getMatchedTopicFilter(topicName)
	}
	return db.userTrie.getMatchedTopicFilter(topicName)
}
