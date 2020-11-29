package redis

import (
	"bytes"
	"strings"
	"sync"

	redigo "github.com/gomodule/redigo/redis"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/persistence/encoding"
	"github.com/DrmagicE/gmqtt/persistence/subscription"
	"github.com/DrmagicE/gmqtt/persistence/subscription/mem"
)

const (
	subPrefix = "sub:"
)

var _ subscription.Store = (*sub)(nil)

func EncodeSubscription(sub *gmqtt.Subscription) []byte {
	w := &bytes.Buffer{}
	encoding.WriteString(w, []byte(sub.ShareName))
	encoding.WriteString(w, []byte(sub.TopicFilter))
	encoding.WriteUint32(w, sub.ID)
	w.WriteByte(sub.QoS)
	encoding.WriteBool(w, sub.NoLocal)
	encoding.WriteBool(w, sub.RetainAsPublished)
	w.WriteByte(sub.RetainHandling)
	return w.Bytes()
}

func DecodeSubscription(b []byte) (*gmqtt.Subscription, error) {
	sub := &gmqtt.Subscription{}
	r := bytes.NewBuffer(b)
	share, err := encoding.ReadString(r)
	if err != nil {
		return &gmqtt.Subscription{}, err
	}
	sub.ShareName = string(share)
	topic, err := encoding.ReadString(r)
	if err != nil {
		return &gmqtt.Subscription{}, err
	}
	sub.TopicFilter = string(topic)
	sub.ID, err = encoding.ReadUint32(r)
	if err != nil {
		return &gmqtt.Subscription{}, err
	}
	sub.QoS, err = r.ReadByte()
	if err != nil {
		return &gmqtt.Subscription{}, err
	}
	sub.NoLocal, err = encoding.ReadBool(r)
	if err != nil {
		return &gmqtt.Subscription{}, err
	}
	sub.RetainAsPublished, err = encoding.ReadBool(r)
	if err != nil {
		return &gmqtt.Subscription{}, err
	}
	sub.RetainHandling, err = r.ReadByte()
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func New(pool *redigo.Pool) *sub {
	return &sub{
		mu:       &sync.Mutex{},
		memStore: mem.NewStore(),
		pool:     pool,
	}
}

type sub struct {
	mu       *sync.Mutex
	memStore *mem.TrieDB
	pool     *redigo.Pool
}

// Init loads the subscriptions of given clientIDs from backend into memory.
func (s *sub) Init(clientIDs []string) error {
	if len(clientIDs) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.pool.Get()
	defer c.Close()
	for _, v := range clientIDs {
		rs, err := redigo.Values(c.Do("hgetall", subPrefix+v))
		if err != nil {
			return err
		}
		for i := 1; i < len(rs); i = i + 2 {
			sub, err := DecodeSubscription(rs[i].([]byte))
			if err != nil {
				return err
			}
			s.memStore.SubscribeLocked(strings.TrimLeft(v, subPrefix), sub)
		}
	}
	return nil
}

func (s *sub) Close() error {
	_ = s.memStore.Close()
	return s.pool.Close()
}

func (s *sub) Subscribe(clientID string, subscriptions ...*gmqtt.Subscription) (rs subscription.SubscribeResult, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.pool.Get()
	defer c.Close()
	// hset sub:clientID topicFilter xxx
	for _, v := range subscriptions {
		err = c.Send("hset", subPrefix+clientID, subscription.GetFullTopicName(v.ShareName, v.TopicFilter), EncodeSubscription(v))
		if err != nil {
			return nil, err
		}
	}
	err = c.Flush()
	if err != nil {
		return nil, err
	}
	rs = s.memStore.SubscribeLocked(clientID, subscriptions...)
	return rs, nil
}

func (s *sub) Unsubscribe(clientID string, topics ...string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.pool.Get()
	defer c.Close()
	_, err := c.Do("hdel", subPrefix+clientID, topics)
	if err != nil {
		return err
	}
	s.memStore.UnsubscribeLocked(clientID, topics...)
	return nil
}

func (s *sub) UnsubscribeAll(clientID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.pool.Get()
	defer c.Close()
	_, err := c.Do("del", subPrefix+clientID)
	if err != nil {
		return err
	}
	s.memStore.UnsubscribeAllLocked(clientID)
	return nil
}

func (s *sub) Iterate(fn subscription.IterateFn, options subscription.IterationOptions) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.memStore.IterateLocked(fn, options)
}

func (s *sub) GetStats() subscription.Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.memStore.GetStatusLocked()
}

func (s *sub) GetClientStats(clientID string) (subscription.Stats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.memStore.GetClientStatsLocked(clientID)
}
