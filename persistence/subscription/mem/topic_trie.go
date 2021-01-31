package mem

import (
	"strings"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/persistence/subscription"
)

// topicTrie
type topicTrie = topicNode

// children
type children = map[string]*topicNode

type clientOpts map[string]*gmqtt.Subscription

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
func (t *topicTrie) subscribe(clientID string, s *gmqtt.Subscription) *topicNode {
	topicSlice := strings.Split(s.TopicFilter, "/")
	var pNode = t
	for _, lv := range topicSlice {
		if _, ok := pNode.children[lv]; !ok {
			pNode.children[lv] = pNode.newChild()
		}
		pNode = pNode.children[lv]
	}
	// shared subscription
	if s.ShareName != "" {
		if pNode.shared[s.ShareName] == nil {
			pNode.shared[s.ShareName] = make(clientOpts)
		}
		pNode.shared[s.ShareName][clientID] = s
	} else {
		// non-shared
		pNode.clients[clientID] = s
	}
	pNode.topicName = s.TopicFilter
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
		rs[cid] = append(rs[cid], subOpts)
	}

	for _, c := range node.shared {
		for cid, subOpts := range c {
			rs[cid] = append(rs[cid], subOpts)
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
			if !fn(clientID, subOpts) {
				return false
			}
		}

		for _, c := range t.shared {
			for clientID, subOpts := range c {
				if !fn(clientID, subOpts) {
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
