package redis

import (
	"fmt"
	"sync"
	"testing"
	"time"

	redigo "github.com/gomodule/redigo/redis"
	"github.com/stretchr/testify/assert"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/persistence/queue"
	"github.com/DrmagicE/gmqtt/pkg/packets"
)

func newPool(addr string) *redigo.Pool {
	return &redigo.Pool{
		MaxIdle:     3,
		IdleTimeout: 240 * time.Second,
		// Dial or DialContext must be set. When both are set, DialContext takes precedence over Dial.
		Dial: func() (redigo.Conn, error) { return redigo.Dial("tcp", addr) },
	}
}
func TestA(t *testing.T) {
	a := assert.New(t)
	p := newPool(":6379")
	r := &Queue{
		cond:            sync.NewCond(&sync.Mutex{}),
		client:          nil,
		clientID:        "cid",
		max:             0,
		len:             0,
		mem:             nil,
		pool:            p,
		conn:            nil,
		closed:          false,
		inflightDrained: false,
		current:         0,
		readCache:       make(map[packets.PacketID][]byte),
	}
	err := r.Init(true)
	a.Nil(err)
	now := time.Now()
	a.Nil(r.Add(&queue.Elem{
		At:     now,
		Expiry: time.Time{},
		MessageWithID: &queue.Publish{
			Message: &gmqtt.Message{
				Dup:                    false,
				QoS:                    1,
				Retained:               false,
				Topic:                  "/topic",
				Payload:                []byte("qos1"),
				PacketID:               0,
				ContentType:            "",
				CorrelationData:        nil,
				MessageExpiry:          0,
				PayloadFormat:          0,
				ResponseTopic:          "",
				SubscriptionIdentifier: nil,
				UserProperties:         nil,
			},
		},
	}))
	a.Nil(r.Add(&queue.Elem{
		At:     now,
		Expiry: time.Time{},
		MessageWithID: &queue.Publish{
			Message: &gmqtt.Message{
				Dup:                    false,
				QoS:                    0,
				Retained:               false,
				Topic:                  "/topic",
				Payload:                []byte("qos0"),
				PacketID:               0,
				ContentType:            "",
				CorrelationData:        nil,
				MessageExpiry:          0,
				PayloadFormat:          0,
				ResponseTopic:          "",
				SubscriptionIdentifier: nil,
				UserProperties:         nil,
			},
		},
	}))
	a.Nil(r.Add(&queue.Elem{
		At:     now,
		Expiry: time.Time{},
		MessageWithID: &queue.Publish{
			Message: &gmqtt.Message{
				Dup:                    false,
				QoS:                    2,
				Retained:               false,
				Topic:                  "/topic",
				Payload:                []byte("qos2"),
				PacketID:               0,
				ContentType:            "",
				CorrelationData:        nil,
				MessageExpiry:          0,
				PayloadFormat:          0,
				ResponseTopic:          "",
				SubscriptionIdentifier: nil,
				UserProperties:         nil,
			},
		},
	}))
	////fmt.Println(c.Do("RPUSH", "cid", "a"))
	////fmt.Println(c.Do("RPUSH", "cid", "b"))
	////fmt.Println(c.Do("RPUSH", "cid", "c"))
	fmt.Println(r.ReadInflight(10))
	rs, err := r.Read([]packets.PacketID{1, 2, 3})
	//a.Nil(err)
	//
	fmt.Println(rs)
	fmt.Println(len(r.readCache))
}

func Test_Read(t *testing.T) {
	p := newPool(":6379")
	c := p.Get()
	rs, err := redigo.Values(c.Do("lrange", "cid", 0, -1))
	if err != nil {
		panic(err)
	}
	for _, v := range rs {
		b := v.([]byte)
		e := &queue.Elem{}
		err := e.Decode(b)
		if err != nil {
			panic(err)
		}
		fmt.Println(string(e.MessageWithID.(*queue.Publish).Payload))
		fmt.Println(e.MessageWithID.(*queue.Publish).PacketID)

	}
}
