package trie

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/DrmagicE/gmqtt/subscription"
)

func TestTrieDB_UnsubscribeAll(t *testing.T) {
	a := assert.New(t)
	db := NewStore()
	tt := []struct {
		clientID string
		topic    packets.Topic
	}{
		{clientID: "id0", topic: packets.Topic{Name: "name0", SubOptions: packets.SubOptions{Qos: packets.Qos0}}},
		{clientID: "id1", topic: packets.Topic{Name: "name1", SubOptions: packets.SubOptions{
			Qos: packets.Qos1,
		}}},
		{clientID: "id2", topic: packets.Topic{Name: "name2", SubOptions: packets.SubOptions{
			Qos: packets.Qos2,
		}}},
		{clientID: "id0", topic: packets.Topic{Name: "name3", SubOptions: packets.SubOptions{
			Qos: packets.Qos2,
		}}},
	}

	for _, v := range tt {
		db.Subscribe(v.clientID, subscription.FromTopic(v.topic, 0))
	}
	removedCid := "id0"
	db.UnsubscribeAll(removedCid)
	for _, v := range tt {
		if v.clientID == removedCid {
			rs := db.getMatchedTopicFilter(v.topic.Name)
			if _, ok := rs[v.clientID]; ok {
				t.Fatalf("remove error")
			}
			continue
		}
		got := db.userIndex[v.clientID][v.topic.Name]
		a.Equal(got.topicName, v.topic.Name)
		a.Equal(got.clients[v.clientID].qos, v.topic.Qos)
		rs := db.getMatchedTopicFilter(v.topic.Name)
		a.EqualValues(v.topic.Qos, rs[v.clientID][0].QoS())
	}
}

func TestTrieDB_Subscribe_Unsubscribe(t *testing.T) {
	a := assert.New(t)
	db := NewStore()
	stt := []struct {
		clientID       string
		topic          packets.Topic
		alreadyExisted bool
	}{
		{clientID: "id0", topic: packets.Topic{Name: "name0", SubOptions: packets.SubOptions{Qos: packets.Qos0}}, alreadyExisted: false},
		{clientID: "id1", topic: packets.Topic{Name: "name1", SubOptions: packets.SubOptions{Qos: packets.Qos1}}, alreadyExisted: false},
		{clientID: "id2", topic: packets.Topic{Name: "name2", SubOptions: packets.SubOptions{Qos: packets.Qos2}}, alreadyExisted: false},
		{clientID: "id0", topic: packets.Topic{Name: "name3", SubOptions: packets.SubOptions{Qos: packets.Qos2}}, alreadyExisted: false},
		{clientID: "id0", topic: packets.Topic{Name: "name3", SubOptions: packets.SubOptions{Qos: packets.Qos2}}, alreadyExisted: true},
	}
	utt := []struct {
		clientID  string
		topicName string
	}{
		{clientID: "id0", topicName: "name0"}, {clientID: "id1", topicName: "name1"},
	}
	for _, v := range stt {
		rs := db.Subscribe(v.clientID, subscription.FromTopic(v.topic, 0))
		a.Equal(v.alreadyExisted, rs[0].AlreadyExisted)
	}
	for _, v := range utt {
		db.Unsubscribe(v.clientID, v.topicName)
	}

	expected := []struct {
		clientID string
		topic    packets.Topic
	}{
		{clientID: "id2", topic: packets.Topic{Name: "name2", SubOptions: packets.SubOptions{Qos: packets.Qos2}}},
		{clientID: "id0", topic: packets.Topic{Name: "name3", SubOptions: packets.SubOptions{Qos: packets.Qos2}}},
	}

	var rs []struct {
		clientID string
		topic    packets.Topic
	}
	db.Iterate(func(clientID string, subscription subscription.Subscription) bool {
		rs = append(rs, struct {
			clientID string
			topic    packets.Topic
		}{clientID: clientID, topic: packets.Topic{
			SubOptions: packets.SubOptions{
				Qos: subscription.QoS(),
			},
			Name: subscription.TopicFilter(),
		}})
		return true
	}, subscription.IterationOptions{
		Type: subscription.TypeAll,
	})
	a.ElementsMatch(expected, rs)

}

func TestTrieDB_Iterate(t *testing.T) {
	a := assert.New(t)
	db := NewStore()
	tt := []struct {
		clientID string
		topic    packets.Topic
	}{
		{clientID: "id0", topic: packets.Topic{Name: "name0", SubOptions: packets.SubOptions{Qos: packets.Qos0}}},
		{clientID: "id1", topic: packets.Topic{Name: "name1", SubOptions: packets.SubOptions{Qos: packets.Qos1}}},
		{clientID: "id2", topic: packets.Topic{Name: "name2", SubOptions: packets.SubOptions{Qos: packets.Qos2}}},
		{clientID: "id3", topic: packets.Topic{Name: "name3", SubOptions: packets.SubOptions{Qos: packets.Qos2}}},
	}
	for _, v := range tt {
		db.Subscribe(v.clientID, subscription.FromTopic(v.topic, 0))
	}
	var rs []struct {
		clientID string
		topic    packets.Topic
	}
	db.Iterate(func(clientID string, subscription subscription.Subscription) bool {
		rs = append(rs, struct {
			clientID string
			topic    packets.Topic
		}{clientID: clientID, topic: packets.Topic{
			SubOptions: packets.SubOptions{
				Qos: subscription.QoS(),
			},
			Name: subscription.TopicFilter(),
		}})
		return true
	}, subscription.IterationOptions{
		Type: subscription.TypeAll,
	})
	a.Len(rs, 4)
	a.ElementsMatch(tt, rs)

}

