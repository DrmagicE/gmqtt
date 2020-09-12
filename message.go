package gmqtt

import (
	"github.com/DrmagicE/gmqtt/pkg/packets"
)

// Msg is the implementation of Message interface
type Msg struct {
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

	totalBytes uint32
}

func (m *Msg) TotalBytes() uint32 {
	return m.totalBytes
}

func (m *Msg) ContentType() string {
	return m.contentType
}

func (m *Msg) CorrelationData() []byte {
	return m.correlationData
}

func (m *Msg) MessageExpiry() uint32 {
	return m.messageExpiry
}

func (m *Msg) PayloadFormat() packets.PayloadFormat {
	return m.payloadFormat
}

func (m *Msg) ResponseTopic() string {
	return m.responseTopic
}

func (m *Msg) UserProperties() []packets.UserProperty {
	return m.userProperties
}

func (m *Msg) Dup() bool {
	return m.dup
}

func (m *Msg) Qos() uint8 {
	return m.qos
}

func (m *Msg) Retained() bool {
	return m.retained
}

func (m *Msg) Topic() string {
	return m.topic
}

func (m *Msg) PacketID() packets.PacketID {
	return m.packetID
}

func (m *Msg) Payload() []byte {
	return m.payload
}

func MessageFromPublish(p *packets.Publish) *Msg {
	m := &Msg{
		dup:      p.Dup,
		qos:      p.Qos,
		retained: p.Retain,
		topic:    string(p.TopicName),
		packetID: p.PacketID,
		payload:  p.Payload,
	}
	remainLenght := len(p.Payload) + 2 + len(p.TopicName)
	if p.Qos > packets.Qos0 {
		remainLenght += 2
	}
	if p.Version == packets.Version5 {
		propertyLenght := 0
		if p.Properties.PayloadFormat != nil {
			m.payloadFormat = *p.Properties.PayloadFormat
			propertyLenght += 2
		}
		if l := len(p.Properties.ContentType); l != 0 {
			propertyLenght += 3 + l
			m.contentType = string(p.Properties.ContentType)
		}
		if l := len(p.Properties.CorrelationData); l != 0 {
			propertyLenght += 3 + l
			m.correlationData = p.Properties.CorrelationData
		}
		if p.Properties.MessageExpiry != nil {
			m.messageExpiry = *p.Properties.MessageExpiry
			propertyLenght += 5
		}
		if l := len(p.Properties.ResponseTopic); l != 0 {
			propertyLenght += 3 + l
			m.responseTopic = string(p.Properties.ResponseTopic)
		}
		for _, v := range p.Properties.User {
			propertyLenght += 5 + len(v.K) + len(v.V)
		}
		m.userProperties = p.Properties.User

		if propertyLenght <= 127 {
			propertyLenght++
		} else if propertyLenght <= 16383 {
			propertyLenght += 2
		} else if propertyLenght <= 2097151 {
			propertyLenght += 3
		} else if propertyLenght <= 268435455 {
			propertyLenght += 4
		}
		remainLenght += propertyLenght
	}
	if remainLenght <= 127 {
		m.totalBytes = 2 + uint32(remainLenght)
	} else if remainLenght <= 16383 {
		m.totalBytes = 3 + uint32(remainLenght)
	} else if remainLenght <= 2097151 {
		m.totalBytes = 4 + uint32(remainLenght)
	} else if remainLenght <= 268435455 {
		m.totalBytes = 5 + uint32(remainLenght)
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

type msgOptions func(msg *Msg)

// Retained sets retained flag to the message
func Retained(retained bool) msgOptions {
	return func(msg *Msg) {
		msg.retained = retained
	}
}

func ContentType(contentType string) msgOptions {
	return func(msg *Msg) {
		msg.contentType = contentType
	}
}

func CorrelationData(b []byte) msgOptions {
	return func(msg *Msg) {
		msg.correlationData = b
	}
}

func MessageExpiry(u uint32) msgOptions {
	return func(msg *Msg) {
		msg.messageExpiry = u
	}
}

func PayloadFormat(format packets.PayloadFormat) msgOptions {
	return func(msg *Msg) {
		msg.payloadFormat = format
	}
}

func ResponseTopic(topic string) msgOptions {
	return func(msg *Msg) {
		msg.responseTopic = topic
	}
}

func UserProperties(userProperties []packets.UserProperty) msgOptions {
	return func(msg *Msg) {
		msg.userProperties = userProperties
	}
}

// NewMessage creates a message for publish service.
func NewMessage(topic string, payload []byte, qos uint8, opts ...msgOptions) packets.Message {
	m := &Msg{
		topic:   topic,
		qos:     qos,
		payload: payload,
	}
	for _, v := range opts {
		v(m)
	}
	return m
}
