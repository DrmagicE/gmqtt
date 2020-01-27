package trie

import (
	"strings"

	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/DrmagicE/gmqtt/subscription"
)

// topicTrie
type topicTrie = topicNode

// children
type children = map[string]*topicNode

// topicNode
type topicNode struct {
	children  children
	clients   map[string]uint8 // clientID => qos
	parent    *topicNode       // pointer of parent node
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
		clients:  make(map[string]uint8),
	}
}

// newChild create a child node of t
func (t *topicNode) newChild() *topicNode {
	return &topicNode{
		children: children{},
		clients:  make(map[string]uint8),
		parent:   t,
	}
}

// subscribe add a subscription and return the added node
func (t *topicTrie) subscribe(clientID string, topic packets.Topic) *topicNode {
	topicSlice := strings.Split(topic.Name, "/")
	var pNode = t
	for _, lv := range topicSlice {
		if _, ok := pNode.children[lv]; !ok {
			pNode.children[lv] = pNode.newChild()
		}
		pNode = pNode.children[lv]
	}
	pNode.clients[clientID] = topic.Qos
	pNode.topicName = topic.Name
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
func setRs(node *topicNode, rs subscription.ClientTopics) {
	for cid, qos := range node.clients {

		if _, ok := rs[cid]; !ok {
			rs[cid] = make([]packets.Topic, 0)
		}
		rs[cid] = append(rs[cid], packets.Topic{
			Qos:  qos,
			Name: node.topicName,
		})
	}
}

// matchTopic get all matched topic for given topicSlice, and set into rs
func (t *topicTrie) matchTopic(topicSlice []string, rs subscription.ClientTopics) {
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
func (t *topicTrie) getMatchedTopicFilter(topicName string) map[string][]packets.Topic {
	topicLv := strings.Split(topicName, "/")
	qos := make(map[string][]packets.Topic)
	t.matchTopic(topicLv, qos)
	return qos
}

func isSystemTopic(topicName string) bool {
	return len(topicName) >= 1 && topicName[0] == '$'
}

func (t *topicTrie) preOrderTraverse(fn subscription.IterateFn) bool {
	if t == nil {
		return false
	}
	if t.topicName != "" {
		for clientID, qos := range t.clients {
			if !fn(clientID, packets.Topic{
				Qos:  qos,
				Name: t.topicName,
			}) {
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