func TestTrieDB_IterateWithTopicFilter(t *testing.T) {
	a := assert.New(t)
	db := NewStore()
	tt := []struct {
		clientID string
		topic    packets.Topic
	}{
		{clientID: "id0", topic: packets.Topic{Name: "name0", SubOptions: packets.SubOptions{Qos: packets.Qos0}}},
		{clientID: "id1", topic: packets.Topic{Name: "name1", SubOptions: packets.SubOptions{Qos: packets.Qos1}}},
		{clientID: "id2", topic: packets.Topic{Name: "name2", SubOptions: packets.SubOptions{Qos: packets.Qos2}}},
		{clientID: "id3", topic: packets.Topic{Name: "name3", SubOptions: packets.SubOptions{Qos: packets.Qos2}}},
		{clientID: "id4", topic: packets.Topic{Name: "name3/name4/name5", SubOptions: packets.SubOptions{Qos: packets.Qos2}}},
	}
	for _, v := range tt {
		db.Subscribe(v.clientID, subscription.FromTopic(v.topic, 0))
	}
	var expected = subscription.ClientSubscriptions{
		"id3": {
			subscription.FromTopic(packets.Topic{
				SubOptions: packets.SubOptions{Qos: packets.Qos2},
				Name:       "name3",
			}, 0),
		},
	}
	rs := subscription.Get(db, "name3", subscription.TypeAll)
	a.Len(rs, 1)
	a.Equal(expected, rs)
}

func TestTrieDB_IterateWithTopicMatched(t *testing.T) {
	a := assert.New(t)
	db := NewStore()
	tt := []struct {
		clientID string
		topic    packets.Topic
	}{
		{clientID: "id0", topic: packets.Topic{Name: "/a/b/c", SubOptions: packets.SubOptions{Qos: packets.Qos0}}},
		{clientID: "id1", topic: packets.Topic{Name: "/a/b/+", SubOptions: packets.SubOptions{Qos: packets.Qos1}}},
		{clientID: "id2", topic: packets.Topic{Name: "/a/b/c/d", SubOptions: packets.SubOptions{Qos: packets.Qos2}}},
	}
	for _, v := range tt {
		db.Subscribe(v.clientID, subscription.FromTopic(v.topic, 0))
	}
	var expected = subscription.ClientSubscriptions{
		"id0": {
			subscription.FromTopic(packets.Topic{
				SubOptions: packets.SubOptions{Qos: packets.Qos0},
				Name:       "/a/b/c",
			}, 0),
		},
		"id1": {subscription.FromTopic(packets.Topic{
			SubOptions: packets.SubOptions{Qos: packets.Qos1},
			Name:       "/a/b/+",
		}, 0)},
	}

	rs := subscription.GetTopicMatched(db, "/a/b/c", subscription.TypeAll)
	a.Len(rs, 2)
	a.Equal(expected, rs)
}

