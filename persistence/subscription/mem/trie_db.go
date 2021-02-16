package mem

import (
	"strings"
	"sync"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/persistence/subscription"
)

var _ subscription.Store = (*TrieDB)(nil)

// TrieDB implement the subscription.Interface, it use trie tree to store topics.
type TrieDB struct {
	sync.RWMutex
	userIndex map[string]map[string]*topicNode // [clientID][topicFilter]
	userTrie  *topicTrie

	// system topic which begin with "$"
	systemIndex map[string]map[string]*topicNode // [clientID][topicFilter]
	systemTrie  *topicTrie

	// shared subscription which begin with "$share"
	sharedIndex map[string]map[string]*topicNode // [clientID][shareName/topicFilter]
	sharedTrie  *topicTrie

	// statistics of the server and each client
	stats       subscription.Stats
	clientStats map[string]*subscription.Stats // [clientID]

}

func (db *TrieDB) Init(clientIDs []string) error {
	return nil
}

func (db *TrieDB) Close() error {
	return nil
}

func iterateShared(fn subscription.IterateFn, options subscription.IterationOptions, index map[string]map[string]*topicNode, trie *topicTrie) bool {
	// 查询指定topicFilter
	if options.TopicName != "" && options.MatchType == subscription.MatchName { //寻找指定topicName
		var shareName string
		var topicFilter string
		if strings.HasPrefix(options.TopicName, "$share/") {
			shared := strings.SplitN(options.TopicName, "/", 3)
			shareName = shared[1]
			topicFilter = shared[2]
		} else {
			return true
		}
		node := trie.find(topicFilter)
		if node == nil {
			return true
		}
		if options.ClientID != "" { // 指定topicName & 指定clientID
			if c := node.shared[shareName]; c != nil {
				if sub, ok := c[options.ClientID]; ok {
					if !fn(options.ClientID, sub) {
						return false
					}
				}
			}
		} else {
			if c := node.shared[shareName]; c != nil {
				for clientID, sub := range c {
					if !fn(clientID, sub) {
						return false
					}
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
		for _, v := range index[options.ClientID] {
			for _, c := range v.shared {
				if sub, ok := c[options.ClientID]; ok {
					if !fn(options.ClientID, sub) {
						return false
					}
				}
			}
		}
		return true
	}
	// 遍历
	return trie.preOrderTraverse(fn)
}

func iterateNonShared(fn subscription.IterateFn, options subscription.IterationOptions, index map[string]map[string]*topicNode, trie *topicTrie) bool {
	// 查询指定topicFilter
	if options.TopicName != "" && options.MatchType == subscription.MatchName { //寻找指定topicName
		node := trie.find(options.TopicName)
		if node == nil {
			return true
		}
		if options.ClientID != "" { // 指定topicName & 指定clientID
			if sub, ok := node.clients[options.ClientID]; ok {
				if !fn(options.ClientID, sub) {
					return false
				}
			}

			for _, v := range node.shared {
				if sub, ok := v[options.ClientID]; ok {
					if !fn(options.ClientID, sub) {
						return false
					}
				}
			}

		} else {
			// 指定topic name 不指定clientid
			for clientID, sub := range node.clients {
				if !fn(clientID, sub) {
					return false
				}
			}
			for _, c := range node.shared {
				for clientID, sub := range c {
					if !fn(clientID, sub) {
						return false
					}
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
		for _, v := range index[options.ClientID] {
			sub := v.clients[options.ClientID]
			if !fn(options.ClientID, sub) {
				return false
			}
		}
		return true
	}
	// 遍历
	return trie.preOrderTraverse(fn)

}

// IterateLocked is the non thread-safe version of Iterate
func (db *TrieDB) IterateLocked(fn subscription.IterateFn, options subscription.IterationOptions) {
	if options.Type&subscription.TypeShared == subscription.TypeShared {
		if !iterateShared(fn, options, db.sharedIndex, db.sharedTrie) {
			return
		}
	}
	if options.Type&subscription.TypeNonShared == subscription.TypeNonShared {
		// The Server MUST NOT match Topic Filters starting with a wildcard character (# or +) with Topic Names beginning with a $ character [MQTT-4.7.2-1]
		if !(options.TopicName != "" && isSystemTopic(options.TopicName)) {
			if !iterateNonShared(fn, options, db.userIndex, db.userTrie) {
				return
			}
		}
	}
	if options.Type&subscription.TypeSYS == subscription.TypeSYS {
		if options.TopicName != "" && !isSystemTopic(options.TopicName) {
			return
		}
		if !iterateNonShared(fn, options, db.systemIndex, db.systemTrie) {
			return
		}
	}
}
func (db *TrieDB) Iterate(fn subscription.IterateFn, options subscription.IterationOptions) {
	db.RLock()
	defer db.RUnlock()
	db.IterateLocked(fn, options)
}

// GetStats is the non thread-safe version of GetStats
func (db *TrieDB) GetStatusLocked() subscription.Stats {
	return db.stats
}

// GetStats returns the statistic information of the store
func (db *TrieDB) GetStats() subscription.Stats {
	db.RLock()
	defer db.RUnlock()
	return db.GetStatusLocked()
}

// GetClientStatsLocked the non thread-safe version of GetClientStats
func (db *TrieDB) GetClientStatsLocked(clientID string) (subscription.Stats, error) {
	if stats, ok := db.clientStats[clientID]; !ok {
		return subscription.Stats{}, subscription.ErrClientNotExists
	} else {
		return *stats, nil
	}
}

func (db *TrieDB) GetClientStats(clientID string) (subscription.Stats, error) {
	db.RLock()
	defer db.RUnlock()
	return db.GetClientStatsLocked(clientID)
}

// NewStore create a new TrieDB instance
func NewStore() *TrieDB {
	return &TrieDB{
		userIndex: make(map[string]map[string]*topicNode),
		userTrie:  newTopicTrie(),

		systemIndex: make(map[string]map[string]*topicNode),
		systemTrie:  newTopicTrie(),

		sharedIndex: make(map[string]map[string]*topicNode),
		sharedTrie:  newTopicTrie(),

		clientStats: make(map[string]*subscription.Stats),
	}
}

// SubscribeLocked is the non thread-safe version of Subscribe
func (db *TrieDB) SubscribeLocked(clientID string, subscriptions ...*gmqtt.Subscription) subscription.SubscribeResult {
	var node *topicNode
	var index map[string]map[string]*topicNode
	rs := make(subscription.SubscribeResult, len(subscriptions))
	for k, sub := range subscriptions {
		topicName := sub.TopicFilter
		rs[k].Subscription = sub
		if sub.ShareName != "" {
			node = db.sharedTrie.subscribe(clientID, sub)
			index = db.sharedIndex
		} else if isSystemTopic(topicName) {
			node = db.systemTrie.subscribe(clientID, sub)
			index = db.systemIndex
		} else {
			node = db.userTrie.subscribe(clientID, sub)
			index = db.userIndex
		}
		if index[clientID] == nil {
			index[clientID] = make(map[string]*topicNode)
			if db.clientStats[clientID] == nil {
				db.clientStats[clientID] = &subscription.Stats{}
			}
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

// SubscribeLocked add subscriptions for the client
func (db *TrieDB) Subscribe(clientID string, subscriptions ...*gmqtt.Subscription) (subscription.SubscribeResult, error) {
	db.Lock()
	defer db.Unlock()
	return db.SubscribeLocked(clientID, subscriptions...), nil
}

// UnsubscribeLocked is the non thread-safe version of Unsubscribe
func (db *TrieDB) UnsubscribeLocked(clientID string, topics ...string) {
	var index map[string]map[string]*topicNode
	var topicTrie *topicTrie
	for _, topic := range topics {
		var shareName string
		shareName, topic := subscription.SplitTopic(topic)
		if shareName != "" {
			topicTrie = db.sharedTrie
			index = db.sharedIndex
		} else if isSystemTopic(topic) {
			index = db.systemIndex
			topicTrie = db.systemTrie
		} else {
			index = db.userIndex
			topicTrie = db.userTrie
		}
		if _, ok := index[clientID]; ok {
			if _, ok := index[clientID][topic]; ok {
				db.stats.SubscriptionsCurrent--
				db.clientStats[clientID].SubscriptionsCurrent--
			}
			delete(index[clientID], topic)
		}
		topicTrie.unsubscribe(clientID, topic, shareName)
	}
}

// Unsubscribe remove subscriptions for the client
func (db *TrieDB) Unsubscribe(clientID string, topics ...string) error {
	db.Lock()
	defer db.Unlock()
	db.UnsubscribeLocked(clientID, topics...)
	return nil
}

func (db *TrieDB) unsubscribeAll(index map[string]map[string]*topicNode, clientID string) {
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

// UnsubscribeAllLocked is the non thread-safe version of UnsubscribeAll
func (db *TrieDB) UnsubscribeAllLocked(clientID string) {
	db.unsubscribeAll(db.userIndex, clientID)
	db.unsubscribeAll(db.systemIndex, clientID)
	db.unsubscribeAll(db.sharedIndex, clientID)
}

// UnsubscribeAll delete all subscriptions of the client
func (db *TrieDB) UnsubscribeAll(clientID string) error {
	db.Lock()
	defer db.Unlock()
	// user topics
	db.UnsubscribeAllLocked(clientID)
	return nil
}

// getMatchedTopicFilter return a map key by clientID that contain all matched topic for the given topicName.
func (db *TrieDB) getMatchedTopicFilter(topicName string) subscription.ClientSubscriptions {
	// system topic
	if isSystemTopic(topicName) {
		return db.systemTrie.getMatchedTopicFilter(topicName)
	}
	return db.userTrie.getMatchedTopicFilter(topicName)
}
