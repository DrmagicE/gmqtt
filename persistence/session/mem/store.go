package mem

import (
	"sync"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/persistence/session"
)

var _ session.Store = (*Store)(nil)

func New() *Store {
	return &Store{
		mu:   sync.Mutex{},
		sess: make(map[string]*gmqtt.Session),
	}
}

type Store struct {
	mu   sync.Mutex
	sess map[string]*gmqtt.Session
}

func (s *Store) Set(session *gmqtt.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sess[session.ClientID] = session
	return nil
}

func (s *Store) Remove(clientID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sess, clientID)
	return nil
}

func (s *Store) Get(clientID string) (*gmqtt.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sess[clientID], nil
}

func (s *Store) GetAll() ([]*gmqtt.Session, error) {
	return nil, nil
}

func (s *Store) SetSessionExpiry(clientID string, expiry uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s, ok := s.sess[clientID]; ok {
		s.ExpiryInterval = expiry

	}
	return nil
}

func (s *Store) Iterate(fn session.IterateFn) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, v := range s.sess {
		cont := fn(v)
		if !cont {
			break
		}
	}
	return nil
}
