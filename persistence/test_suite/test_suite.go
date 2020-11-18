package test_suite

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/persistence/queue"

	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/DrmagicE/gmqtt/server"
)

var (
	TestServerConfig = server.Config{MaxQueuedMsg: 5}
	TestClient       server.Client
	TestHooks        server.Hooks
	dropMsg          = make(map[string][]*gmqtt.Message)
	cid              = "cid"
)

func initDrop() {
	dropMsg = make(map[string][]*gmqtt.Message)
}

func init() {
	TestServerConfig = server.Config{
		MaxQueuedMsg: 5,
	}
	TestHooks = server.Hooks{
		OnMsgDropped: func(ctx context.Context, client server.Client, msg *gmqtt.Message) {

		},
	}
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
	return store.Init(true)
}

func add(store queue.Store) error {
	for _, v := range initElems {
		err := store.Add(v)
		if err != nil {
			return err
		}
	}
	return nil
}
func assertDrop(a *assert.Assertions, elem *queue.Elem) {
	a.Len(dropMsg, 1)
	a.Equal(elem.MessageWithID.(*queue.Publish).Message, dropMsg[cid][0])
	initDrop()
}

func reconnect(a *assert.Assertions, cleanStart bool, store queue.Store) {
	a.Nil(store.Close())
	a.Nil(store.Init(cleanStart))
}

type New func(config server.Config, hooks server.Hooks) (server.Persistence, error)

func TestQueue(t *testing.T, factory server.PersistenceFactory) {
	a := assert.New(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	c := server.NewMockClient(ctrl)
	p, err := factory.New(TestServerConfig, server.Hooks{OnMsgDropped: func(ctx context.Context, client server.Client, msg *gmqtt.Message) {
		dropMsg[cid] = append(dropMsg[cid], msg)
	}})
	a.Nil(err)
	a.Nil(p.Open())
	defer p.Close()
	store, err := p.NewQueueStore(TestServerConfig, c)
	a.Nil(err)
	c.EXPECT().ClientOptions().Return(&server.ClientOptions{ClientID: cid}).AnyTimes()
	a.Nil(err)
	a.Nil(initStore(store))
	a.Nil(add(store))
	testRead(a, store)
	testDrop(a, store)
	testReplace(a, store)
	testCleanStart(a, store)
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
	assertDrop(a, dropElem)
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
			},
		},
	}
	a.Nil(store.Add(dropFront))

	assertDrop(a, dropExpired)
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
			},
		},
	}))
	assertDrop(a, dropQoS0)

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
			},
		},
	}))

	assertDrop(a, dropFront)
}

func testRead(a *assert.Assertions, store queue.Store) {
	// 2 inflight
	e, err := store.ReadInflight(1)
	a.Nil(err)
	a.Len(e, 1)
	a.Equal(initElems[0], e[0])
	e, err = store.ReadInflight(2)
	a.Len(e, 1)
	a.Equal(initElems[1], e[0])
	pids := []packets.PacketID{3, 4, 5}
	e, err = store.Read(pids)
	a.Len(e, 3)

	// must consume packet id in order and do not skip packet id if there are qos0 messages.
	a.EqualValues(3, e[0].MessageWithID.ID())
	a.EqualValues(0, e[1].MessageWithID.ID())
	a.EqualValues(4, e[2].MessageWithID.ID())

	// Read should block
	//timer := time.After(1 * time.Second)
	//c := make(chan struct{})
	//go func() {
	//	store.Read([]packets.PacketID{6})
	//	<-c
	//}()
	//
	//select {
	//case <-timer:
	//case <-c:
	//	a.Fail("Read is not block")
	//}

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
