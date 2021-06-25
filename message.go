package gmqtt

import (
	"github.com/DrmagicE/gmqtt/pkg/packets"
)

type Message struct {
	Dup      bool
	QoS      uint8
	Retained bool
	Topic    string
	Payload  []byte
	PacketID packets.PacketID
	// The following fields are introduced in v5 specification.
	// Excepting MessageExpiry, these fields will not take effect when it represents a v3.x publish packet.
	ContentType            string
	CorrelationData        []byte
	MessageExpiry          uint32
	PayloadFormat          packets.PayloadFormat
	ResponseTopic          string
	SubscriptionIdentifier []uint32
	UserProperties         []packets.UserProperty
}

// Copy deep copies the Message and return the new one
func (m *Message) Copy() *Message {
	newMsg := &Message{
		Dup:           m.Dup,
		QoS:           m.QoS,
		Retained:      m.Retained,
		Topic:         m.Topic,
		PacketID:      m.PacketID,
		ContentType:   m.ContentType,
		MessageExpiry: m.MessageExpiry,
		PayloadFormat: m.PayloadFormat,
		ResponseTopic: m.ResponseTopic,
	}
	newMsg.Payload = make([]byte, len(m.Payload))
	copy(newMsg.Payload, m.Payload)

	if len(m.CorrelationData) != 0 {
		newMsg.CorrelationData = make([]byte, len(m.CorrelationData))
		copy(newMsg.CorrelationData, m.CorrelationData)
	}

	if len(m.SubscriptionIdentifier) != 0 {
		newMsg.SubscriptionIdentifier = make([]uint32, len(m.SubscriptionIdentifier))
		copy(newMsg.SubscriptionIdentifier, m.SubscriptionIdentifier)
	}
	if len(m.UserProperties) != 0 {
		newMsg.UserProperties = make([]packets.UserProperty, len(m.UserProperties))
		for k := range newMsg.UserProperties {
			newMsg.UserProperties[k].K = make([]byte, len(m.UserProperties[k].K))
			copy(newMsg.UserProperties[k].K, m.UserProperties[k].K)

			newMsg.UserProperties[k].V = make([]byte, len(m.UserProperties[k].V))
			copy(newMsg.UserProperties[k].V, m.UserProperties[k].V)
		}
	}
	return newMsg

}

func getVariablelenght(l int) int {
	if l <= 127 {
		return 1
	} else if l <= 16383 {
		return 2
	} else if l <= 2097151 {
		return 3
	} else if l <= 268435455 {
		return 4
	}
	return 0
}

// TotalBytes return the publish packets total bytes.
func (m *Message) TotalBytes(version packets.Version) uint32 {
	remainLenght := len(m.Payload) + 2 + len(m.Topic)
	if m.QoS > packets.Qos0 {
		remainLenght += 2
	}
	if version == packets.Version5 {
		propertyLenght := 0
		if m.PayloadFormat == packets.PayloadFormatString {
			propertyLenght += 2
		}
		if l := len(m.ContentType); l != 0 {
			propertyLenght += 3 + l
		}
		if l := len(m.CorrelationData); l != 0 {
			propertyLenght += 3 + l
		}

		for _, v := range m.SubscriptionIdentifier {
			propertyLenght++
			propertyLenght += getVariablelenght(int(v))
		}

		if m.MessageExpiry != 0 {
			propertyLenght += 5
		}
		if l := len(m.ResponseTopic); l != 0 {
			propertyLenght += 3 + l
		}
		for _, v := range m.UserProperties {
			propertyLenght += 5 + len(v.K) + len(v.V)
		}
		remainLenght += propertyLenght + getVariablelenght(propertyLenght)
	}
	if remainLenght <= 127 {
		return 2 + uint32(remainLenght)
	} else if remainLenght <= 16383 {
		return 3 + uint32(remainLenght)
	} else if remainLenght <= 2097151 {
		return 4 + uint32(remainLenght)
	}
	return 5 + uint32(remainLenght)
}

// MessageFromPublish create the Message instance from  publish packets
func MessageFromPublish(p *packets.Publish) *Message {
	m := &Message{
		Dup:      p.Dup,
		QoS:      p.Qos,
		Retained: p.Retain,
		Topic:    string(p.TopicName),
		Payload:  p.Payload,
	}
	if p.Version == packets.Version5 {
		if p.Properties.PayloadFormat != nil {
			m.PayloadFormat = *p.Properties.PayloadFormat
		}
		if l := len(p.Properties.ContentType); l != 0 {
			m.ContentType = string(p.Properties.ContentType)
		}
		if l := len(p.Properties.CorrelationData); l != 0 {
			m.CorrelationData = p.Properties.CorrelationData
		}
		if p.Properties.MessageExpiry != nil {
			m.MessageExpiry = *p.Properties.MessageExpiry
		}
		if l := len(p.Properties.ResponseTopic); l != 0 {
			m.ResponseTopic = string(p.Properties.ResponseTopic)
		}
		m.UserProperties = p.Properties.User

	}
	return m
}

// MessageToPublish create the publish packet instance from *Message
func MessageToPublish(msg *Message, version packets.Version) *packets.Publish {
	pub := &packets.Publish{
		Dup:       msg.Dup,
		Qos:       msg.QoS,
		PacketID:  msg.PacketID,
		Retain:    msg.Retained,
		TopicName: []byte(msg.Topic),
		Payload:   msg.Payload,
		Version:   version,
	}
	if version == packets.Version5 {
		var msgExpiry *uint32
		if e := msg.MessageExpiry; e != 0 {
			msgExpiry = &e
		}
		var contentType []byte
		if msg.ContentType != "" {
			contentType = []byte(msg.ContentType)
		}
		var responseTopic []byte
		if msg.ResponseTopic != "" {
			responseTopic = []byte(msg.ResponseTopic)
		}
		var payloadFormat *byte
		if e := msg.PayloadFormat; e == packets.PayloadFormatString {
			payloadFormat = &e
		}
		pub.Properties = &packets.Properties{
			CorrelationData:        msg.CorrelationData,
			ContentType:            contentType,
			MessageExpiry:          msgExpiry,
			ResponseTopic:          responseTopic,
			PayloadFormat:          payloadFormat,
			User:                   msg.UserProperties,
			SubscriptionIdentifier: msg.SubscriptionIdentifier,
		}
	}
	return pub
}
