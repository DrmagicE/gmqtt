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
		{clientID: "id0", topic: packets.Topic{Name: "name0", Qos: packets.QOS_0}},
		{clientID: "id1", topic: packets.Topic{Name: "name1", Qos: packets.QOS_1}},
		{clientID: "id2", topic: packets.Topic{Name: "name2", Qos: packets.QOS_2}},
		{clientID: "id0", topic: packets.Topic{Name: "name3", Qos: packets.QOS_2}},
	}

	for _, v := range tt {
		db.Subscribe(v.clientID, v.topic)
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
		a.Equal(got.clients[v.clientID], v.topic.Qos)

		rs := db.getMatchedTopicFilter(v.topic.Name)
		a.Equal(rs[v.clientID][0].Qos, v.topic.Qos)
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
		{clientID: "id0", topic: packets.Topic{Name: "name0", Qos: packets.QOS_0}, alreadyExisted: false},
		{clientID: "id1", topic: packets.Topic{Name: "name1", Qos: packets.QOS_1}, alreadyExisted: false},
		{clientID: "id2", topic: packets.Topic{Name: "name2", Qos: packets.QOS_2}, alreadyExisted: false},
		{clientID: "id0", topic: packets.Topic{Name: "name3", Qos: packets.QOS_2}, alreadyExisted: false},
		{clientID: "id0", topic: packets.Topic{Name: "name3", Qos: packets.QOS_2}, alreadyExisted: true},
	}
	utt := []struct {
		clientID  string
		topicName string
	}{
		{clientID: "id0", topicName: "name0"}, {clientID: "id1", topicName: "name1"},
	}
	for _, v := range stt {
		rs := db.Subscribe(v.clientID, v.topic)
		a.Equal(v.alreadyExisted, rs[0].AlreadyExisted)
	}
	for _, v := range utt {
		db.Unsubscribe(v.clientID, v.topicName)
	}

	expected := []struct {
		clientID string
		topic    packets.Topic
	}{
		{clientID: "id2", topic: packets.Topic{Name: "name2", Qos: packets.QOS_2}},
		{clientID: "id0", topic: packets.Topic{Name: "name3", Qos: packets.QOS_2}},
	}

	var rs []struct {
		clientID string
		topic    packets.Topic
	}
	db.Iterate(func(clientID string, topic packets.Topic) bool {
		rs = append(rs, struct {
			clientID string
			topic    packets.Topic
		}{clientID: clientID, topic: topic})
		return true
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
		{clientID: "id0", topic: packets.Topic{Name: "name0", Qos: packets.QOS_0}},
		{clientID: "id1", topic: packets.Topic{Name: "name1", Qos: packets.QOS_1}},
		{clientID: "id2", topic: packets.Topic{Name: "name2", Qos: packets.QOS_2}},
		{clientID: "id3", topic: packets.Topic{Name: "name3", Qos: packets.QOS_2}},
	}
	for _, v := range tt {
		db.Subscribe(v.clientID, v.topic)
	}
	var rs []struct {
		clientID string
		topic    packets.Topic
	}
	db.Iterate(func(clientID string, topic packets.Topic) bool {
		rs = append(rs, struct {
			clientID string
			topic    packets.Topic
		}{
			clientID: clientID,
			topic:    topic,
		})
		return true
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
		{clientID: "id0", topic: packets.Topic{Name: "name0", Qos: packets.QOS_0}},
		{clientID: "id1", topic: packets.Topic{Name: "name1", Qos: packets.QOS_1}},
		{clientID: "id2", topic: packets.Topic{Name: "name2", Qos: packets.QOS_2}},
		{clientID: "id3", topic: packets.Topic{Name: "name3", Qos: packets.QOS_2}},
		{clientID: "id4", topic: packets.Topic{Name: "name3/name4/name5", Qos: packets.QOS_2}},
	}
	for _, v := range tt {
		db.Subscribe(v.clientID, v.topic)
	}
	var expected = subscription.ClientTopics{
		"id3": {packets.Topic{
			Qos:  packets.QOS_2,
			Name: "name3",
		}},
	}

	rs := db.Get("name3")
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
		{clientID: "id0", topic: packets.Topic{Name: "/a/b/c", Qos: packets.QOS_0}},
		{clientID: "id1", topic: packets.Topic{Name: "/a/b/+", Qos: packets.QOS_1}},
		{clientID: "id2", topic: packets.Topic{Name: "/a/b/c/d", Qos: packets.QOS_2}},
	}
	for _, v := range tt {
		db.Subscribe(v.clientID, v.topic)
	}
	var expected = subscription.ClientTopics{
		"id0": {{
			Qos:  packets.QOS_0,
			Name: "/a/b/c",
		}},
		"id1": {{
			Qos:  packets.QOS_1,
			Name: "/a/b/+",
		}},
	}

	rs := db.GetTopicMatched("/a/b/c")
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
		{clientID: "id0", topic: packets.Topic{Name: "name0", Qos: packets.QOS_0}},

		{clientID: "id1", topic: packets.Topic{Name: "name1", Qos: packets.QOS_1}},

		{clientID: "id2", topic: packets.Topic{Name: "name2", Qos: packets.QOS_2}},

		{clientID: "id3", topic: packets.Topic{Name: "name3", Qos: packets.QOS_2}},

		{clientID: "id4", topic: packets.Topic{Name: "name3", Qos: packets.QOS_2}},
		{clientID: "id4", topic: packets.Topic{Name: "name4", Qos: packets.QOS_2}},
	}
	for _, v := range tt {
		db.Subscribe(v.clientID, v.topic)
	}
	stats := db.GetStats()
	a.EqualValues(len(tt), stats.SubscriptionsTotal)
	a.EqualValues(len(tt), stats.SubscriptionsCurrent)

	// If subscribe duplicated topic, total and current statistics should not increase
	db.Subscribe("id0", packets.Topic{packets.QOS_0, "name0"})
	stats = db.GetStats()
	a.EqualValues(len(tt), stats.SubscriptionsTotal)
	a.EqualValues(len(tt), stats.SubscriptionsCurrent)

	utt := []struct {
		clientID string
		topic    packets.Topic
	}{
		{clientID: "id0", topic: packets.Topic{Name: "name0", Qos: packets.QOS_0}},
		{clientID: "id1", topic: packets.Topic{Name: "name1", Qos: packets.QOS_1}},
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
		{clientID: "id0", topic: packets.Topic{Name: "name0", Qos: packets.QOS_0}},
		{clientID: "id0", topic: packets.Topic{Name: "name1", Qos: packets.QOS_1}},
		{clientID: "id1", topic: packets.Topic{Name: "name2", Qos: packets.QOS_2}},
		{clientID: "id1", topic: packets.Topic{Name: "name3", Qos: packets.QOS_2}},
		{clientID: "id2", topic: packets.Topic{Name: "name4", Qos: packets.QOS_2}},
		{clientID: "id2", topic: packets.Topic{Name: "name5", Qos: packets.QOS_2}},
	}
	for _, v := range tt {
		db.Subscribe(v.clientID, v.topic)
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
		{clientID: "id0", topic: packets.Topic{Name: "name0", Qos: packets.QOS_0}},
		{clientID: "id0", topic: packets.Topic{Name: "name1", Qos: packets.QOS_1}},
		{clientID: "id1", topic: packets.Topic{Name: "name2", Qos: packets.QOS_2}},
		{clientID: "id1", topic: packets.Topic{Name: "name3", Qos: packets.QOS_2}},
		{clientID: "id2", topic: packets.Topic{Name: "name4", Qos: packets.QOS_2}},
		{clientID: "id2", topic: packets.Topic{Name: "name5", Qos: packets.QOS_2}},
	}
	for _, v := range tt {
		db.Subscribe(v.clientID, v.topic)
	}
	expected := []packets.Topic{
		{Name: "name0", Qos: packets.QOS_0},
		{Name: "name1", Qos: packets.QOS_1},
	}
	rs := db.GetClientSubscriptions("id0")
	a.ElementsMatch(expected, rs)
	// not exists
	rs = db.GetClientSubscriptions("id5")
	a.Nil(rs)
}
