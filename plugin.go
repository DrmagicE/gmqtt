package gmqtt

// HookWrapper groups all hook wrappers function
type HookWrapper struct {
	OnConnectWrapper           OnConnectWrapper
	OnConnectedWrapper         OnConnectedWrapper
	OnSessionCreatedWrapper    OnSessionCreatedWrapper
	OnSessionResumedWrapper    OnSessionResumedWrapper
	OnSessionTerminatedWrapper OnSessionTerminatedWrapper
	OnSubscribeWrapper         OnSubscribeWrapper
	OnSubscribedWrapper        OnSubscribedWrapper
	OnUnsubscribedWrapper      OnUnsubscribedWrapper
	OnMsgArrivedWrapper        OnMsgArrivedWrapper
	OnAckedWrapper             OnAckedWrapper
	OnMsgDroppedWrapper        OnMsgDroppedWrapper
	OnDeliverWrapper           OnDeliverWrapper
	OnCloseWrapper             OnCloseWrapper
	OnAcceptWrapper            OnAcceptWrapper
	OnStopWrapper              OnStopWrapper
}

// Plugable is the interface for every plugins
type Plugable interface {
	// Load will be called in server.Run().if return error, the server will panic
	Load(service ServerService) error
	// Unload will be called when the server is shutdown, the return error is only for loggin
	Unload() error
	// HookWrapper returns all hook wrappers that used by the plugin.
	// Set the hook wrapper to nil if the plugin does not need the hook
	HookWrapper() HookWrapper
	// Name return the plugin name
	Name() string
}
