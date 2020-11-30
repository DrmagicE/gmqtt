package config

import (
	"fmt"
	"time"

	"github.com/DrmagicE/gmqtt/pkg/packets"
)

const (
	Overlap  = "overlap"
	OnlyOnce = "onlyonce"
)

var (
	// DefaultMQTTConfig
	DefaultMQTTConfig = MQTT{
		SessionExpiry:              2 * time.Hour,
		SessionExpiryCheckInterval: 20 * time.Second,
		MessageExpiry:              2 * time.Hour,
		MaxPacketSize:              packets.MaximumSize,
		ReceiveMax:                 100,
		MaxKeepAlive:               60,
		TopicAliasMax:              10,
		SubscriptionIDAvailable:    true,
		SharedSubAvailable:         true,
		WildcardAvailable:          true,
		RetainAvailable:            true,
		MaxQueuedMsg:               1000,
		MaximumQoS:                 2,
		QueueQos0Msg:               true,
		DeliveryMode:               OnlyOnce,
		AllowZeroLenClientID:       true,
	}
)

type MQTT struct {
	SessionExpiry              time.Duration `yaml:"session_expiry"`
	SessionExpiryCheckInterval time.Duration `yaml:"session_expiry_check_Interval"`
	MessageExpiry              time.Duration `yaml:"message_expiry"`
	MaxPacketSize              uint32        `yaml:"max_packet_size"`
	ReceiveMax                 uint16        `yaml:"server_receive_maximum"`
	MaxKeepAlive               uint16        `yaml:"max_keepalive"`
	TopicAliasMax              uint16        `yaml:"topic_alias_maximum"`
	SubscriptionIDAvailable    bool          `yaml:"subscription_identifier_available"`
	SharedSubAvailable         bool          `yaml:"shared_subscription_available"`
	WildcardAvailable          bool          `yaml:"wildcard_subscription_available"`
	RetainAvailable            bool          `yaml:"retain_available"`
	MaxQueuedMsg               int           `yaml:"max_queued_messages"`
	MaximumQoS                 uint8         `yaml:"maximum_qos"`
	QueueQos0Msg               bool          `yaml:"queue_qos0_messages"`
	DeliveryMode               string        `yaml:"delivery_mode"`
	AllowZeroLenClientID       bool          `yaml:"allow_zero_length_clientid"`
}

func (c MQTT) Validate() error {
	if c.MaximumQoS > packets.Qos2 {
		return fmt.Errorf("invalid maximum_qos: %d", c.MaximumQoS)
	}
	if c.MaxQueuedMsg <= 0 {
		return fmt.Errorf("invalid max_queued_messages : %d", c.MaxQueuedMsg)
	}
	if c.ReceiveMax == 0 {
		return fmt.Errorf("server_receive_maximum cannot be 0")
	}
	if c.MaxPacketSize == 0 {
		return fmt.Errorf("max_packet_size cannot be 0")
	}
	if c.DeliveryMode != Overlap && c.DeliveryMode != OnlyOnce {
		return fmt.Errorf("invalid delivery_mode: %s", c.DeliveryMode)
	}
	return nil
}
