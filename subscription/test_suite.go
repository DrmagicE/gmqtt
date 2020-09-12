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

func EqualSubscription(expected Subscription, actual Subscription, a *assert.Assertions) {
	a.Equal(expected.ShareName(), actual.ShareName())
	a.Equal(expected.TopicFilter(), actual.TopicFilter())
	a.Equal(expected.QoS(), actual.QoS())
	a.Equal(expected.RetainHandling(), actual.RetainHandling())
	a.Equal(expected.RetainAsPublished(), actual.RetainAsPublished())
	a.Equal(expected.ID(), actual.ID())
	a.Equal(expected.NoLocal(), actual.NoLocal())
}

func ElementsMatchSub(expected []Subscription, actual []Subscription, a *assert.Assertions) {
	var subActual []Subscription
	for _, v := range actual {
		subActual = append(subActual, New(v.TopicFilter(), v.QoS(),
			RetainAsPublished(v.RetainAsPublished()),
			RetainHandling(v.RetainHandling()),
			NoLocal(v.NoLocal()),
			ID(v.ID()),
			ShareName(v.ShareName()),
		))
	}
	a.ElementsMatch(expected, subActual)
}

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
	}
}
func testGetTopic(t *testing.T, store Store) {
	a := assert.New(t)

	rs := Get(store, topicA.TopicFilter(), TypeAll)
	EqualSubscription(topicA, rs["client1"][0], a)
	EqualSubscription(topicA, rs["client2"][0], a)

	rs = Get(store, topicA.TopicFilter(), TypeNonShared)
	EqualSubscription(topicA, rs["client1"][0], a)
	EqualSubscription(topicA, rs["client2"][0], a)

	rs = Get(store, systemTopicA.TopicFilter(), TypeAll)
	EqualSubscription(systemTopicA, rs["client1"][0], a)
	EqualSubscription(systemTopicA, rs["client2"][0], a)

	rs = Get(store, systemTopicA.TopicFilter(), TypeSYS)
	EqualSubscription(systemTopicA, rs["client1"][0], a)
	EqualSubscription(systemTopicA, rs["client2"][0], a)

	rs = Get(store, "$share/"+sharedTopicA1.ShareName()+"/"+sharedTopicA1.TopicFilter(), TypeAll)
	EqualSubscription(sharedTopicA1, rs["client1"][0], a)
	EqualSubscription(sharedTopicA1, rs["client2"][0], a)
}
func testTopicMatch(t *testing.T, store Store) {
	a := assert.New(t)
	rs := GetTopicMatched(store, topicA.TopicFilter(), TypeAll)
	ElementsMatchSub([]Subscription{topicA, sharedTopicA1, sharedTopicA2}, rs["client1"], a)
	ElementsMatchSub([]Subscription{topicA, sharedTopicA1, sharedTopicA2}, rs["client2"], a)

	rs = GetTopicMatched(store, topicA.TopicFilter(), TypeNonShared)
	ElementsMatchSub([]Subscription{topicA}, rs["client1"], a)
	ElementsMatchSub([]Subscription{topicA}, rs["client2"], a)

	rs = GetTopicMatched(store, topicA.TopicFilter(), TypeShared)
	ElementsMatchSub([]Subscription{sharedTopicA1, sharedTopicA2}, rs["client1"], a)
	ElementsMatchSub([]Subscription{sharedTopicA1, sharedTopicA2}, rs["client2"], a)

	rs = GetTopicMatched(store, systemTopicA.TopicFilter(), TypeSYS)
	ElementsMatchSub([]Subscription{systemTopicA}, rs["client1"], a)
	ElementsMatchSub([]Subscription{systemTopicA}, rs["client2"], a)
}
func testUnsubscribe(t *testing.T, store Store) {
	a := assert.New(t)
	store.Unsubscribe("client1", topicA.TopicFilter())
	rs := Get(store, topicA.TopicFilter(), TypeAll)
	a.Nil(rs["client1"])
	ElementsMatchSub([]Subscription{topicA}, rs["client2"], a)
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
	ElementsMatchSub([]Subscription{topicA, topicB}, got["client1"], a)
	ElementsMatchSub([]Subscription{topicA, topicB}, got["client2"], a)

	// Iterate all non-shared subscriptions with ClientID option.
	got = make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type:     TypeNonShared,
		ClientID: "client1",
	})
	ElementsMatchSub([]Subscription{topicA, topicB}, got["client1"], a)
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
	ElementsMatchSub([]Subscription{topicA}, got["client1"], a)
	ElementsMatchSub([]Subscription{topicA}, got["client2"], a)

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
	ElementsMatchSub([]Subscription{topicA}, got["client1"], a)
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
	ElementsMatchSub([]Subscription{topicA}, got["client1"], a)
	ElementsMatchSub([]Subscription{topicA}, got["client2"], a)

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
	ElementsMatchSub([]Subscription{topicA}, got["client1"], a)
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
	ElementsMatchSub([]Subscription{sharedTopicA1, sharedTopicA2, sharedTopicB1, sharedTopicB2}, got["client1"], a)
	ElementsMatchSub([]Subscription{sharedTopicA1, sharedTopicA2, sharedTopicB1, sharedTopicB2}, got["client2"], a)

	// Iterate all shared subscriptions with ClientID option.
	got = make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type:     TypeShared,
		ClientID: "client1",
	})
	ElementsMatchSub([]Subscription{sharedTopicA1, sharedTopicA2, sharedTopicB1, sharedTopicB2}, got["client1"], a)
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
	ElementsMatchSub([]Subscription{sharedTopicA1}, got["client1"], a)
	ElementsMatchSub([]Subscription{sharedTopicA1}, got["client2"], a)

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
	ElementsMatchSub([]Subscription{sharedTopicA1}, got["client1"], a)
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
	ElementsMatchSub([]Subscription{sharedTopicA1, sharedTopicA2}, got["client1"], a)
	ElementsMatchSub([]Subscription{sharedTopicA1, sharedTopicA2}, got["client2"], a)

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
	ElementsMatchSub([]Subscription{sharedTopicA1, sharedTopicA2}, got["client1"], a)
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
	ElementsMatchSub([]Subscription{systemTopicA, systemTopicB}, got["client1"], a)
	ElementsMatchSub([]Subscription{systemTopicA, systemTopicB}, got["client2"], a)

	// Iterate all system subscriptions with ClientID option.
	got = make(ClientSubscriptions)
	store.Iterate(func(clientID string, sub Subscription) bool {
		got[clientID] = append(got[clientID], sub)
		return true
	}, IterationOptions{
		Type:     TypeSYS,
		ClientID: "client1",
	})
	ElementsMatchSub([]Subscription{systemTopicA, systemTopicB}, got["client1"], a)
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
	ElementsMatchSub([]Subscription{systemTopicA}, got["client1"], a)
	ElementsMatchSub([]Subscription{systemTopicA}, got["client2"], a)

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
	ElementsMatchSub([]Subscription{systemTopicA}, got["client1"], a)
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
	ElementsMatchSub([]Subscription{systemTopicA}, got["client1"], a)
	ElementsMatchSub([]Subscription{systemTopicA}, got["client2"], a)

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
	ElementsMatchSub([]Subscription{systemTopicA}, got["client1"], a)
	a.Len(got["client2"], 0)

}
