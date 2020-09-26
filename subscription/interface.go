package subscription

import (
	"strings"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/pkg/packets"
)

// IterationType specifies the types of subscription that will be iterated.
type IterationType byte

const (
	// TypeSYS represents system topic, which start with '$'.
	TypeSYS = 1 << iota
	// TypeSYS represents shared topic, which start with '$share/'.
	TypeShared
	// TypeNonShared represents non-shared topic.
	TypeNonShared
	TypeAll = TypeSYS | TypeShared | TypeNonShared
)

type MatchType byte

const (
	MatchName MatchType = iota
	MatchFilter
)

func FromTopic(topic packets.Topic, id uint32) *gmqtt.Subscription {
	var shareName string
	var topicFilter string
	if strings.HasPrefix(topic.Name, "$share/") {
		shared := strings.SplitN(topic.Name, "/", 3)
		shareName = shared[1]
		topicFilter = shared[2]
	} else {
		topicFilter = topic.Name
	}

	s := &gmqtt.Subscription{
		ShareName:         shareName,
		TopicFilter:       topicFilter,
		ID:                id,
		QoS:               topic.Qos,
		NoLocal:           topic.NoLocal,
		RetainAsPublished: topic.RetainAsPublished,
		RetainHandling:    topic.RetainHandling,
	}
	return s
}

// IterateFn is the callback function used by Iterate()
// Return false means to stop the iteration.
type IterateFn func(clientID string, sub *gmqtt.Subscription) bool

// SubscribeResult is the result of Subscribe()
type SubscribeResult = []struct {
	// Topic is the Subscribed topic
	Subscription *gmqtt.Subscription
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

// ClientSubscriptions groups the subscriptions by client id.
type ClientSubscriptions map[string][]*gmqtt.Subscription

// IterationOptions
type IterationOptions struct {
	// Type specifies the types of subscription that will be iterated.
	// For example, if Type = TypeShared | TypeNonShared , then all shared and non-shared subscriptions will be iterated
	Type IterationType
	// ClientID specifies the subscriber client id.
	ClientID string
	// TopicName represents topic filter or topic name. This field works together with MatchType.
	TopicName string
	// MatchType specifies the matching type of the iteration.
	// if MatchName, the IterateFn will be called when the subscription topic filter is equal to TopicName.
	// if MatchTopic,  the IterateFn will be called when the TopicName match the subscription topic filter.
	MatchType MatchType
}

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
	Subscribe(clientID string, subscriptions ...*gmqtt.Subscription) (rs SubscribeResult)
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
	Iterate(fn IterateFn, options IterationOptions)

	StatsReader
}

// GetTopicMatched returns the subscriptions that match the passed topic.
func GetTopicMatched(store Store, topicFilter string, t IterationType) ClientSubscriptions {
	rs := make(ClientSubscriptions)
	store.Iterate(func(clientID string, subscription *gmqtt.Subscription) bool {
		rs[clientID] = append(rs[clientID], subscription)
		return true
	}, IterationOptions{
		Type:      t,
		TopicName: topicFilter,
		MatchType: MatchFilter,
	})
	if len(rs) == 0 {
		return nil
	}
	return rs
}

// Get returns the subscriptions that equals the passed topic filter.
func Get(store Store, topicFilter string, t IterationType) ClientSubscriptions {
	rs := make(ClientSubscriptions)
	store.Iterate(func(clientID string, subscription *gmqtt.Subscription) bool {
		rs[clientID] = append(rs[clientID], subscription)
		return true
	}, IterationOptions{
		Type:      t,
		TopicName: topicFilter,
		MatchType: MatchName,
	})
	if len(rs) == 0 {
		return nil
	}
	return rs
}

// GetClientSubscriptions returns the subscriptions of a specific client.
func GetClientSubscriptions(store Store, clientID string, t IterationType) []*gmqtt.Subscription {
	var rs []*gmqtt.Subscription
	store.Iterate(func(clientID string, subscription *gmqtt.Subscription) bool {
		rs = append(rs, subscription)
		return true
	}, IterationOptions{
		Type:     t,
		ClientID: clientID,
	})
	return rs
}

// StatsReader provides the ability to get statistics information.
type StatsReader interface {
	// GetStats return the global stats.
	GetStats() Stats
	// GetClientStats return the stats of a specific client.
	// If stats not exists, return an error.
	GetClientStats(clientID string) (Stats, error)
}
