package trie

import (
	"errors"
	"strings"
	"sync"

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

	// shared subscription which begin with "$share"
	sharedIndex map[string]map[string]*topicNode // [clientID][topicName]
	sharedTrie  *topicTrie

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

func iterate(fn subscription.IterateFn, options subscription.IterationOptions, index map[string]map[string]*topicNode, trie *topicTrie) bool {
	// 查询指定topicFilter
	if options.TopicName != "" && options.MatchType == subscription.MatchName { //寻找指定topicName
		node := trie.find(options.TopicName)
		if node == nil {
			return true
		}
		if options.ClientID != "" { // 指定topicName & 指定clientID
			if subOpts, ok := node.clients[options.ClientID]; ok {
				if !fn(options.ClientID, subOpts.subscription(node.topicName)) {
					return false
				}
			}
		} else {
			// 指定topic name 不指定clientid
			for clientID, subOpts := range node.clients {
				if !fn(clientID, subOpts.subscription(node.topicName)) {
					return false
				}
			}
		}
		return true
	}
	// 查询Match指定topicFilter
	if options.TopicName != "" && options.MatchType == subscription.MatchFilter { // match指定的topicfilter
		node := trie.getMatchedTopicFilter(options.TopicName)
		if node == nil {
			return true
		}
		if options.ClientID != "" {
			for _, v := range node[options.ClientID] {
				if !fn(options.ClientID, v) {
					return false
				}
			}
		} else {
			for clientID, subs := range node {
				for _, v := range subs {
					if !fn(clientID, v) {
						return false
					}
				}
			}
		}
		return true
	}
	// 查询指定clientID下的所有topic
	if options.ClientID != "" {
		for topicFilter, v := range index[options.ClientID] {
			subOpts := v.clients[options.ClientID]
			if !fn(options.ClientID, subOpts.subscription(topicFilter)) {
				return false
			}
		}
		return true
	}
	// 遍历
	return trie.preOrderTraverse(fn)

}

func (db *trieDB) Iterate(fn subscription.IterateFn, options subscription.IterationOptions) {
	db.RLock()
	defer db.RUnlock()
	if options.Type&subscription.TypeShared == subscription.TypeShared {
		// TODO test case
		if options.MatchType == options.MatchType && options.TopicName != "" {
			if isSystemTopic(options.TopicName) {
				return
			}
		}
		if !iterate(fn, options, db.sharedIndex, db.sharedTrie) {
			return
		}
	}
	if options.Type&subscription.TypeNonShared == subscription.TypeNonShared {
		// TODO test case
		if options.MatchType == options.MatchType && options.TopicName != "" {
			if isSystemTopic(options.TopicName) {
				return
			}
		}
		if !iterate(fn, options, db.userIndex, db.userTrie) {
			return
		}
	}
	if options.Type&subscription.TypeSYS == subscription.TypeSYS {
		if !iterate(fn, options, db.systemIndex, db.systemTrie) {
			return
		}
	}

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

// NewStore create a new trieDB instance
func NewStore() *trieDB {
	return &trieDB{
		userIndex: make(map[string]map[string]*topicNode),
		userTrie:  newTopicTrie(),

		systemIndex: make(map[string]map[string]*topicNode),
		systemTrie:  newTopicTrie(),

		sharedIndex: make(map[string]map[string]*topicNode),
		sharedTrie:  newTopicTrie(),

		clientStats: make(map[string]*subscription.Stats),
	}
}

// Subscribe add subscriptions
func (db *trieDB) Subscribe(clientID string, subscriptions ...subscription.Subscription) subscription.SubscribeResult {
	db.Lock()
	defer db.Unlock()
	var node *topicNode
	var index map[string]map[string]*topicNode
	rs := make(subscription.SubscribeResult, len(subscriptions))
	for k, sub := range subscriptions {
		topicName := sub.TopicFilter()
		rs[k].Subscription = sub
		if sub.ShareName() != "" {
			node = db.sharedTrie.subscribe(clientID, topicName, fromSubscription(sub))
			index = db.sharedIndex
		} else if isSystemTopic(topicName) {
			node = db.systemTrie.subscribe(clientID, topicName, fromSubscription(sub))
			index = db.systemIndex
		} else {
			node = db.userTrie.subscribe(clientID, topicName, fromSubscription(sub))
			index = db.userIndex
		}
		if index[clientID] == nil {
			index[clientID] = make(map[string]*topicNode)
			db.clientStats[clientID] = &subscription.Stats{}
		}
		if _, ok := index[clientID][topicName]; !ok {
			db.stats.SubscriptionsTotal++
			db.stats.SubscriptionsCurrent++
			db.clientStats[clientID].SubscriptionsTotal++
			db.clientStats[clientID].SubscriptionsCurrent++
		} else {
			rs[k].AlreadyExisted = true
		}
		index[clientID][topicName] = node
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
func (db *trieDB) getMatchedTopicFilter(topicName string) subscription.ClientSubscriptions {
	// system topic
	if isSystemTopic(topicName) {
		return db.systemTrie.getMatchedTopicFilter(topicName)
	}
	return db.userTrie.getMatchedTopicFilter(topicName)
}
