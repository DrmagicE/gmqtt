package gmqtt

import (
	"github.com/DrmagicE/gmqtt/pkg/packets"
)

// PublishService provides the ability to publish messages to the broker.
type PublishService interface {
	// Publish publish a message to broker.
	// Calling this method will not trigger OnMsgArrived hook.
	Publish(message packets.Message)
	// PublishToClient publish a message to a specific client.
	// If match sets to true, the message will send to the client
	// only if the client is subscribed to a topic that matches the message.
	// If match sets to false, the message will send to the client directly even
	// there are no matched subscriptions.
	// Calling this method will not trigger OnMsgArrived hook.
	PublishToClient(clientID string, message packets.Message, match bool)
}
type publishService struct {
	server *server
}

func (p *publishService) Publish(message packets.Message) {
	p.server.msgRouter <- &msgRouter{msg: message, match: true}
}
func (p *publishService) PublishToClient(clientID string, message packets.Message, match bool) {
	p.server.msgRouter <- &msgRouter{msg: message, clientID: clientID, match: match}
}

type msgOptions func(msg *msg)

// Retained sets retained flag to the message
func Retained(retained bool) msgOptions {
	return func(msg *msg) {
		msg.retained = retained
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
