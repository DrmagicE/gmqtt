package server

import (
	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/persistence/session"
	"github.com/DrmagicE/gmqtt/persistence/subscription"
	"github.com/DrmagicE/gmqtt/retained"
)

// Publisher provides the ability to Publish messages to the broker.
type Publisher interface {
	// Publish Publish a message to broker.
	// Calling this method will not trigger OnMsgArrived hook.
	Publish(message *gmqtt.Message)
}

// ClientIterateFn is the callback function used by ClientService.IterateClient
// Return false means to stop the iteration.
type ClientIterateFn = func(client Client) bool

// ClientService provides the ability to query and close clients.
type ClientService interface {
	IterateSession(fn session.IterateFn) error
	GetSession(clientID string) (*gmqtt.Session, error)
	GetClient(clientID string) Client
	IterateClient(fn ClientIterateFn)
	TerminateSession(clientID string)
}

// SubscriptionService providers the ability to query and add/delete subscriptions.
type SubscriptionService interface {
	// Subscribe adds subscriptions to a specific client.
	// Notice:
	// This method will succeed even if the client is not exists, the subscriptions
	// will affect the new client with the client id.
	Subscribe(clientID string, subscriptions ...*gmqtt.Subscription) (rs subscription.SubscribeResult, err error)
	// Unsubscribe removes subscriptions of a specific client.
	Unsubscribe(clientID string, topics ...string) error
	// UnsubscribeAll removes all subscriptions of a specific client.
	UnsubscribeAll(clientID string) error
	// Iterate iterates all subscriptions. The callback is called once for each subscription.
	// If callback return false, the iteration will be stopped.
	// Notice:
	// The results are not sorted in any way, no ordering of any kind is guaranteed.
	// This method will walk through all subscriptions,
	// so it is a very expensive operation. Do not call it frequently.
	Iterate(fn subscription.IterateFn, options subscription.IterationOptions)
	subscription.StatsReader
}

// RetainedService providers the ability to query and add/delete retained messages.
type RetainedService interface {
	retained.Store
}
