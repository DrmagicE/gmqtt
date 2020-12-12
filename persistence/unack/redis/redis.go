package redis

import (
	"github.com/gomodule/redigo/redis"

	"github.com/DrmagicE/gmqtt/persistence/unack"
	"github.com/DrmagicE/gmqtt/pkg/packets"
)

const (
	unackPrefix = "unack:"
)

var _ unack.Store = (*Store)(nil)

type Store struct {
	clientID     string
	pool         *redis.Pool
	unackpublish map[packets.PacketID]struct{}
}

type Options struct {
	ClientID string
	Pool     *redis.Pool
}

func New(opts Options) *Store {
	return &Store{
		clientID:     opts.ClientID,
		pool:         opts.Pool,
		unackpublish: make(map[packets.PacketID]struct{}),
	}
}

func getKey(clientID string) string {
	return unackPrefix + clientID
}
func (s *Store) Init(cleanStart bool) error {
	if cleanStart {
		c := s.pool.Get()
		defer c.Close()
		s.unackpublish = make(map[packets.PacketID]struct{})
		_, err := c.Do("del", getKey(s.clientID))
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Set(id packets.PacketID) (bool, error) {
	// from cache
	if _, ok := s.unackpublish[id]; ok {
		return true, nil
	}
	c := s.pool.Get()
	defer c.Close()
	_, err := c.Do("hset", getKey(s.clientID), id, 1)
	if err != nil {
		return false, err
	}
	s.unackpublish[id] = struct{}{}
	return false, nil
}

func (s *Store) Remove(id packets.PacketID) error {
	c := s.pool.Get()
	defer c.Close()
	_, err := c.Do("hdel", getKey(s.clientID), id)
	if err != nil {
		return err
	}
	delete(s.unackpublish, id)
	return nil
}
