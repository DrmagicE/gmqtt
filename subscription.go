package gmqtt

import (
	"github.com/DrmagicE/gmqtt/pkg/packets"
)

type Subscription struct {
	// ShareName is the share name of a shared subscription.
	// set to "" if it is a non-shared subscription.
	ShareName string
	// TopicFilter is the topic filter which does not include the share name.
	TopicFilter string
	// ID is the subscription identifier
	ID                uint32
	QoS               packets.QoS
	NoLocal           bool
	RetainAsPublished bool
	RetainHandling    byte
}
