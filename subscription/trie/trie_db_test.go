package trie

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/DrmagicE/gmqtt/subscription"
)

// TODO move to TestSuite
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

// TODO move to TestSuite
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

func TestSuite(t *testing.T) {
	store := NewStore()
	subscription.TestSuite(t, store)
}