func TestTrieDB_GetStats(t *testing.T) {
	a := assert.New(t)
	db := NewStore()
	tt := []struct {
		clientID string
		topic    packets.Topic
	}{
		{clientID: "id0", topic: packets.Topic{Name: "name0", SubOptions: packets.SubOptions{Qos: packets.Qos0}}},

		{clientID: "id1", topic: packets.Topic{Name: "name1", SubOptions: packets.SubOptions{Qos: packets.Qos1}}},

		{clientID: "id2", topic: packets.Topic{Name: "name2", SubOptions: packets.SubOptions{Qos: packets.Qos2}}},

		{clientID: "id3", topic: packets.Topic{Name: "name3", SubOptions: packets.SubOptions{Qos: packets.Qos2}}},

		{clientID: "id4", topic: packets.Topic{Name: "name3", SubOptions: packets.SubOptions{Qos: packets.Qos2}}},
		{clientID: "id4", topic: packets.Topic{Name: "name4", SubOptions: packets.SubOptions{Qos: packets.Qos2}}},
	}
	for _, v := range tt {
		db.Subscribe(v.clientID, subscription.FromTopic(v.topic, 0))
	}
	stats := db.GetStats()
	a.EqualValues(len(tt), stats.SubscriptionsTotal)
	a.EqualValues(len(tt), stats.SubscriptionsCurrent)

	// If subscribe duplicated topic, total and current statistics should not increase
	db.Subscribe("id0", subscription.FromTopic(packets.Topic{SubOptions: packets.SubOptions{Qos: packets.Qos0}, Name: "name0"}, 0))
	stats = db.GetStats()
	a.EqualValues(len(tt), stats.SubscriptionsTotal)
	a.EqualValues(len(tt), stats.SubscriptionsCurrent)

	utt := []struct {
		clientID string
		topic    packets.Topic
	}{
		{clientID: "id0", topic: packets.Topic{Name: "name0", SubOptions: packets.SubOptions{Qos: packets.Qos0}}},
		{clientID: "id1", topic: packets.Topic{Name: "name1", SubOptions: packets.SubOptions{Qos: packets.Qos1}}},
	}
	for _, v := range utt {
		db.Unsubscribe(v.clientID, v.topic.Name)
	}
	stats = db.GetStats()
	a.EqualValues(len(tt), stats.SubscriptionsTotal)
	a.EqualValues(len(tt)-len(utt), stats.SubscriptionsCurrent)

	//if unsubscribe not exists topic, current statistics should not decrease
	db.Unsubscribe("id0", "name555")
	stats = db.GetStats()
	a.EqualValues(len(tt), stats.SubscriptionsTotal)
	a.EqualValues(len(tt)-len(utt), stats.SubscriptionsCurrent)

	db.UnsubscribeAll("id4")
	stats = db.GetStats()
	a.EqualValues(len(tt), stats.SubscriptionsTotal)
	a.EqualValues(len(tt)-len(utt)-2, stats.SubscriptionsCurrent)
}

func TestTrieDB_GetClientStats(t *testing.T) {
	a := assert.New(t)
	db := NewStore()
	tt := []struct {
		clientID string
		topic    packets.Topic
	}{
		{clientID: "id0", topic: packets.Topic{Name: "name0", SubOptions: packets.SubOptions{Qos: packets.Qos0}}},
		{clientID: "id0", topic: packets.Topic{Name: "name1", SubOptions: packets.SubOptions{Qos: packets.Qos1}}},
		{clientID: "id1", topic: packets.Topic{Name: "name2", SubOptions: packets.SubOptions{Qos: packets.Qos2}}},
		{clientID: "id1", topic: packets.Topic{Name: "name3", SubOptions: packets.SubOptions{Qos: packets.Qos2}}},
		{clientID: "id2", topic: packets.Topic{Name: "name4", SubOptions: packets.SubOptions{Qos: packets.Qos2}}},
		{clientID: "id2", topic: packets.Topic{Name: "name5", SubOptions: packets.SubOptions{Qos: packets.Qos2}}},
	}
	for _, v := range tt {
		db.Subscribe(v.clientID, subscription.FromTopic(v.topic, 0))
	}
	stats, _ := db.GetClientStats("id0")
	a.EqualValues(2, stats.SubscriptionsTotal)
	a.EqualValues(2, stats.SubscriptionsCurrent)

	db.UnsubscribeAll("id0")
	stats, _ = db.GetClientStats("id0")
	a.EqualValues(2, stats.SubscriptionsTotal)
	a.EqualValues(0, stats.SubscriptionsCurrent)
}

func TestTrieDB_GetClientSubscriptions(t *testing.T) {
	a := assert.New(t)
	db := NewStore()
	tt := []struct {
		clientID string
		topic    packets.Topic
	}{
		{clientID: "id0", topic: packets.Topic{Name: "name0", SubOptions: packets.SubOptions{Qos: packets.Qos0}}},
		{clientID: "id0", topic: packets.Topic{Name: "name1", SubOptions: packets.SubOptions{Qos: packets.Qos1}}},
		{clientID: "id1", topic: packets.Topic{Name: "name2", SubOptions: packets.SubOptions{Qos: packets.Qos2}}},
		{clientID: "id1", topic: packets.Topic{Name: "name3", SubOptions: packets.SubOptions{Qos: packets.Qos2}}},
		{clientID: "id2", topic: packets.Topic{Name: "name4", SubOptions: packets.SubOptions{Qos: packets.Qos2}}},
		{clientID: "id2", topic: packets.Topic{Name: "name5", SubOptions: packets.SubOptions{Qos: packets.Qos2}}},
	}
	for _, v := range tt {
		db.Subscribe(v.clientID, subscription.FromTopic(v.topic, 0))
	}
	expected := []subscription.Subscription{
		subscription.FromTopic(packets.Topic{Name: "name0", SubOptions: packets.SubOptions{Qos: packets.Qos0}}, 0),
		subscription.FromTopic(packets.Topic{Name: "name1", SubOptions: packets.SubOptions{Qos: packets.Qos1}}, 0),
	}
	rs := subscription.GetClientSubscriptions(db, "id0", subscription.TypeAll)
	a.ElementsMatch(expected, rs)
	// not exists
	rs = subscription.GetClientSubscriptions(db, "id5", subscription.TypeAll)
	a.Nil(rs)
}
