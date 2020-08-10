package trie

import (
	"strings"

	"github.com/DrmagicE/gmqtt/subscription"
)

// topicTrie
type topicTrie = topicNode

// children
type children = map[string]*topicNode

type subOpts struct {
	shareName      string
	qos            byte
	noLocal        bool
	rap            bool
	retainHandling byte
	// subscription identifier
	id uint32
}

func fromSubscription(sub subscription.Subscription) subOpts {
	return subOpts{
		shareName:      sub.ShareName(),
		qos:            sub.QoS(),
		noLocal:        sub.NoLocal(),
		rap:            sub.RetainAsPublished(),
		retainHandling: sub.RetainHandling(),
		id:             sub.ID(),
	}
}

func (s *subOpts) subscription(topicFilter string) subscription.Subscription {
	return subscription.New(
		topicFilter,
		s.qos,
		subscription.ShareName(s.shareName),
		subscription.NoLocal(s.noLocal),
		subscription.ID(s.id),
		subscription.RetainHandling(s.retainHandling),
		subscription.RetainAsPublished(s.rap),
	)
}

// topicNode
type topicNode struct {
	children  children
	clients   map[string]subOpts
	parent    *topicNode // pointer of parent node
	topicName string
}

// newTopicTrie create a new trie tree
func newTopicTrie() *topicTrie {
	return newNode()
}

// newNode create a new trie node
func newNode() *topicNode {
	return &topicNode{
		children: children{},
		clients:  make(map[string]subOpts),
	}
}

// newChild create a child node of t
func (t *topicNode) newChild() *topicNode {
	return &topicNode{
		children: children{},
		clients:  make(map[string]subOpts),
		parent:   t,
	}
}

// subscribe add a subscription and return the added node
func (t *topicTrie) subscribe(clientID string, topicName string, opts subOpts) *topicNode {
	topicSlice := strings.Split(topicName, "/")
	var pNode = t
	for _, lv := range topicSlice {
		if _, ok := pNode.children[lv]; !ok {
			pNode.children[lv] = pNode.newChild()
		}
		pNode = pNode.children[lv]
	}
	pNode.clients[clientID] = opts
	pNode.topicName = topicName
	return pNode
}

// find walk through the tire and return the node that represent the topicFilter
// return nil if not found
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
func (t *topicTrie) unsubscribe(clientID string, topicName string) {
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
	delete(pNode.clients, clientID)
	if len(pNode.clients) == 0 && len(pNode.children) == 0 {
		delete(pNode.parent.children, topicSlice[l-1])
	}
}

// setRs set the node into rs
func setRs(node *topicNode, rs subscription.ClientSubscriptions) {
	for cid, subOpts := range node.clients {
		if _, ok := rs[cid]; !ok {
			rs[cid] = make([]subscription.Subscription, 0)
		}
		rs[cid] = append(rs[cid], subOpts.subscription(node.topicName))
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
	}
	for _, c := range t.children {
		if !c.preOrderTraverse(fn) {
			return false
		}
	}
	return true
}
