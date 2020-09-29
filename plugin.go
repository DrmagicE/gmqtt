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
	OnUnsubscribeWrapper       OnUnsubscribeWrapper
	OnUnsubscribedWrapper      OnUnsubscribedWrapper
	OnMsgArrivedWrapper        OnMsgArrivedWrapper
	OnAckedWrapper             OnAckedWrapper
	OnMsgDroppedWrapper        OnMsgDroppedWrapper
	OnDeliverWrapper           OnDeliverWrapper
	OnCloseWrapper             OnCloseWrapper
	OnAcceptWrapper            OnAcceptWrapper
	OnStopWrapper              OnStopWrapper
}

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
