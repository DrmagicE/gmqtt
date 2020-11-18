package subscription

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DrmagicE/gmqtt"
)

var (
	topicA = &gmqtt.Subscription{
		TopicFilter: "topic/A",
		ID:          1,

		QoS:               1,
		NoLocal:           true,
		RetainAsPublished: true,
		RetainHandling:    1,
	}

	topicB = &gmqtt.Subscription{
		TopicFilter: "topic/B",

		QoS:               1,
		NoLocal:           false,
		RetainAsPublished: true,
		RetainHandling:    0,
	}

	systemTopicA = &gmqtt.Subscription{
		TopicFilter: "$topic/A",
		ID:          1,

		QoS:               1,
		NoLocal:           true,
		RetainAsPublished: true,
		RetainHandling:    1,
	}

	systemTopicB = &gmqtt.Subscription{
		TopicFilter: "$topic/B",

		QoS:               1,
		NoLocal:           false,
		RetainAsPublished: true,
		RetainHandling:    0,
	}

	sharedTopicA1 = &gmqtt.Subscription{
		ShareName:   "name1",
		TopicFilter: "topic/A",
		ID:          1,

		QoS:               1,
		NoLocal:           true,
		RetainAsPublished: true,
		RetainHandling:    1,
	}

	sharedTopicB1 = &gmqtt.Subscription{
		ShareName:   "name1",
		TopicFilter: "topic/B",
		ID:          1,

		QoS:               1,
		NoLocal:           true,
		RetainAsPublished: true,
		RetainHandling:    1,
	}

	sharedTopicA2 = &gmqtt.Subscription{
		ShareName:   "name2",
		TopicFilter: "topic/A",
		ID:          1,

		QoS:               1,
		NoLocal:           true,
		RetainAsPublished: true,
		RetainHandling:    1,
	}

	sharedTopicB2 = &gmqtt.Subscription{
		ShareName:   "name2",
		TopicFilter: "topic/B",
		ID:          1,

		QoS:               1,
		NoLocal:           true,
		RetainAsPublished: true,
		RetainHandling:    1,
	}
)

var testSubs = []struct {
	clientID string
	subs     []*gmqtt.Subscription
}{
	// non-share and non-system subscription
	{

		clientID: "client1",
		subs: []*gmqtt.Subscription{
			topicA, topicB,
		},
	}, {
		clientID: "client2",
		subs: []*gmqtt.Subscription{
			topicA, topicB,
		},
	},
	// system subscription
	{

		clientID: "client1",
		subs: []*gmqtt.Subscription{
			systemTopicA, systemTopicB,
		},
	}, {

		clientID: "client2",
		subs: []*gmqtt.Subscription{
			systemTopicA, systemTopicB,
		},
	},
	// share subscription
	{
		clientID: "client1",
		subs: []*gmqtt.Subscription{
			sharedTopicA1, sharedTopicB1, sharedTopicA2, sharedTopicB2,
		},
	},
	{
		clientID: "client2",
		subs: []*gmqtt.Subscription{
			sharedTopicA1, sharedTopicB1, sharedTopicA2, sharedTopicB2,
		},
	},
}

func testAddSubscribe(store Store) {
	for _, v := range testSubs {
		store.Subscribe(v.clientID, v.subs...)
	}
}

