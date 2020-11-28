package mem

import (
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/DrmagicE/gmqtt/server"
)

type Store struct {
	clientID     string
	unackpublish map[packets.PacketID]struct{}
}

func New(config server.Config, clientID string) *Store {
	return &Store{
		clientID:     clientID,
		unackpublish: make(map[packets.PacketID]struct{}),
	}
}

func (s *Store) Init(cleanStart bool) error {
	if cleanStart {
		s.unackpublish = make(map[packets.PacketID]struct{})
	}
	return nil
}

func (s *Store) Set(id packets.PacketID) (bool, error) {
	if _, ok := s.unackpublish[id]; ok {
		return true, nil
	}
	s.unackpublish[id] = struct{}{}
	return false, nil
}

func (s *Store) Remove(id packets.PacketID) error {
	delete(s.unackpublish, id)
	return nil
}
