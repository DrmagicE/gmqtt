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
	return &msg{
		dup:      p.Dup,
		qos:      p.Qos,
		retained: p.Retain,
		topic:    string(p.TopicName),
		packetID: p.PacketID,
		payload:  p.Payload,
	}
}

func messageToPublish(msg packets.Message) *packets.Publish {
	return &packets.Publish{
		Dup:       msg.Dup(),
		Qos:       msg.Qos(),
		Retain:    msg.Retained(),
		TopicName: []byte(msg.Topic()),
		PacketID:  msg.PacketID(),
		Payload:   msg.Payload(),
	}
}
