package redis

import (
	"sync"
	"testing"
	"time"

	redigo "github.com/gomodule/redigo/redis"
	"github.com/stretchr/testify/assert"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/persistence/subscription/mem"
)

func TestEncodeDecodeSubscription(t *testing.T) {
	a := assert.New(t)
	tt := []*gmqtt.Subscription{
		{
			ShareName:         "shareName",
			TopicFilter:       "filter",
			ID:                1,
			QoS:               1,
			NoLocal:           false,
			RetainAsPublished: false,
			RetainHandling:    0,
		}, {
			ShareName:         "",
			TopicFilter:       "abc",
			ID:                0,
			QoS:               2,
			NoLocal:           false,
			RetainAsPublished: true,
			RetainHandling:    1,
		},
	}

	for _, v := range tt {
		b := EncodeSubscription(v)
		sub, err := DecodeSubscription(b)
		a.Nil(err)
		a.Equal(v, sub)
	}
}

func newPool(addr string) *redigo.Pool {
	return &redigo.Pool{
		MaxIdle:     3,
		IdleTimeout: 240 * time.Second,
		// Dial or DialContext must be set. When both are set, DialContext takes precedence over Dial.
		Dial: func() (redigo.Conn, error) { return redigo.Dial("tcp", addr) },
	}
}

func Test_aa(t *testing.T) {
	p := newPool(":6379")
	s := &sub{
		mu:       &sync.Mutex{},
		memStore: mem.NewStore(),
		pool:     p,
	}

}
