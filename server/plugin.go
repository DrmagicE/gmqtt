package server

import (
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
	OnUnsubscribeWrapper       OnUnsubscribeWrapper
	OnUnsubscribedWrapper      OnUnsubscribedWrapper
	OnMsgArrivedWrapper        OnMsgArrivedWrapper
	OnMsgDroppedWrapper        OnMsgDroppedWrapper
	OnDeliveredWrapper         OnDeliveredWrapper
	OnClosedWrapper            OnClosedWrapper
	OnAcceptWrapper            OnAcceptWrapper
	OnStopWrapper              OnStopWrapper
	OnWillPublishWrapper       OnWillPublishWrapper
	OnWillPublishedWrapper     OnWillPublishedWrapper
}

// NewPlugin is the constructor of a plugin.
type NewPlugin func(config config.Config) (Plugin, error)

// Plugin is the interface need to be implemented for every plugins.
type Plugin interface {
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
