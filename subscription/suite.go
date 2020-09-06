package subscription

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	topicA = New("topic/A", 1, RetainAsPublished(true), RetainHandling(1), ShareName(""), ID(1), NoLocal(true))
	topicB = New("topic/B", 1, RetainAsPublished(true), RetainHandling(0), ShareName(""), ID(0), NoLocal(false))

	systemTopicA = New("$topic/A", 1, RetainAsPublished(true), RetainHandling(1), ShareName(""), ID(1), NoLocal(true))
	systemTopicB = New("$topic/B", 1, RetainAsPublished(true), RetainHandling(0), ShareName(""), ID(0), NoLocal(false))

	sharedTopicA1 = New("topic/A", 1, RetainAsPublished(true), RetainHandling(1), ShareName("name1"), ID(1), NoLocal(true))
	sharedTopicB1 = New("topic/B", 1, RetainAsPublished(true), RetainHandling(1), ShareName("name1"), ID(1), NoLocal(true))

	sharedTopicA2 = New("topic/A", 1, RetainAsPublished(true), RetainHandling(1), ShareName("name2"), ID(1), NoLocal(true))
	sharedTopicB2 = New("topic/B", 1, RetainAsPublished(true), RetainHandling(1), ShareName("name2"), ID(1), NoLocal(true))
)

