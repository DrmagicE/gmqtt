package redis

import (
	"bytes"
	"sync"
	"time"

	"github.com/gomodule/redigo/redis"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/persistence/encoding"
	"github.com/DrmagicE/gmqtt/persistence/session"
)

const (
	sessPrefix = "session:"
)

var _ session.Store = (*Store)(nil)

type Store struct {
	mu   sync.Mutex
	pool *redis.Pool
}

func New(pool *redis.Pool) *Store {
	return &Store{
		mu:   sync.Mutex{},
		pool: pool,
	}
}

func getKey(clientID string) string {
	return sessPrefix + clientID
}
func (s *Store) Set(session *gmqtt.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.pool.Get()
	defer c.Close()
	b := &bytes.Buffer{}
	encoding.EncodeMessage(session.Will, b)
	_, err := c.Do("hset", getKey(session.ClientID),
		"client_id", session.ClientID,
		"will", b.Bytes(),
		"will_delay_interval", session.WillDelayInterval,
		"connected_at", session.ConnectedAt.Unix(),
		"expiry_interval", session.ExpiryInterval,
	)
	return err
}

func (s *Store) Remove(clientID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.pool.Get()
	defer c.Close()
	_, err := c.Do("del", getKey(clientID))
	return err
}

func (s *Store) Get(clientID string) (*gmqtt.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.pool.Get()
	defer c.Close()
	return getSessionLocked(getKey(clientID), c)
}

func getSessionLocked(key string, c redis.Conn) (*gmqtt.Session, error) {
	replay, err := redis.Values(c.Do("hmget", key, "client_id", "will", "will_delay_interval", "connected_at", "expiry_interval"))
	if err != nil {
		return nil, err
	}
	sess := &gmqtt.Session{}
	var connectedAt uint32
	var will []byte
	_, err = redis.Scan(replay, &sess.ClientID, &will, &sess.WillDelayInterval, &connectedAt, &sess.ExpiryInterval)
	if err != nil {
		return nil, err
	}
	sess.ConnectedAt = time.Unix(int64(connectedAt), 0)
	sess.Will, err = encoding.DecodeMessageFromBytes(will)
	if err != nil {
		return nil, err
	}
	return sess, nil
}

func (s *Store) SetSessionExpiry(clientID string, expiry uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.pool.Get()
	defer c.Close()
	_, err := c.Do("hset", getKey(clientID),
		"expiry_interval", expiry,
	)
	return err
}

func (s *Store) Iterate(fn session.IterateFn) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.pool.Get()
	defer c.Close()
	iter := 0
	for {
		arr, err := redis.Values(c.Do("SCAN", iter, "MATCH", sessPrefix+"*"))
		if err != nil {
			return err
		}
		if len(arr) >= 1 {
			for _, v := range arr[1:] {
				for _, vv := range v.([]interface{}) {
					sess, err := getSessionLocked(string(vv.([]uint8)), c)
					if err != nil {
						return err
					}
					cont := fn(sess)
					if !cont {
						return nil
					}
				}
			}
		}
		iter, _ = redis.Int(arr[0], nil)
		if iter == 0 {
			break
		}
	}
	return nil
}
