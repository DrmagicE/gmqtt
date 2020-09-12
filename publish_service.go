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
	// The message will send to the client only if the client is subscribed to a topic that matches the message.
	// Calling this method will not trigger OnMsgArrived hook.
	PublishToClient(clientID string, message packets.Message)
}
type publishService struct {
	server *server
}

func (p *publishService) Publish(message packets.Message) {
	p.server.mu.Lock()
	p.server.deliverMessageHandler("", "", message)
	p.server.mu.Unlock()
}
func (p *publishService) PublishToClient(clientID string, message packets.Message) {
	p.server.mu.Lock()
	p.server.deliverMessageHandler("", clientID, message)
	p.server.mu.Unlock()
}
