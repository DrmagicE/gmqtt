package gmqtt

import (
	"testing"

	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/stretchr/testify/assert"
)

func TestTrieDB_deleteAll(t *testing.T) {
	a := assert.New(t)
	db := newTrieDB(&subscriptionStats{})
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
		db.subscribe(v.clientID, v.topic)
	}
	removedCid := "id0"
	db.deleteAll(removedCid)
	for _, v := range tt {
		if v.clientID == removedCid {
			rs := db.getMatchedTopicFilter(v.topic.Name)
			if _, ok := rs[v.clientID]; ok {
				t.Fatalf("remove error")
			}
			continue
		}
		got := db.topicIndex[v.clientID][v.topic.Name]
		a.Equal(got.topicName, v.topic.Name)
		a.Equal(got.clients[v.clientID], v.topic.Qos)

		rs := db.getMatchedTopicFilter(v.topic.Name)
		a.Equal(rs[v.clientID][0].Qos, v.topic.Qos)
	}
}

func TestTrieDB_subscribe_unsubscribe(t *testing.T) {
	a := assert.New(t)
	db := newTrieDB(&subscriptionStats{})
	stt := []struct {
		clientID string
		topic    packets.Topic
	}{
		{clientID: "id0", topic: packets.Topic{Name: "name0", Qos: packets.QOS_0}},
		{clientID: "id1", topic: packets.Topic{Name: "name1", Qos: packets.QOS_1}},
		{clientID: "id2", topic: packets.Topic{Name: "name2", Qos: packets.QOS_2}},
		{clientID: "id0", topic: packets.Topic{Name: "name3", Qos: packets.QOS_2}},
	}
	utt := []struct {
		topicName string
		clientID  string
	}{
		{topicName: "name0", clientID: "id0"}, {topicName: "name1", clientID: "id1"},
	}
	ugot := []struct {
		topicName string
		clientID  string
		topic     packets.Topic
	}{
		{topicName: "name2", clientID: "id2", topic: packets.Topic{Name: "name2", Qos: packets.QOS_2}},
		{topicName: "name3", clientID: "id0", topic: packets.Topic{Name: "name3", Qos: packets.QOS_2}},
	}
	for _, v := range stt {
		db.subscribe(v.clientID, v.topic)
	}
	for _, v := range stt {
		got := db.topicIndex[v.clientID][v.topic.Name]
		a.Equal(got.clients[v.clientID], v.topic.Qos)

		rs := db.getMatchedTopicFilter(v.topic.Name)
		a.Equal(rs[v.clientID][0].Qos, v.topic.Qos)
	}
	for _, v := range utt {
		db.unsubscribe(v.clientID, v.topicName)
	}

	for _, v := range ugot {
		got := db.topicIndex[v.clientID][v.topicName]
		a.Equal(got.clients[v.clientID], v.topic.Qos)
		rs := db.getMatchedTopicFilter(v.topicName)
		a.Equal(rs[v.clientID][0].Qos, v.topic.Qos)
	}
}
