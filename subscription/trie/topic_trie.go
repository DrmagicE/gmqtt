package trie

import (
	"strings"

	"github.com/DrmagicE/gmqtt/subscription"
)

// topicTrie
type topicTrie = topicNode

// children
type children = map[string]*topicNode

// sub implements subscription.Subscription interface
type sub struct {
	shareName      string
	topicFilter    string
	qos            byte
	noLocal        bool
	rap            bool
	retainHandling byte
	id             uint32
}

func (s sub) ShareName() string {
	return s.shareName
}

func (s sub) TopicFilter() string {
	return s.topicFilter
}

func (s sub) ID() uint32 {
	return s.id
}

func (s sub) QoS() byte {
	return s.qos
}

func (s sub) NoLocal() bool {
	return s.noLocal
}

func (s sub) RetainAsPublished() bool {
	return s.rap
}

func (s sub) RetainHandling() byte {
	return s.retainHandling
}

func fromSubscription(s subscription.Subscription) sub {
	return sub{
		shareName:      s.ShareName(),
		qos:            s.QoS(),
		noLocal:        s.NoLocal(),
		rap:            s.RetainAsPublished(),
		retainHandling: s.RetainHandling(),
		id:             s.ID(),
	}
}

func (s *sub) subscription(topicFilter string) subscription.Subscription {
	return &sub{
		shareName:      s.shareName,
		topicFilter:    topicFilter,
		noLocal:        s.noLocal,
		rap:            s.rap,
		retainHandling: s.retainHandling,
		id:             s.id,
		qos:            s.qos,
	}
}

type clientOpts map[string]sub

// topicNode
type topicNode struct {
	children children
	// clients store non-share subscription
	clients   clientOpts
	parent    *topicNode // pointer of parent node
	topicName string
	// shared store shared subscription, key by ShareName
	shared map[string]clientOpts
}

// newTopicTrie create a new trie tree
func newTopicTrie() *topicTrie {
	return newNode()
}

// newNode create a new trie node
func newNode() *topicNode {
	return &topicNode{
		children: children{},
		clients:  make(clientOpts),
		shared:   make(map[string]clientOpts),
	}
}

// newChild create a child node of t
func (t *topicNode) newChild() *topicNode {
	n := newNode()
	n.parent = t
	return n
}

// subscribe add a subscription and return the added node
func (t *topicTrie) subscribe(clientID string, topicName string, s sub) *topicNode {
	topicSlice := strings.Split(topicName, "/")
	var pNode = t
	for _, lv := range topicSlice {
		if _, ok := pNode.children[lv]; !ok {
			pNode.children[lv] = pNode.newChild()
		}
		pNode = pNode.children[lv]
	}
	// shared subscription
	if s.shareName != "" {
		if pNode.shared[s.shareName] == nil {
			pNode.shared[s.shareName] = make(clientOpts)
		}
		pNode.shared[s.shareName][clientID] = s
	} else {
		// non-shared
		pNode.clients[clientID] = s
	}
	pNode.topicName = topicName
	return pNode
}

// find walk through the tire and return the node that represent the topicFilter.
// Return nil if not found
func (t *topicTrie) find(topicFilter string) *topicNode {
	topicSlice := strings.Split(topicFilter, "/")
	var pNode = t
	for _, lv := range topicSlice {
		if _, ok := pNode.children[lv]; ok {
			pNode = pNode.children[lv]
		} else {
			return nil
		}
	}
	if pNode.topicName == topicFilter {
		return pNode
	}
	return nil
}

// unsubscribe
func (t *topicTrie) unsubscribe(clientID string, topicName string, shareName string) {
	topicSlice := strings.Split(topicName, "/")
	l := len(topicSlice)
	var pNode = t
	for _, lv := range topicSlice {
		if _, ok := pNode.children[lv]; ok {
			pNode = pNode.children[lv]
		} else {
			return
		}
	}
	if shareName != "" {
		if c := pNode.shared[shareName]; c != nil {
			delete(c, clientID)
			if len(pNode.shared[shareName]) == 0 {
				delete(pNode.shared, shareName)
			}
			if len(pNode.shared) == 0 && len(pNode.children) == 0 {
				delete(pNode.parent.children, topicSlice[l-1])
			}
		}
	} else {
		delete(pNode.clients, clientID)
		if len(pNode.clients) == 0 && len(pNode.children) == 0 {
			delete(pNode.parent.children, topicSlice[l-1])
		}
	}

}

// setRs set the node subscription info into rs
func setRs(node *topicNode, rs subscription.ClientSubscriptions) {
	for cid, subOpts := range node.clients {
		if _, ok := rs[cid]; !ok {
			rs[cid] = make([]subscription.Subscription, 0)
		}
		rs[cid] = append(rs[cid], subOpts.subscription(node.topicName))
	}

	for _, c := range node.shared {
		for cid, subOpts := range c {
			if _, ok := rs[cid]; !ok {
				rs[cid] = make([]subscription.Subscription, 0)
			}
			rs[cid] = append(rs[cid], subOpts.subscription(node.topicName))
		}
	}
}

// matchTopic get all matched topic for given topicSlice, and set into rs
func (t *topicTrie) matchTopic(topicSlice []string, rs subscription.ClientSubscriptions) {
	endFlag := len(topicSlice) == 1
	if cnode := t.children["#"]; cnode != nil {
		setRs(cnode, rs)
	}
	if cnode := t.children["+"]; cnode != nil {
		if endFlag {
			setRs(cnode, rs)
			if n := cnode.children["#"]; n != nil {
				setRs(n, rs)
			}
		} else {
			cnode.matchTopic(topicSlice[1:], rs)
		}
	}
	if cnode := t.children[topicSlice[0]]; cnode != nil {
		if endFlag {
			setRs(cnode, rs)
			if n := cnode.children["#"]; n != nil {
				setRs(n, rs)
			}
		} else {
			cnode.matchTopic(topicSlice[1:], rs)
		}
	}
}

// getMatchedTopicFilter return a map key by clientID that contain all matched topic for the given topicName.
func (t *topicTrie) getMatchedTopicFilter(topicName string) subscription.ClientSubscriptions {
	topicLv := strings.Split(topicName, "/")
	subs := make(subscription.ClientSubscriptions)
	t.matchTopic(topicLv, subs)
	return subs
}

func isSystemTopic(topicName string) bool {
	return len(topicName) >= 1 && topicName[0] == '$'
}

func (t *topicTrie) preOrderTraverse(fn subscription.IterateFn) bool {
	if t == nil {
		return false
	}
	if t.topicName != "" {
		for clientID, subOpts := range t.clients {
			if !fn(clientID, subOpts.subscription(t.topicName)) {
				return false
			}
		}

		for _, c := range t.shared {
			for clientID, subOpts := range c {
				if !fn(clientID, subOpts.subscription(t.topicName)) {
					return false
				}
			}
		}
	}
	for _, c := range t.children {
		if !c.preOrderTraverse(fn) {
			return false
		}
	}
	return true
}
