package test

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/config"
	"github.com/DrmagicE/gmqtt/persistence/queue"

	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/DrmagicE/gmqtt/server"
)

var (
	TestServerConfig = config.Config{
		MQTT: config.MQTT{
			MaxQueuedMsg: 5,
		},
	}
	TestHooks = server.Hooks{OnMsgDropped: func(ctx context.Context, clientID string, msg *gmqtt.Message, err error) {
		dropMsg[cid] = append(dropMsg[cid], msg)
		dropErr[cid] = err
	}}
	dropMsg      = make(map[string][]*gmqtt.Message)
	dropErr      = make(map[string]error)
	cid          = "cid"
	TestClientID = cid
)

func initDrop() {
	dropMsg = make(map[string][]*gmqtt.Message)
	dropErr = make(map[string]error)
}

func assertElemEqual(a *assert.Assertions, expected, actual *queue.Elem) {
	expected.At = time.Unix(expected.At.Unix(), 0)
	expected.Expiry = time.Unix(expected.Expiry.Unix(), 0)
	actual.At = time.Unix(actual.At.Unix(), 0)
	actual.Expiry = time.Unix(actual.Expiry.Unix(), 0)
	a.Equal(expected, actual)
}

// 2 inflight message + 3 new message
var initElems = []*queue.Elem{
	{
		At:     time.Now(),
		Expiry: time.Time{},
		MessageWithID: &queue.Publish{
			Message: &gmqtt.Message{
				QoS:      packets.Qos1,
				Retained: false,
				Topic:    "/topic1_qos1",
				Payload:  []byte("qos1"),
				PacketID: 1,
			},
		},
	}, {
		At:     time.Now(),
		Expiry: time.Time{},
		MessageWithID: &queue.Publish{
			Message: &gmqtt.Message{
				QoS:      packets.Qos2,
				Retained: false,
				Topic:    "/topic1_qos2",
				Payload:  []byte("qos2"),
				PacketID: 2,
			},
		},
	}, {
		At:     time.Now(),
		Expiry: time.Time{},
		MessageWithID: &queue.Publish{
			Message: &gmqtt.Message{
				QoS:      packets.Qos1,
				Retained: false,
				Topic:    "/topic1_qos1",
				Payload:  []byte("qos1"),
				PacketID: 0,
			},
		},
	},
	{
		At:     time.Now(),
		Expiry: time.Time{},
		MessageWithID: &queue.Publish{
			Message: &gmqtt.Message{
				QoS:      packets.Qos0,
				Retained: false,
				Topic:    "/topic1_qos0",
				Payload:  []byte("qos0"),
				PacketID: 0,
			},
		},
	},
	{
		At:     time.Now(),
		Expiry: time.Time{},
		MessageWithID: &queue.Publish{
			Message: &gmqtt.Message{
				QoS:      packets.Qos2,
				Retained: false,
				Topic:    "/topic1_qos2",
				Payload:  []byte("qos2"),
				PacketID: 0,
			},
		},
	},
}

func initStore(store queue.Store) error {
	return store.Init(&queue.InitOptions{
		CleanStart:     true,
		Version:        packets.Version5,
		ReadBytesLimit: 100,
	})
}

func add(store queue.Store) error {
	for _, v := range initElems {
		elem := *v
		elem.MessageWithID = &queue.Publish{
			Message: elem.MessageWithID.(*queue.Publish).Message.Copy(),
		}
		err := store.Add(&elem)
		if err != nil {
			return err
		}
	}
	return nil
}
func assertDrop(a *assert.Assertions, elem *queue.Elem, err error) {
	a.Len(dropMsg, 1)
	a.Equal(elem.MessageWithID.(*queue.Publish).Message, dropMsg[cid][0])
	a.Equal(err, dropErr[cid])
	initDrop()
}

func reconnect(a *assert.Assertions, cleanStart bool, store queue.Store) {
	a.Nil(store.Close())
	a.Nil(store.Init(&queue.InitOptions{
		CleanStart:     cleanStart,
		Version:        packets.Version5,
		ReadBytesLimit: 100,
	}))
}

type New func(config config.Config, hooks server.Hooks) (server.Persistence, error)

