package trie

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DrmagicE/gmqtt/pkg/packets"
)

type mockMsg struct {
	dup      bool
	qos      uint8
	retained bool
	topic    string
	packetID packets.PacketID
	payload  []byte
}

func (m *mockMsg) Dup() bool {
	return m.dup
}

func (m *mockMsg) Qos() uint8 {
	return m.qos
}

func (m *mockMsg) Retained() bool {
	return m.retained
}

func (m *mockMsg) Topic() string {
	return m.topic
}

func (m *mockMsg) PacketID() packets.PacketID {
	return m.packetID
}

func (m *mockMsg) Payload() []byte {
	return m.payload
}

func TestTrieDB_ClearAll(t *testing.T) {
	a := assert.New(t)
	s := NewStore()
	s.AddOrReplace(&mockMsg{
		topic: "a/b/c",
	})
	s.AddOrReplace(&mockMsg{
		topic:   "a/b/c/d",
		payload: []byte{1, 2, 3},
	})
	s.ClearAll()
	a.Nil(s.GetRetainedMessage("a/b/c"))
	a.Nil(s.GetRetainedMessage("a/b/c/d"))
}

func TestTrieDB_GetRetainedMessage(t *testing.T) {
	a := assert.New(t)
	s := NewStore()
	tt := []packets.Message{
		&mockMsg{
			topic:   "a/b/c/d",
			payload: []byte{1, 2, 3},
		},
		&mockMsg{
			topic:   "a/b/c/",
			payload: []byte{1, 2, 3, 4},
		},
		&mockMsg{
			topic:   "a/",
			payload: []byte{1, 2, 3},
		},
	}
	for _, v := range tt {
		s.AddOrReplace(v)
	}
	for _, v := range tt {
		a.Equal(v, s.GetRetainedMessage(v.Topic()))
	}
	a.Nil(s.GetRetainedMessage("a/b"))
}

func TestTrieDB_GetMatchedMessages(t *testing.T) {
	a := assert.New(t)
	s := NewStore()
	msgs := []packets.Message{
		&mockMsg{
			topic:   "a/b/c/d",
			payload: []byte{1, 2, 3},
		},
		&mockMsg{
			topic:   "a/b/c/",
			payload: []byte{1, 2, 3, 4},
		},
		&mockMsg{
			topic:   "a/",
			payload: []byte{1, 2, 3},
		},
		&mockMsg{
			topic:   "a/b",
			payload: []byte{1, 2, 3},
		},
		&mockMsg{
			topic:   "a",
			payload: []byte{1, 2, 3},
		},
	}
	var tt = []struct {
		topicFilter string
		expected    []packets.Message
	}{
		{
			topicFilter: "a/+",
			expected: []packets.Message{
				&mockMsg{
					topic:   "a/",
					payload: []byte{1, 2, 3},
				},
				&mockMsg{
					topic:   "a/b",
					payload: []byte{1, 2, 3},
				}},
		},
		{
			topicFilter: "#",
			expected: []packets.Message{
				&mockMsg{
					topic:   "a/b/c/d",
					payload: []byte{1, 2, 3},
				},
				&mockMsg{
					topic:   "a/b/c/",
					payload: []byte{1, 2, 3, 4},
				},
				&mockMsg{
					topic:   "a/",
					payload: []byte{1, 2, 3},
				},
				&mockMsg{
					topic:   "a/b",
					payload: []byte{1, 2, 3},
				},
				&mockMsg{
					topic:   "a",
					payload: []byte{1, 2, 3},
				},
			},
		},
		{
			topicFilter: "a/#",
			expected: []packets.Message{
				&mockMsg{
					topic:   "a/b/c/d",
					payload: []byte{1, 2, 3},
				},
				&mockMsg{
					topic:   "a/b/c/",
					payload: []byte{1, 2, 3, 4},
				},
				&mockMsg{
					topic:   "a/",
					payload: []byte{1, 2, 3},
				},
				&mockMsg{
					topic:   "a/b",
					payload: []byte{1, 2, 3},
				},
				&mockMsg{
					topic:   "a",
					payload: []byte{1, 2, 3},
				},
			},
		},
		{
			topicFilter: "a/b/c/d",
			expected: []packets.Message{
				&mockMsg{
					topic:   "a/b/c/d",
					payload: []byte{1, 2, 3},
				},
			},
		},
	}
	for _, v := range msgs {
		s.AddOrReplace(v)
	}
	for _, v := range tt {
		a.ElementsMatch(v.expected, s.GetMatchedMessages(v.topicFilter))
	}
}

func TestTrieDB_Remove(t *testing.T) {
	a := assert.New(t)
	s := NewStore()
	s.AddOrReplace(&mockMsg{
		topic: "a/b/c",
	})
	s.AddOrReplace(&mockMsg{
		topic:   "a/b/c/d",
		payload: []byte{1, 2, 3},
	})
	a.NotNil(s.GetRetainedMessage("a/b/c"))
	s.Remove("a/b/c")
	a.Nil(s.GetRetainedMessage("a/b/c"))
}

func TestTrieDB_Iterate(t *testing.T) {
	a := assert.New(t)
	s := NewStore()
	msgs := []packets.Message{
		&mockMsg{
			topic:   "a/b/c/d",
			payload: []byte{1, 2, 3},
		},
		&mockMsg{
			topic:   "a/b/c/",
			payload: []byte{1, 2, 3, 4},
		},
		&mockMsg{
			topic:   "a/",
			payload: []byte{1, 2, 3},
		},
		&mockMsg{
			topic:   "a/b",
			payload: []byte{1, 2, 3},
		},
		&mockMsg{
			topic:   "a",
			payload: []byte{1, 2, 3},
		},
	}

	for _, v := range msgs {
		s.AddOrReplace(v)
	}
	var rs []packets.Message
	s.Iterate(func(message packets.Message) bool {
		rs = append(rs, message)
		return true
	})
	a.ElementsMatch(msgs, rs)

}