var testSubs = []struct {
	clientID string
	subs     []Subscription
}{
	// non-share and non-system subscription
	{

		clientID: "client1",
		subs: []Subscription{
			topicA, topicB,
		},
	}, {
		clientID: "client2",
		subs: []Subscription{
			topicA, topicB,
		},
	},
	// system subscription
	{

		clientID: "client1",
		subs: []Subscription{
			systemTopicA, systemTopicB,
		},
	}, {

		clientID: "client2",
		subs: []Subscription{
			systemTopicA, systemTopicB,
		},
	},
	// share subscription
	{
		clientID: "client1",
		subs: []Subscription{
			sharedTopicA1, sharedTopicB1, sharedTopicA2, sharedTopicB2,
		},
	},
	{
		clientID: "client2",
		subs: []Subscription{
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
		t.Run("iterate"+strconv.Itoa(i), func(t *testing.T) {
			testIterate(t, store)
		})
		t.Run("testUnsubscribe"+strconv.Itoa(i), func(t *testing.T) {
			testUnsubscribe(t, store)
		})
	}
}
func testGetTopic(t *testing.T, store Store) {
	a := assert.New(t)

	rs := Get(store, topicA.TopicFilter(), TypeAll)
	a.Equal(topicA, rs["client1"][0])
	a.Equal(topicA, rs["client2"][0])

	rs = Get(store, topicA.TopicFilter(), TypeNonShared)
	a.Equal(topicA, rs["client1"][0])
	a.Equal(topicA, rs["client2"][0])

	rs = Get(store, systemTopicA.TopicFilter(), TypeAll)
	a.Equal(systemTopicA, rs["client1"][0])
	a.Equal(systemTopicA, rs["client2"][0])

	rs = Get(store, systemTopicA.TopicFilter(), TypeSYS)
	a.Equal(systemTopicA, rs["client1"][0])
	a.Equal(systemTopicA, rs["client2"][0])

	rs = Get(store, "$share/"+sharedTopicA1.ShareName()+"/"+sharedTopicA1.TopicFilter(), TypeAll)
	a.Equal(sharedTopicA1, rs["client1"][0])
	a.Equal(sharedTopicA1, rs["client2"][0])
}
func testTopicMatch(t *testing.T, store Store) {
	a := assert.New(t)
	rs := GetTopicMatched(store, topicA.TopicFilter(), TypeAll)
	a.ElementsMatch([]Subscription{topicA, sharedTopicA1, sharedTopicA2}, rs["client1"])
	a.ElementsMatch([]Subscription{topicA, sharedTopicA1, sharedTopicA2}, rs["client2"])

	rs = GetTopicMatched(store, topicA.TopicFilter(), TypeNonShared)
	a.ElementsMatch([]Subscription{topicA}, rs["client1"])
	a.ElementsMatch([]Subscription{topicA}, rs["client2"])

	rs = GetTopicMatched(store, topicA.TopicFilter(), TypeShared)
	a.ElementsMatch([]Subscription{sharedTopicA1, sharedTopicA2}, rs["client1"])
	a.ElementsMatch([]Subscription{sharedTopicA1, sharedTopicA2}, rs["client2"])

	rs = GetTopicMatched(store, systemTopicA.TopicFilter(), TypeSYS)
	a.ElementsMatch([]Subscription{systemTopicA}, rs["client1"])
	a.ElementsMatch([]Subscription{systemTopicA}, rs["client2"])
}
func testUnsubscribe(t *testing.T, store Store) {
	a := assert.New(t)
	store.Unsubscribe("client1", topicA.TopicFilter())
	rs := Get(store, topicA.TopicFilter(), TypeAll)
	a.Nil(rs["client1"])
	a.ElementsMatch([]Subscription{topicA}, rs["client2"])
	store.UnsubscribeAll("client2")
	store.UnsubscribeAll("client1")
	var iterationCalled bool
	store.Iterate(func(clientID string, sub Subscription) bool {
		fmt.Println(sub)
		iterationCalled = true
		return true
	}, IterationOptions{Type: TypeAll})
	a.False(iterationCalled)
}
func testIterate(t *testing.T, store Store) {
	a := assert.New(t)

	var iterationCalled bool
	// invalid IterationOptions
	store.Iterate(func(clientID string, sub Subscription) bool {
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
	// Iterate all non-shared subscriptions.
	got := make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type: TypeNonShared,
	})
	a.ElementsMatch([]Subscription{topicA, topicB}, got["client1"])
	a.ElementsMatch([]Subscription{topicA, topicB}, got["client2"])

	// Iterate all non-shared subscriptions with ClientID option.
	got = make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type:     TypeNonShared,
		ClientID: "client1",
	})
	a.ElementsMatch([]Subscription{topicA, topicB}, got["client1"])
	a.Len(got["client2"], 0)

	// Iterate all non-shared subscriptions that matched given topic name.
	got = make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type:      TypeNonShared,
		MatchType: MatchName,
		TopicName: topicA.TopicFilter(),
	})
	a.ElementsMatch([]Subscription{topicA}, got["client1"])
	a.ElementsMatch([]Subscription{topicA}, got["client2"])

	// Iterate all non-shared subscriptions that matched given topic name and client id
	got = make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type:      TypeNonShared,
		MatchType: MatchName,
		TopicName: topicA.TopicFilter(),
		ClientID:  "client1",
	})
	a.ElementsMatch([]Subscription{topicA}, got["client1"])
	a.Len(got["client2"], 0)

	// Iterate all non-shared subscriptions that matched given topic filter.
	got = make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type:      TypeNonShared,
		MatchType: MatchFilter,
		TopicName: topicA.TopicFilter(),
	})
	a.ElementsMatch([]Subscription{topicA}, got["client1"])
	a.ElementsMatch([]Subscription{topicA}, got["client2"])

	// Iterate all non-shared subscriptions that matched given topic filter and client id
	got = make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type:      TypeNonShared,
		MatchType: MatchFilter,
		TopicName: topicA.TopicFilter(),
		ClientID:  "client1",
	})
	a.ElementsMatch([]Subscription{topicA}, got["client1"])
	a.Len(got["client2"], 0)
}
func testIterateShared(t *testing.T, store Store) {
	a := assert.New(t)
	// Iterate all shared subscriptions.
	got := make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type: TypeShared,
	})
	a.ElementsMatch([]Subscription{sharedTopicA1, sharedTopicA2, sharedTopicB1, sharedTopicB2}, got["client1"])
	a.ElementsMatch([]Subscription{sharedTopicA1, sharedTopicA2, sharedTopicB1, sharedTopicB2}, got["client2"])

	// Iterate all shared subscriptions with ClientID option.
	got = make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type:     TypeShared,
		ClientID: "client1",
	})
	a.ElementsMatch([]Subscription{sharedTopicA1, sharedTopicA2, sharedTopicB1, sharedTopicB2}, got["client1"])
	a.Len(got["client2"], 0)

	// Iterate all shared subscriptions that matched given topic filter.
	got = make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type:      TypeShared,
		MatchType: MatchName,
		TopicName: "$share/" + sharedTopicA1.ShareName() + "/" + sharedTopicA1.TopicFilter(),
	})
	a.ElementsMatch([]Subscription{sharedTopicA1}, got["client1"])
	a.ElementsMatch([]Subscription{sharedTopicA1}, got["client2"])

	// Iterate all shared subscriptions that matched given topic filter and client id
	got = make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type:      TypeShared,
		MatchType: MatchName,
		TopicName: "$share/" + sharedTopicA1.ShareName() + "/" + sharedTopicA1.TopicFilter(),
		ClientID:  "client1",
	})
	a.ElementsMatch([]Subscription{sharedTopicA1}, got["client1"])
	a.Len(got["client2"], 0)

	// Iterate all shared subscriptions that matched given topic name.
	got = make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type:      TypeShared,
		MatchType: MatchFilter,
		TopicName: sharedTopicA1.TopicFilter(),
	})
	a.ElementsMatch([]Subscription{sharedTopicA1, sharedTopicA2}, got["client1"])
	a.ElementsMatch([]Subscription{sharedTopicA1, sharedTopicA2}, got["client2"])

	// Iterate all shared subscriptions that matched given topic name and clientID
	got = make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type:      TypeShared,
		MatchType: MatchFilter,
		TopicName: sharedTopicA1.TopicFilter(),
		ClientID:  "client1",
	})
	a.ElementsMatch([]Subscription{sharedTopicA1, sharedTopicA2}, got["client1"])
	a.Len(got["client2"], 0)

}
func testIterateSystem(t *testing.T, store Store) {
	a := assert.New(t)
	// Iterate all system subscriptions.
	got := make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type: TypeSYS,
	})
	a.ElementsMatch([]Subscription{systemTopicA, systemTopicB}, got["client1"])
	a.ElementsMatch([]Subscription{systemTopicA, systemTopicB}, got["client2"])

	// Iterate all system subscriptions with ClientID option.
	got = make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type:     TypeSYS,
		ClientID: "client1",
	})
	a.ElementsMatch([]Subscription{systemTopicA, systemTopicB}, got["client1"])
	a.Len(got["client2"], 0)

	// Iterate all system subscriptions that matched given topic filter.
	got = make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type:      TypeSYS,
		MatchType: MatchName,
		TopicName: systemTopicA.TopicFilter(),
	})
	a.ElementsMatch([]Subscription{systemTopicA}, got["client1"])
	a.ElementsMatch([]Subscription{systemTopicA}, got["client2"])

	// Iterate all system subscriptions that matched given topic filter and client id
	got = make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type:      TypeSYS,
		MatchType: MatchName,
		TopicName: systemTopicA.TopicFilter(),
		ClientID:  "client1",
	})
	a.ElementsMatch([]Subscription{systemTopicA}, got["client1"])
	a.Len(got["client2"], 0)

	// Iterate all system subscriptions that matched given topic name.
	got = make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type:      TypeSYS,
		MatchType: MatchFilter,
		TopicName: systemTopicA.TopicFilter(),
	})
	a.ElementsMatch([]Subscription{systemTopicA}, got["client1"])
	a.ElementsMatch([]Subscription{systemTopicA}, got["client2"])

	// Iterate all system subscriptions that matched given topic name and clientID
	got = make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type:      TypeSYS,
		MatchType: MatchFilter,
		TopicName: systemTopicA.TopicFilter(),
		ClientID:  "client1",
	})
	a.ElementsMatch([]Subscription{systemTopicA}, got["client1"])
	a.Len(got["client2"], 0)

}
