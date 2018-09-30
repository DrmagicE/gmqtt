package server

import (
	"github.com/DrmagicE/gmqtt/pkg/packets"
)

type Store interface {
	Open() error
	Close() error
	PutOfflineMsg(clientId string, packet []packets.Packet) error
	GetOfflineMsg(clientId string) ([]packets.Packet,error)
	PutSessions(sp []*SessionPersistence) error
	GetSessions() ([]*SessionPersistence,error)
}