package gmqtt

type Subscription interface {
	// ShareName is the share name of a shared subscription.
	// If it is a non-shared subscription, return ""
	ShareName() string
	// TopicFilter return the topic filter which does not include the share name
	TopicFilter() string
	ID() uint32 // subscription identifier
	SubOpts
}
type SubOpts interface {
	QoS() byte
	NoLocal() bool
	RetainAsPublished() bool
	RetainHandling() byte
}
