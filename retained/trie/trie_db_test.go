package trie

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DrmagicE/gmqtt"
)

func TestTrieDB_ClearAll(t *testing.T) {
	a := assert.New(t)
	s := NewStore()
	s.AddOrReplace(&gmqtt.Message{
		Topic: "a/b/c",
	})
	s.AddOrReplace(&gmqtt.Message{
		Topic:   "a/b/c/d",
		Payload: []byte{1, 2, 3},
	})
	s.ClearAll()
	a.Nil(s.GetRetainedMessage("a/b/c"))
	a.Nil(s.GetRetainedMessage("a/b/c/d"))
}

func TestTrieDB_GetRetainedMessage(t *testing.T) {
	a := assert.New(t)
	s := NewStore()
	tt := []*gmqtt.Message{
		{
			Topic:   "a/b/c/d",
			Payload: []byte{1, 2, 3},
		},
		{
			Topic:   "a/b/c/",
			Payload: []byte{1, 2, 3, 4},
		},
		{
			Topic:   "a/",
			Payload: []byte{1, 2, 3},
		},
	}
	for _, v := range tt {
		s.AddOrReplace(v)
	}
	for _, v := range tt {
		a.Equal(v, s.GetRetainedMessage(v.Topic))
	}
	a.Nil(s.GetRetainedMessage("a/b"))
}

func TestTrieDB_GetMatchedMessages(t *testing.T) {
	a := assert.New(t)
	s := NewStore()
	msgs := []*gmqtt.Message{
		{
			Topic:   "a/b/c/d",
			Payload: []byte{1, 2, 3},
		},
		{
			Topic:   "a/b/c/",
			Payload: []byte{1, 2, 3, 4},
		},
		{
			Topic:   "a/",
			Payload: []byte{1, 2, 3},
		},
		{
			Topic:   "a/b",
			Payload: []byte{1, 2, 3},
		},
		{
			Topic:   "a",
			Payload: []byte{1, 2, 3},
		},
	}
	var tt = []struct {
		TopicFilter string
		expected    []*gmqtt.Message
	}{
		{
			TopicFilter: "a/+",
			expected: []*gmqtt.Message{
				{
					Topic:   "a/",
					Payload: []byte{1, 2, 3},
				},
				{
					Topic:   "a/b",
					Payload: []byte{1, 2, 3},
				}},
		},
		{
			TopicFilter: "#",
			expected: []*gmqtt.Message{
				{
					Topic:   "a/b/c/d",
					Payload: []byte{1, 2, 3},
				},
				{
					Topic:   "a/b/c/",
					Payload: []byte{1, 2, 3, 4},
				},
				{
					Topic:   "a/",
					Payload: []byte{1, 2, 3},
				},
				{
					Topic:   "a/b",
					Payload: []byte{1, 2, 3},
				},
				{
					Topic:   "a",
					Payload: []byte{1, 2, 3},
				},
			},
		},
		{
			TopicFilter: "a/#",
			expected: []*gmqtt.Message{
				{
					Topic:   "a/b/c/d",
					Payload: []byte{1, 2, 3},
				},
				{
					Topic:   "a/b/c/",
					Payload: []byte{1, 2, 3, 4},
				},
				{
					Topic:   "a/",
					Payload: []byte{1, 2, 3},
				},
				{
					Topic:   "a/b",
					Payload: []byte{1, 2, 3},
				},
				{
					Topic:   "a",
					Payload: []byte{1, 2, 3},
				},
			},
		},
		{
			TopicFilter: "a/b/c/d",
			expected: []*gmqtt.Message{
				{
					Topic:   "a/b/c/d",
					Payload: []byte{1, 2, 3},
				},
			},
		},
	}
	for _, v := range msgs {
		s.AddOrReplace(v)
	}
	for _, v := range tt {
		a.ElementsMatch(v.expected, s.GetMatchedMessages(v.TopicFilter))
	}
}

func TestTrieDB_Remove(t *testing.T) {
	a := assert.New(t)
	s := NewStore()
	s.AddOrReplace(&gmqtt.Message{
		Topic: "a/b/c",
	})
	s.AddOrReplace(&gmqtt.Message{
		Topic:   "a/b/c/d",
		Payload: []byte{1, 2, 3},
	})
	a.NotNil(s.GetRetainedMessage("a/b/c"))
	s.Remove("a/b/c")
	a.Nil(s.GetRetainedMessage("a/b/c"))
}

func TestTrieDB_Iterate(t *testing.T) {
	a := assert.New(t)
	s := NewStore()
	msgs := []*gmqtt.Message{
		{
			Topic:   "a/b/c/d",
			Payload: []byte{1, 2, 3},
		},
		{
			Topic:   "a/b/c/",
			Payload: []byte{1, 2, 3, 4},
		},
		{
			Topic:   "a/",
			Payload: []byte{1, 2, 3},
		},
		{
			Topic:   "a/b",
			Payload: []byte{1, 2, 3},
		},
		{
			Topic:   "a",
			Payload: []byte{1, 2, 3},
		},
	}

	for _, v := range msgs {
		s.AddOrReplace(v)
	}
	var rs []*gmqtt.Message
	s.Iterate(func(message *gmqtt.Message) bool {
		rs = append(rs, message)
		return true
	})
	a.ElementsMatch(msgs, rs)

}
