package server

import (
	"context"

	"github.com/DrmagicE/gmqtt/config"
)

// HookWrapper groups all hook wrappers function
type HookWrapper struct {
	OnBasicAuthWrapper         OnBasicAuthWrapper
	OnEnhancedAuthWrapper      OnEnhancedAuthWrapper
	OnConnectedWrapper         OnConnectedWrapper
	OnReAuthWrapper            OnReAuthWrapper
	OnSessionCreatedWrapper    OnSessionCreatedWrapper
	OnSessionResumedWrapper    OnSessionResumedWrapper
	OnSessionTerminatedWrapper OnSessionTerminatedWrapper
	OnSubscribeWrapper         OnSubscribeWrapper
	OnSubscribedWrapper        OnSubscribedWrapper
	OnUnsubscribedWrapper      OnUnsubscribedWrapper
	OnMsgArrivedWrapper        OnMsgArrivedWrapper
	OnMsgDroppedWrapper        OnMsgDroppedWrapper
	OnDeliverWrapper           OnDeliverWrapper
	OnCloseWrapper             OnCloseWrapper
	OnAcceptWrapper            OnAcceptWrapper
	OnStopWrapper              OnStopWrapper
}

// NewPlugin is the constructor of a plugin. The context is used for sharing information between plugins.
type NewPlugin func(ctx context.Context, config config.Config) (Plugable, error)

// Plugable is the interface need to be implemented for every plugins.
type Plugable interface {
	// Load will be called in server.Run(). If return error, the server will panic.
	Load(service Server) error
	// Unload will be called when the server is shutdown, the return error is only for logging
	Unload() error
	// HookWrapper returns all hook wrappers that used by the plugin.
	// Return a empty wrapper  if the plugin does not need any hooks
	HookWrapper() HookWrapper
	// Name return the plugin name
	Name() string
}
