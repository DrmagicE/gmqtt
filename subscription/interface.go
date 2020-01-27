package subscription

import (
	"github.com/DrmagicE/gmqtt/pkg/packets"
)

type IterateFn func(clientID string, topic packets.Topic) bool

type SubscribeResult = []struct {
	Topic          packets.Topic
	AlreadyExisted bool
}

type Stats struct {
	SubscriptionsTotal   uint64
	SubscriptionsCurrent uint64
}
type ClientTopics map[string][]packets.Topic

type Store interface {
	Subscribe(clientID string, topics ...packets.Topic) (rs SubscribeResult)
	Unsubscribe(clientID string, topics ...string)
	UnsubscribeAll(clientID string)
	Iterate(fn IterateFn)
	Get(topicFilter string) ClientTopics
	GetTopicMatched(topicName string) ClientTopics
	GetClientSubscriptions(clientID string) []packets.Topic
	StatsReader
}

type StatsReader interface {
	GetStats() Stats
	GetClientStats(clientID string) (Stats, error)
}
