package subscription

import (
	"github.com/DrmagicE/gmqtt/pkg/packets"
)

// IterateFn is the callback function used by Iterate()
// Return false means to stop the iteration.
type IterateFn func(clientID string, topic packets.Topic) bool

// SubscribeResult is the result of Subscribe()
type SubscribeResult = []struct {
	// Topic is the Subscribed topic
	Topic packets.Topic
	// AlreadyExisted shows whether the topic is already existed.
	AlreadyExisted bool
}

// Stats is the statistics information of the store
type Stats struct {
	// SubscriptionsTotal shows how many subscription has been added to the store.
	// Duplicated subscription is not counting.
	SubscriptionsTotal uint64
	// SubscriptionsCurrent shows the current subscription number in the store.
	SubscriptionsCurrent uint64
}

// ClientTopics groups the topics by client id.
type ClientTopics map[string][]packets.Topic

// Store is the interface used by gmqtt.server and external logic to handler the operations of subscriptions.
// User can get the implementation from gmqtt.Server interface.
// This interface provides the ability for extensions to interact with the subscriptions.
// Notice:
// This methods will not trigger any gmqtt hooks.
type Store interface {
	// Subscribe add subscriptions to a specific client.
	// Notice:
	// This method will succeed even if the client is not exists, the subscriptions
	// will affect the new client with the client id.
	Subscribe(clientID string, topics ...packets.Topic) (rs SubscribeResult)
	// Unsubscribe remove subscriptions of a specific client.
	Unsubscribe(clientID string, topics ...string)
	// UnsubscribeAll remove all subscriptions of a specific client.
	UnsubscribeAll(clientID string)
	// Iterate iterate all subscriptions. The callback is called once for each subscription.
	// If callback return false, the iteration will be stopped.
	// Notice:
	// The results are not sorted in any way, no ordering of any kind is guaranteed.
	// This method will walk through all subscriptions,
	// so it is a very expensive operation. Do not call it frequently.
	Iterate(fn IterateFn)
	// Get returns the subscriptions that equals the passed topic filter.
	Get(topicFilter string) ClientTopics
	// GetTopicMatched returns the subscriptions that match the passed topic.
	GetTopicMatched(topicName string) ClientTopics
	// GetClientSubscriptions returns the subscriptions of a specific client.
	GetClientSubscriptions(clientID string) []packets.Topic
	StatsReader
}

// StatsReader provides the ability to get statistics information.
type StatsReader interface {
	// GetStats return the global stats.
	GetStats() Stats
	// GetClientStats return the stats of a specific client.
	// If stats not exists, return an error.
	GetClientStats(clientID string) (Stats, error)
}
