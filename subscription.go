package gmqtt

import (
	"github.com/DrmagicE/gmqtt/pkg/packets"
)

// Subscription represents a subscription in gmqtt.
type Subscription struct {
	// ShareName is the share name of a shared subscription.
	// set to "" if it is a non-shared subscription.
	ShareName string
	// TopicFilter is the topic filter which does not include the share name.
	TopicFilter string
	// ID is the subscription identifier
	ID uint32
	// The following fields are Subscription Options.
	// See: https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901169

	// QoS is the qos level of the Subscription.
	QoS packets.QoS
	// NoLocal is the No Local option.
	NoLocal bool
	// RetainAsPublished is the Retain As Published option.
	RetainAsPublished bool
	// RetainHandling the Retain Handling option.
	RetainHandling byte
}
