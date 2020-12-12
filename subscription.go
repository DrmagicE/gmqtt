package gmqtt

import (
	"errors"

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

// GetFullTopicName returns the full topic name of the subscription.
func (s *Subscription) GetFullTopicName() string {
	if s.ShareName != "" {
		return "$share/" + s.ShareName + "/" + s.TopicFilter
	}
	return s.TopicFilter
}

// Copy makes a copy of subscription.
func (s *Subscription) Copy() *Subscription {
	return &Subscription{
		ShareName:         s.ShareName,
		TopicFilter:       s.TopicFilter,
		ID:                s.ID,
		QoS:               s.QoS,
		NoLocal:           s.NoLocal,
		RetainAsPublished: s.RetainAsPublished,
		RetainHandling:    s.RetainHandling,
	}
}

// Validate returns whether the subscription is valid.
// If you can ensure the subscription is valid then just skip the validation.
func (s *Subscription) Validate() error {
	if !packets.ValidV5Topic([]byte(s.GetFullTopicName())) {
		return errors.New("invalid topic name")
	}
	if s.QoS > 2 {
		return errors.New("invalid qos")
	}
	if s.RetainHandling != 0 && s.RetainHandling != 1 && s.RetainHandling != 2 {
		return errors.New("invalid retain handling")
	}
	return nil
}
