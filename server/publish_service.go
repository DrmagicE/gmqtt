package server

import "github.com/DrmagicE/gmqtt"

type publishService struct {
	server *server
}

func (p *publishService) Publish(message *gmqtt.Message) {
	p.server.mu.Lock()
	p.server.deliverMessage("", message, defaultIterateOptions(message.Topic))
	p.server.mu.Unlock()
}