func TestSuite(t *testing.T, store Store) {
	for i := 0; i <= 1; i++ {
		testAddSubscribe(store)
		t.Run("getTopic"+strconv.Itoa(i), func(t *testing.T) {
			testGetTopic(t, store)
		})
		t.Run("testTopicMatch"+strconv.Itoa(i), func(t *testing.T) {
			testTopicMatch(t, store)
		})
		t.Run("testIterate"+strconv.Itoa(i), func(t *testing.T) {
			testIterate(t, store)
		})
		t.Run("testUnsubscribe"+strconv.Itoa(i), func(t *testing.T) {
			testUnsubscribe(t, store)
		})
	}
}
func testGetTopic(t *testing.T, store Store) {
	a := assert.New(t)

	rs := Get(store, topicA.TopicFilter, TypeAll)
	a.Equal(topicA, rs["client1"][0])
	a.Equal(topicA, rs["client2"][0])

	rs = Get(store, topicA.TopicFilter, TypeNonShared)
	a.Equal(topicA, rs["client1"][0])
	a.Equal(topicA, rs["client2"][0])

	rs = Get(store, systemTopicA.TopicFilter, TypeAll)
	a.Equal(systemTopicA, rs["client1"][0])
	a.Equal(systemTopicA, rs["client2"][0])

	rs = Get(store, systemTopicA.TopicFilter, TypeSYS)
	a.Equal(systemTopicA, rs["client1"][0])
	a.Equal(systemTopicA, rs["client2"][0])

	rs = Get(store, "$share/"+sharedTopicA1.ShareName+"/"+sharedTopicA1.TopicFilter, TypeAll)
	a.Equal(sharedTopicA1, rs["client1"][0])
	a.Equal(sharedTopicA1, rs["client2"][0])

}
func testTopicMatch(t *testing.T, store Store) {
	a := assert.New(t)
	rs := GetTopicMatched(store, topicA.TopicFilter, TypeAll)
	a.ElementsMatch([]*gmqtt.Subscription{topicA, sharedTopicA1, sharedTopicA2}, rs["client1"])
	a.ElementsMatch([]*gmqtt.Subscription{topicA, sharedTopicA1, sharedTopicA2}, rs["client2"])

	rs = GetTopicMatched(store, topicA.TopicFilter, TypeNonShared)
	a.ElementsMatch([]*gmqtt.Subscription{topicA}, rs["client1"])
	a.ElementsMatch([]*gmqtt.Subscription{topicA}, rs["client2"])

	rs = GetTopicMatched(store, topicA.TopicFilter, TypeShared)
	a.ElementsMatch([]*gmqtt.Subscription{sharedTopicA1, sharedTopicA2}, rs["client1"])
	a.ElementsMatch([]*gmqtt.Subscription{sharedTopicA1, sharedTopicA2}, rs["client2"])

	rs = GetTopicMatched(store, systemTopicA.TopicFilter, TypeSYS)
	a.ElementsMatch([]*gmqtt.Subscription{systemTopicA}, rs["client1"])
	a.ElementsMatch([]*gmqtt.Subscription{systemTopicA}, rs["client2"])

}
func testUnsubscribe(t *testing.T, store Store) {
	a := assert.New(t)
	store.Unsubscribe("client1", topicA.TopicFilter)
	rs := Get(store, topicA.TopicFilter, TypeAll)
	a.Nil(rs["client1"])
	a.ElementsMatch([]*gmqtt.Subscription{topicA}, rs["client2"])

	store.UnsubscribeAll("client2")
	store.UnsubscribeAll("client1")
	var iterationCalled bool
	store.Iterate(func(clientID string, sub *gmqtt.Subscription) bool {
		iterationCalled = true
		return true
	}, IterationOptions{Type: TypeAll})
	a.False(iterationCalled)
}
func testIterate(t *testing.T, store Store) {
	a := assert.New(t)

	var iterationCalled bool
	// invalid IterationOptions
	store.Iterate(func(clientID string, sub *gmqtt.Subscription) bool {
		iterationCalled = true
		return true
	}, IterationOptions{})
	a.False(iterationCalled)
	testIterateNonShared(t, store)
	testIterateShared(t, store)
	testIterateSystem(t, store)
}
func testIterateNonShared(t *testing.T, store Store) {
	a := assert.New(t)
	// iterate all non-shared subscriptions.
	got := make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub *gmqtt.Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type: TypeNonShared,
	})
	a.ElementsMatch([]*gmqtt.Subscription{topicA, topicB}, got["client1"])
	a.ElementsMatch([]*gmqtt.Subscription{topicA, topicB}, got["client2"])

	// iterate all non-shared subscriptions with ClientID option.
	got = make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub *gmqtt.Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type:     TypeNonShared,
		ClientID: "client1",
	})

	a.ElementsMatch([]*gmqtt.Subscription{topicA, topicB}, got["client1"])
	a.Len(got["client2"], 0)

	// iterate all non-shared subscriptions that matched given topic name.
	got = make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub *gmqtt.Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type:      TypeNonShared,
		MatchType: MatchName,
		TopicName: topicA.TopicFilter,
	})
	a.ElementsMatch([]*gmqtt.Subscription{topicA}, got["client1"])
	a.ElementsMatch([]*gmqtt.Subscription{topicA}, got["client2"])

	// iterate all non-shared subscriptions that matched given topic name and client id
	got = make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub *gmqtt.Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type:      TypeNonShared,
		MatchType: MatchName,
		TopicName: topicA.TopicFilter,
		ClientID:  "client1",
	})
	a.ElementsMatch([]*gmqtt.Subscription{topicA}, got["client1"])
	a.Len(got["client2"], 0)

	// iterate all non-shared subscriptions that matched given topic filter.
	got = make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub *gmqtt.Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type:      TypeNonShared,
		MatchType: MatchFilter,
		TopicName: topicA.TopicFilter,
	})
	a.ElementsMatch([]*gmqtt.Subscription{topicA}, got["client1"])
	a.ElementsMatch([]*gmqtt.Subscription{topicA}, got["client2"])

	// iterate all non-shared subscriptions that matched given topic filter and client id
	got = make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub *gmqtt.Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type:      TypeNonShared,
		MatchType: MatchFilter,
		TopicName: topicA.TopicFilter,
		ClientID:  "client1",
	})
	a.ElementsMatch([]*gmqtt.Subscription{topicA}, got["client1"])
	a.Len(got["client2"], 0)
}
func testIterateShared(t *testing.T, store Store) {
	a := assert.New(t)
	// iterate all shared subscriptions.
	got := make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub *gmqtt.Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type: TypeShared,
	})
	a.ElementsMatch([]*gmqtt.Subscription{sharedTopicA1, sharedTopicA2, sharedTopicB1, sharedTopicB2}, got["client1"])
	a.ElementsMatch([]*gmqtt.Subscription{sharedTopicA1, sharedTopicA2, sharedTopicB1, sharedTopicB2}, got["client2"])

	// iterate all shared subscriptions with ClientID option.
	got = make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub *gmqtt.Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type:     TypeShared,
		ClientID: "client1",
	})
	a.ElementsMatch([]*gmqtt.Subscription{sharedTopicA1, sharedTopicA2, sharedTopicB1, sharedTopicB2}, got["client1"])
	a.Len(got["client2"], 0)

	// iterate all shared subscriptions that matched given topic filter.
	got = make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub *gmqtt.Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type:      TypeShared,
		MatchType: MatchName,
		TopicName: "$share/" + sharedTopicA1.ShareName + "/" + sharedTopicA1.TopicFilter,
	})
	a.ElementsMatch([]*gmqtt.Subscription{sharedTopicA1}, got["client1"])
	a.ElementsMatch([]*gmqtt.Subscription{sharedTopicA1}, got["client2"])

	// iterate all shared subscriptions that matched given topic filter and client id
	got = make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub *gmqtt.Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type:      TypeShared,
		MatchType: MatchName,
		TopicName: "$share/" + sharedTopicA1.ShareName + "/" + sharedTopicA1.TopicFilter,
		ClientID:  "client1",
	})
	a.ElementsMatch([]*gmqtt.Subscription{sharedTopicA1}, got["client1"])
	a.Len(got["client2"], 0)

	// iterate all shared subscriptions that matched given topic name.
	got = make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub *gmqtt.Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type:      TypeShared,
		MatchType: MatchFilter,
		TopicName: sharedTopicA1.TopicFilter,
	})
	a.ElementsMatch([]*gmqtt.Subscription{sharedTopicA1, sharedTopicA2}, got["client1"])
	a.ElementsMatch([]*gmqtt.Subscription{sharedTopicA1, sharedTopicA2}, got["client2"])

	// iterate all shared subscriptions that matched given topic name and clientID
	got = make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub *gmqtt.Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type:      TypeShared,
		MatchType: MatchFilter,
		TopicName: sharedTopicA1.TopicFilter,
		ClientID:  "client1",
	})
	a.ElementsMatch([]*gmqtt.Subscription{sharedTopicA1, sharedTopicA2}, got["client1"])
	a.Len(got["client2"], 0)

}
func testIterateSystem(t *testing.T, store Store) {
	a := assert.New(t)
	// iterate all system subscriptions.
	got := make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub *gmqtt.Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type: TypeSYS,
	})
	a.ElementsMatch([]*gmqtt.Subscription{systemTopicA, systemTopicB}, got["client1"])
	a.ElementsMatch([]*gmqtt.Subscription{systemTopicA, systemTopicB}, got["client2"])

	// iterate all system subscriptions with ClientID option.
	got = make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub *gmqtt.Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type:     TypeSYS,
		ClientID: "client1",
	})
	a.ElementsMatch([]*gmqtt.Subscription{systemTopicA, systemTopicB}, got["client1"])
	a.Len(got["client2"], 0)

	// iterate all system subscriptions that matched given topic filter.
	got = make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub *gmqtt.Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type:      TypeSYS,
		MatchType: MatchName,
		TopicName: systemTopicA.TopicFilter,
	})
	a.ElementsMatch([]*gmqtt.Subscription{systemTopicA}, got["client1"])
	a.ElementsMatch([]*gmqtt.Subscription{systemTopicA}, got["client2"])

	// iterate all system subscriptions that matched given topic filter and client id
	got = make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub *gmqtt.Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type:      TypeSYS,
		MatchType: MatchName,
		TopicName: systemTopicA.TopicFilter,
		ClientID:  "client1",
	})
	a.ElementsMatch([]*gmqtt.Subscription{systemTopicA}, got["client1"])
	a.Len(got["client2"], 0)

	// iterate all system subscriptions that matched given topic name.
	got = make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub *gmqtt.Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type:      TypeSYS,
		MatchType: MatchFilter,
		TopicName: systemTopicA.TopicFilter,
	})
	a.ElementsMatch([]*gmqtt.Subscription{systemTopicA}, got["client1"])
	a.ElementsMatch([]*gmqtt.Subscription{systemTopicA}, got["client2"])

	// iterate all system subscriptions that matched given topic name and clientID
	got = make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub *gmqtt.Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type:      TypeSYS,
		MatchType: MatchFilter,
		TopicName: systemTopicA.TopicFilter,
		ClientID:  "client1",
	})
	a.ElementsMatch([]*gmqtt.Subscription{systemTopicA}, got["client1"])
	a.Len(got["client2"], 0)

}
