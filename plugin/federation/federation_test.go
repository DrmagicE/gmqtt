package federation

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/persistence/subscription/mem"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/DrmagicE/gmqtt/server"
)

func TestLocalSubStore_init(t *testing.T) {
	a := assert.New(t)
	var tt = struct {
		clientID []string
		topics   []*gmqtt.Subscription
		expected map[string]uint64
	}{
		clientID: []string{"client1", "client2", "client3"},
		topics: []*gmqtt.Subscription{
			{
				ShareName:   "abc",
				TopicFilter: "filter1",
			}, {
				TopicFilter: "filter2",
			}, {
				TopicFilter: "filter3",
			},
		},
		expected: map[string]uint64{
			"$share/abc/filter1": 3,
			"filter2":            3,
			"filter3":            3,
		},
	}
	l := &localSubStore{}
	subStore := mem.NewStore()
	for _, v := range tt.clientID {
		_, err := subStore.Subscribe(v, tt.topics...)
		a.Nil(err)
	}
	l.init(subStore)
	l.Lock()
	a.Equal(tt.expected, l.topics)
	l.Unlock()
}

func TestLocalSubStore_sub_unsub(t *testing.T) {
	a := assert.New(t)

	l := &localSubStore{}
	subStore := mem.NewStore()
	l.init(subStore)

	a.True(l.subscribe("client1", "topic1"))
	// test duplicated subscribe
	a.False(l.subscribe("client1", "topic1"))
	a.Equal(map[string]uint64{
		"topic1": 1,
	}, l.topics)
	a.Equal(map[string]map[string]struct{}{
		"client1": {
			"topic1": struct{}{},
		},
	}, l.index)

	// test duplicated subscribe
	a.False(l.subscribe("client2", "topic1"))
	a.Equal(map[string]uint64{
		"topic1": 2,
	}, l.topics)
	a.Equal(map[string]map[string]struct{}{
		"client1": {
			"topic1": struct{}{},
		},
		"client2": {
			"topic1": struct{}{},
		},
	}, l.index)

	a.True(l.subscribe("client3", "topic2"))
	a.Equal(map[string]uint64{
		"topic1": 2,
		"topic2": 1,
	}, l.topics)
	a.Equal(map[string]map[string]struct{}{
		"client1": {
			"topic1": struct{}{},
		},
		"client2": {
			"topic1": struct{}{},
		},
		"client3": {
			"topic2": struct{}{},
		},
	}, l.index)

	// test unsubscribe not exists topic
	a.False(l.unsubscribe("client4", "topic1"))
	a.Equal(map[string]uint64{
		"topic1": 2,
		"topic2": 1,
	}, l.topics)

	for i := 0; i < 1; i++ {
		a.False(l.unsubscribe("client2", "topic1"))
		a.Equal(map[string]uint64{
			"topic1": 1,
			"topic2": 1,
		}, l.topics)
		a.Equal(map[string]map[string]struct{}{
			"client1": {
				"topic1": struct{}{},
			},
			"client3": {
				"topic2": struct{}{},
			},
		}, l.index)
	}

	unsub := l.unsubscribeAll("client3")
	a.Equal([]string{"topic2"}, unsub)
	a.Equal(map[string]uint64{
		"topic1": 1,
	}, l.topics)

	a.Equal(map[string]map[string]struct{}{
		"client1": {
			"topic1": struct{}{},
		},
	}, l.index)

	a.Len(l.unsubscribeAll("client3"), 0)

	a.True(l.unsubscribe("client1", "topic1"))
	a.False(l.unsubscribe("client1", "topic1"))
}

func TestMessageToEvent(t *testing.T) {
	a := assert.New(t)
	var tt = []struct {
		msg      *gmqtt.Message
		expected *Message
	}{
		{
			msg: &gmqtt.Message{
				Dup:             true,
				QoS:             1,
				Retained:        true,
				Topic:           "topic1",
				Payload:         []byte("topic1"),
				PacketID:        1,
				ContentType:     "ct",
				CorrelationData: []byte("data"),
				MessageExpiry:   1,
				PayloadFormat:   1,
				ResponseTopic:   "respTopic",
				UserProperties: []packets.UserProperty{
					{
						K: []byte("K"),
						V: []byte("V"),
					},
				},
			},
			expected: &Message{
				TopicName:       "topic1",
				Payload:         "topic1",
				Qos:             1,
				Retained:        true,
				ContentType:     "ct",
				CorrelationData: "data",
				MessageExpiry:   1,
				PayloadFormat:   1,
				ResponseTopic:   "respTopic",
				UserProperties: []*UserProperty{
					{
						K: []byte("K"),
						V: []byte("V"),
					},
				},
			},
		},
	}
	for _, v := range tt {
		a.Equal(v.expected, messageToEvent(v.msg))
	}

}

func TestLRUCache(t *testing.T) {
	a := assert.New(t)
	lcache := newLRUCache(1)
	a.False(lcache.set(1))
	a.True(lcache.set(1))
	a.False(lcache.set(2))
	a.Len(lcache.items, 1)
	a.Equal(1, lcache.l.Len())
}

func TestFederation_eventStreamHandler(t *testing.T) {
	a := assert.New(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	p, _ := New(testConfig)
	f := p.(*Federation)

	pub := server.NewMockPublisher(ctrl)
	f.publisher = pub

	sess := &session{
		id:          "abc",
		nodeName:    "node1",
		nextEventID: 0,
		seenEvents:  newLRUCache(3),
	}
	var ack *Ack
	ack = f.eventStreamHandler(sess, &Event{
		Id: 0,
		Event: &Event_Subscribe{
			Subscribe: &Subscribe{
				ShareName:   "",
				TopicFilter: "a",
			},
		},
	})
	a.EqualValues(0, ack.EventId)
	sts, _ := f.feSubStore.GetClientStats("node1")
	a.EqualValues(1, sts.SubscriptionsCurrent)

	msgEvent := &Event_Message{
		Message: &Message{
			TopicName: "a",
			Payload:   "b",
			Qos:       1,
		},
	}
	pub.EXPECT().Publish(eventToMessage(msgEvent.Message))
	ack = f.eventStreamHandler(sess, &Event{
		Id:    1,
		Event: msgEvent,
	})
	a.EqualValues(1, ack.EventId)
	ack = f.eventStreamHandler(sess, &Event{
		Id: 2,
		Event: &Event_Unsubscribe{
			Unsubscribe: &Unsubscribe{
				TopicName: "a",
			},
		},
	})
	sts, _ = f.feSubStore.GetClientStats("node1")
	a.EqualValues(0, sts.SubscriptionsCurrent)
	a.EqualValues(2, ack.EventId)

	// send duplicated event
	ack = f.eventStreamHandler(sess, &Event{
		Id: 0,
		Event: &Event_Subscribe{
			Subscribe: &Subscribe{
				ShareName:   "",
				TopicFilter: "a",
			},
		},
	})
	a.EqualValues(0, ack.EventId)
	sts, _ = f.feSubStore.GetClientStats("node1")
	a.EqualValues(0, sts.SubscriptionsCurrent)

}