func TestQueue(t *testing.T, store queue.Store) {
	initDrop()
	a := assert.New(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	a.Nil(initStore(store))
	a.Nil(add(store))
	testRead(a, store)
	testDrop(a, store)
	testReplace(a, store)
	testCleanStart(a, store)
	testReadExceedsDrop(a, store)
	testClose(a, store)
}

func testDrop(a *assert.Assertions, store queue.Store) {
	for i := 0; i < 3; i++ {
		err := store.Add(&queue.Elem{
			At:     time.Now(),
			Expiry: time.Time{},
			MessageWithID: &queue.Publish{
				Message: &gmqtt.Message{
					Dup:      false,
					QoS:      2,
					Retained: false,
					Topic:    "123",
					Payload:  []byte("123"),
					PacketID: 0,
				},
			},
		})
		a.Nil(err)
	}
	e, err := store.Read([]packets.PacketID{5, 6, 7})
	a.Nil(err)
	a.Len(e, 3)
	a.EqualValues(5, e[0].MessageWithID.ID())
	a.EqualValues(6, e[1].MessageWithID.ID())
	a.EqualValues(7, e[2].MessageWithID.ID())

	// queue: 1,2,5,6,7

	// drop case 1. the current elem if there is no more non-inflight messages.
	dropElem := &queue.Elem{
		At:     time.Now(),
		Expiry: time.Time{},
		MessageWithID: &queue.Publish{
			Message: &gmqtt.Message{
				Dup:      false,
				QoS:      1,
				Retained: false,
				Topic:    "123",
				Payload:  []byte("123"),
				PacketID: 0,
			},
		},
	}
	err = store.Add(dropElem)
	a.Nil(err)
	assertDrop(a, dropElem, queue.ErrDropQueueFull)
	// queue: 1,2,5,6,7
	a.Nil(store.Remove(1))
	a.Nil(store.Remove(2))
	// queue: 5,6,7

	dropQoS0 := &queue.Elem{
		At:     time.Now(),
		Expiry: time.Time{},
		MessageWithID: &queue.Publish{
			Message: &gmqtt.Message{
				Dup:      false,
				QoS:      0,
				Retained: false,
				Topic:    "/t_qos0",
				Payload:  []byte("test"),
			},
		},
	}
	a.Nil(store.Add(dropQoS0))

	// add expired elem
	dropExpired := &queue.Elem{
		At:     time.Now(),
		Expiry: time.Now().Add(-10 * time.Second),
		MessageWithID: &queue.Publish{
			Message: &gmqtt.Message{
				Dup:      false,
				QoS:      0,
				Retained: false,
				Topic:    "/drop",
				Payload:  []byte("test"),
			},
		},
	}

	a.Nil(store.Add(dropExpired))

	// drop case 2. expired message
	dropFront := &queue.Elem{
		At:     time.Now(),
		Expiry: time.Time{},
		MessageWithID: &queue.Publish{
			Message: &gmqtt.Message{
				Dup:      false,
				QoS:      0,
				Retained: false,
				Topic:    "/drop_front",
				Payload:  []byte("test"),
			},
		},
	}
	a.Nil(store.Add(dropFront))

	assertDrop(a, dropExpired, queue.ErrDropExpired)
	// queue: 5,6,7,0,0
	// drop case 3. drop qos0 message
	a.Nil(store.Add(&queue.Elem{
		At:     time.Now(),
		Expiry: time.Time{},
		MessageWithID: &queue.Publish{
			Message: &gmqtt.Message{
				Dup:      false,
				QoS:      packets.Qos1,
				Retained: false,
				Topic:    "/t_qos1",
				Payload:  []byte("test"),
			},
		},
	}))
	assertDrop(a, dropQoS0, queue.ErrDropQueueFull)

	// queue: 5(qos2),6(qos2),7(qos2),0(qos0),0(qos1)
	// drop case 4. drop the front message
	a.Nil(store.Add(&queue.Elem{
		At:     time.Now(),
		Expiry: time.Time{},
		MessageWithID: &queue.Publish{
			Message: &gmqtt.Message{
				Dup:      false,
				QoS:      1,
				Retained: false,
				Topic:    "/t",
				Payload:  []byte("test"),
			},
		},
	}))

	assertDrop(a, dropFront, queue.ErrDropQueueFull)

}

func testRead(a *assert.Assertions, store queue.Store) {
	// 2 inflight
	e, err := store.ReadInflight(1)
	a.Nil(err)
	a.Len(e, 1)
	assertElemEqual(a, initElems[0], e[0])

	e, err = store.ReadInflight(2)
	a.Len(e, 1)
	assertElemEqual(a, initElems[1], e[0])
	pids := []packets.PacketID{3, 4, 5}
	e, err = store.Read(pids)
	a.Len(e, 3)

	// must consume packet id in order and do not skip packet id if there are qos0 messages.
	a.EqualValues(3, e[0].MessageWithID.ID())
	a.EqualValues(0, e[1].MessageWithID.ID())
	a.EqualValues(4, e[2].MessageWithID.ID())

	err = store.Remove(3)
	a.Nil(err)
	err = store.Remove(4)
	a.Nil(err)

}

func testReplace(a *assert.Assertions, store queue.Store) {
	// queue: 5(qos2),6(qos2),7(qos2),0(qos0),0(qos1)
	for i := 5; i <= 8; i++ {
		replaced, err := store.Replace(&queue.Elem{
			At:     time.Now(),
			Expiry: time.Time{},
			MessageWithID: &queue.Pubrel{
				PacketID: packets.PacketID(i),
			},
		})
		a.Nil(err)
		if i <= 7 {
			a.True(replaced)
		} else {
			a.False(replaced, "must not replace unread packet")
		}
	}
	reconnect(a, false, store)

	inflights, err := store.ReadInflight(5)
	a.Nil(err)
	a.Len(inflights, 3)
	a.Equal(&queue.Pubrel{
		PacketID: 5,
	}, inflights[0].MessageWithID)
	a.Equal(&queue.Pubrel{
		PacketID: 6,
	}, inflights[1].MessageWithID)
	a.Equal(&queue.Pubrel{
		PacketID: 7,
	}, inflights[2].MessageWithID)

}

func testReadExceedsDrop(a *assert.Assertions, store queue.Store) {
	// add exceeded message
	exceeded := &queue.Elem{
		At: time.Now(),
		MessageWithID: &queue.Publish{
			Message: &gmqtt.Message{
				Dup:      false,
				QoS:      1,
				Retained: false,
				Topic:    "/drop_exceed",

				Payload: make([]byte, 100),
			},
		},
	}
	a.Nil(store.Add(exceeded))
	e, err := store.Read([]packets.PacketID{1})
	a.Nil(err)
	a.Len(e, 0)
	assertDrop(a, exceeded, queue.ErrDropExceedsMaxPacketSize)

	expired := &queue.Elem{
		At:     time.Now(),
		Expiry: time.Now(),
		MessageWithID: &queue.Publish{
			Message: &gmqtt.Message{
				Dup:      false,
				QoS:      1,
				Retained: false,
				Topic:    "/drop_exceed",

				Payload: make([]byte, 100),
			},
		},
	}
	a.Nil(store.Add(expired))
	e, err = store.Read([]packets.PacketID{1})
	a.Nil(err)
	a.Len(e, 0)
	assertDrop(a, exceeded, queue.ErrDropExpired)
}

func testCleanStart(a *assert.Assertions, store queue.Store) {
	reconnect(a, true, store)
	rs, err := store.ReadInflight(10)
	a.Nil(err)
	a.Len(rs, 0)
}

func testClose(a *assert.Assertions, store queue.Store) {

	t := time.After(2 * time.Second)

	result := make(chan struct {
		len int
		err error
	})
	go func() {
		// should block
		rs, err := store.Read([]packets.PacketID{1, 2, 3})
		result <- struct {
			len int
			err error
		}{len: len(rs), err: err}

	}()
	select {
	case <-t:
		a.Fail("Read must be blocked before Close")
	default:
	}

	a.Nil(store.Close())
	timeout := time.After(5 * time.Second)
	select {
	case <-timeout:
		a.Fail("Read must be unblocked after Close")
	case r := <-result:
		a.Zero(r.len)
		a.Equal(queue.ErrClosed, r.err)
	}
}
