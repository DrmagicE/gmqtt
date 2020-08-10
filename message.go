package gmqtt

import (
	"github.com/DrmagicE/gmqtt/pkg/packets"
)

// msg is the implementation of Message interface
type msg struct {
	dup      bool
	qos      uint8
	retained bool
	topic    string
	packetID packets.PacketID
	payload  []byte

	contentType     string
	correlationData []byte
	messageExpiry   uint32
	payloadFormat   packets.PayloadFormat
	responseTopic   string
	userProperties  []packets.UserProperty
}

func (m *msg) ContentType() string {
	return m.contentType
}

func (m *msg) CorrelationData() []byte {
	return m.correlationData
}

func (m *msg) MessageExpiry() uint32 {
	return m.messageExpiry
}

func (m *msg) PayloadFormat() packets.PayloadFormat {
	return m.payloadFormat
}

func (m *msg) ResponseTopic() string {
	return m.responseTopic
}

func (m *msg) UserProperties() []packets.UserProperty {
	return m.userProperties
}

func (m *msg) Dup() bool {
	return m.dup
}

func (m *msg) Qos() uint8 {
	return m.qos
}

func (m *msg) Retained() bool {
	return m.retained
}

func (m *msg) Topic() string {
	return m.topic
}

func (m *msg) PacketID() packets.PacketID {
	return m.packetID
}

func (m *msg) Payload() []byte {
	return m.payload
}

func messageFromPublish(p *packets.Publish) *msg {
	m := &msg{
		dup:      p.Dup,
		qos:      p.Qos,
		retained: p.Retain,
		topic:    string(p.TopicName),
		packetID: p.PacketID,
		payload:  p.Payload,
	}

	if p.Version == packets.Version5 {
		if p.Properties.PayloadFormat != nil {
			m.payloadFormat = *p.Properties.PayloadFormat
		}

		m.contentType = string(p.Properties.ContentType)
		m.correlationData = p.Properties.CorrelationData

		if p.Properties.MessageExpiry != nil {
			m.messageExpiry = *p.Properties.MessageExpiry
		}
		m.responseTopic = string(p.Properties.ResponseTopic)
		m.userProperties = p.Properties.User
	}
	return m
}

func messageToPublish(msg packets.Message, version packets.Version) *packets.Publish {
	pub := &packets.Publish{
		Dup:       msg.Dup(),
		Qos:       msg.Qos(),
		Retain:    msg.Retained(),
		TopicName: []byte(msg.Topic()),
		PacketID:  msg.PacketID(),
		Payload:   msg.Payload(),
		Version:   version,
	}
	if version == packets.Version5 {
		var msgExpiry *uint32
		if e := msg.MessageExpiry(); e != 0 {
			msgExpiry = &e
		}
		var contentType []byte
		if msg.ContentType() != "" {
			contentType = []byte(msg.ContentType())
		}
		var responseTopic []byte
		if msg.ResponseTopic() != "" {
			responseTopic = []byte(msg.ResponseTopic())
		}
		var payloadFormat *byte
		if e := msg.PayloadFormat(); e == packets.PayloadFormatString {
			payloadFormat = &e
		}
		pub.Properties = &packets.Properties{
			CorrelationData: msg.CorrelationData(),
			ContentType:     contentType,
			MessageExpiry:   msgExpiry,
			ResponseTopic:   responseTopic,
			PayloadFormat:   payloadFormat,
			User:            msg.UserProperties(),
		}
	}
	return pub
}

type msgOptions func(msg *msg)

// Retained sets retained flag to the message
func Retained(retained bool) msgOptions {
	return func(msg *msg) {
		msg.retained = retained
	}
}

func ContentType(contentType string) msgOptions {
	return func(msg *msg) {
		msg.contentType = contentType
	}
}

func CorrelationData(b []byte) msgOptions {
	return func(msg *msg) {
		msg.correlationData = b
	}
}

func MessageExpiry(u uint32) msgOptions {
	return func(msg *msg) {
		msg.messageExpiry = u
	}
}

func PayloadFormat(format packets.PayloadFormat) msgOptions {
	return func(msg *msg) {
		msg.payloadFormat = format
	}
}

func ResponseTopic(topic string) msgOptions {
	return func(msg *msg) {
		msg.responseTopic = topic
	}
}

func UserProperties(userProperties []packets.UserProperty) msgOptions {
	return func(msg *msg) {
		msg.userProperties = userProperties
	}
}

// NewMessage creates a message for publish service.
func NewMessage(topic string, payload []byte, qos uint8, opts ...msgOptions) packets.Message {
	m := &msg{
		topic:   topic,
		qos:     qos,
		payload: payload,
	}
	for _, v := range opts {
		v(m)
	}
	return m
}
