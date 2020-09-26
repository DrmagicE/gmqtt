package server

import "github.com/DrmagicE/gmqtt"

type publishService struct {
	server *server
}

func (p *publishService) Publish(message *gmqtt.Message) {
	p.server.mu.Lock()
	p.server.deliverMessageHandler("", "", message)
	p.server.mu.Unlock()
}
func (p *publishService) PublishToClient(clientID string, message *gmqtt.Message) {
	p.server.mu.Lock()
	p.server.deliverMessageHandler("", clientID, message)
	p.server.mu.Unlock()
}
