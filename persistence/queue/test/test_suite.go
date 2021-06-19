package test

import (
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
			MaxQueuedMsg:   5,
			InflightExpiry: 2 * time.Second,
		},
	}
	cid          = "cid"
	TestClientID = cid
	TestNotifier = &testNotifier{}
)

type testNotifier struct {
	dropElem    []*queue.Elem
	dropErr     error
	inflightLen int
	msgQueueLen int
}

func (t *testNotifier) NotifyDropped(elem *queue.Elem, err error) {
	t.dropElem = append(t.dropElem, elem)
	t.dropErr = err
}

func (t *testNotifier) NotifyInflightAdded(delta int) {
	t.inflightLen += delta
	if t.inflightLen < 0 {
		t.inflightLen = 0
	}
}

func (t *testNotifier) NotifyMsgQueueAdded(delta int) {
	t.msgQueueLen += delta
	if t.msgQueueLen < 0 {
		t.msgQueueLen = 0
	}
}

func initDrop() {
	TestNotifier.dropElem = nil
	TestNotifier.dropErr = nil
}

func initNotifierLen() {
	TestNotifier.inflightLen = 0
	TestNotifier.msgQueueLen = 0
}

func assertMsgEqual(a *assert.Assertions, expected, actual *queue.Elem) {
	expMsg := expected.MessageWithID.(*queue.Publish).Message
	actMsg := actual.MessageWithID.(*queue.Publish).Message
	a.Equal(expMsg.Topic, actMsg.Topic)
	a.Equal(expMsg.QoS, actMsg.QoS)
	a.Equal(expMsg.Payload, actMsg.Payload)
	a.Equal(expMsg.PacketID, actMsg.PacketID)
}

func assertQueueLen(a *assert.Assertions, inflightLen, msgQueueLen int) {
	a.Equal(inflightLen, TestNotifier.inflightLen)
	a.Equal(msgQueueLen, TestNotifier.msgQueueLen)
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
		Notifier:       TestNotifier,
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
	TestNotifier.inflightLen = 2
	return nil
}

func assertDrop(a *assert.Assertions, elem *queue.Elem, err error) {
	a.Len(TestNotifier.dropElem, 1)
	switch elem.MessageWithID.(type) {
	case *queue.Publish:
		actual := TestNotifier.dropElem[0].MessageWithID.(*queue.Publish)
		pub := elem.MessageWithID.(*queue.Publish)
		a.Equal(pub.Message.Topic, actual.Topic)
		a.Equal(pub.Message.QoS, actual.QoS)
		a.Equal(pub.Payload, actual.Payload)
		a.Equal(pub.PacketID, actual.PacketID)
		a.Equal(err, TestNotifier.dropErr)
	case *queue.Pubrel:
		actual := TestNotifier.dropElem[0].MessageWithID.(*queue.Pubrel)
		pubrel := elem.MessageWithID.(*queue.Pubrel)
		a.Equal(pubrel.PacketID, actual.PacketID)
		a.Equal(err, TestNotifier.dropErr)
	default:
		a.FailNow("unexpected elem type")

	}
	initDrop()
}

func reconnect(a *assert.Assertions, cleanStart bool, store queue.Store) {
	a.NoError(store.Close())
	a.NoError(store.Init(&queue.InitOptions{
		CleanStart:     cleanStart,
		Version:        packets.Version5,
		ReadBytesLimit: 100,
		Notifier:       TestNotifier,
	}))
}

type New func(config config.Config, hooks server.Hooks) (server.Persistence, error)

func TestQueue(t *testing.T, store queue.Store) {
	initDrop()
	a := assert.New(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	a.NoError(initStore(store))
	a.NoError(add(store))
	assertQueueLen(a, 2, 5)
	testRead(a, store)
	testDrop(a, store)
	testReplace(a, store)
	testCleanStart(a, store)
	testReadExceedsDrop(a, store)
	testClose(a, store)
}

func testDrop(a *assert.Assertions, store queue.Store) {
	// wait inflight messages to expire
	time.Sleep(TestServerConfig.MQTT.InflightExpiry)
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
	// drop expired inflight message (pid=1)
	dropElem := initElems[0]
	// queue: 1,2,0(qos2),0(qos2),0(qos2) (1 and 2 are expired inflight messages)
	err := store.Add(&queue.Elem{
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
	})
	a.NoError(err)
	assertDrop(a, dropElem, queue.ErrDropExpiredInflight)
	assertQueueLen(a, 1, 5)

	e, err := store.Read([]packets.PacketID{5, 6, 7})
	a.NoError(err)
	a.Len(e, 3)
	a.EqualValues(5, e[0].MessageWithID.ID())
	a.EqualValues(6, e[1].MessageWithID.ID())
	a.EqualValues(7, e[2].MessageWithID.ID())
	// queue: 2,5(qos2),6(qos2),7(qos2), 0(qos1) (2 is expired inflight message)
	assertQueueLen(a, 4, 5)
	// drop expired inflight message (pid=2)
	dropElem = initElems[1]
	err = store.Add(&queue.Elem{
		At:     time.Now(),
		Expiry: time.Time{},
		MessageWithID: &queue.Publish{
			Message: &gmqtt.Message{
				Dup:      false,
				QoS:      1,
				Retained: false,
				Topic:    "1234",
				Payload:  []byte("1234"),
				PacketID: 0,
			},
		},
	})
	a.NoError(err)
	// queue: 5(qos2),6(qos2),7(qos2), 0(qos1), 0(qos1)
	assertDrop(a, dropElem, queue.ErrDropExpiredInflight)

	assertQueueLen(a, 3, 5)
	e, err = store.Read([]packets.PacketID{8, 9})

	a.NoError(err)

	// queue: 5(qos2),6(qos2),7(qos2),8(qos1),9(qos1)
	a.Len(e, 2)
	a.EqualValues(8, e[0].MessageWithID.ID())
	a.EqualValues(9, e[1].MessageWithID.ID())
	assertQueueLen(a, 5, 5)

	// drop the elem that is going to enqueue if there is no more non-inflight messages.
	dropElem = &queue.Elem{
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
	a.NoError(err)
	assertDrop(a, dropElem, queue.ErrDropQueueFull)
	assertQueueLen(a, 5, 5)

	// queue: 5(qos2),6(qos2),7(qos2),8(qos1),9(qos1)
	a.NoError(store.Remove(5))
	a.NoError(store.Remove(6))
	// queue: 7(qos2),8(qos2),9(qos2)
	assertQueueLen(a, 3, 3)

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
	a.NoError(store.Add(dropQoS0))
	// queue: 7(qos2),8(qos2),9(qos2),0 (qos0/t_qos0)
	assertQueueLen(a, 3, 4)

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
	a.NoError(store.Add(dropExpired))
	// queue: 7(qos2),8(qos2),9(qos2), 0(qos0/t_qos0), 0(qos0/drop)
	assertQueueLen(a, 3, 5)
	dropFront := &queue.Elem{
		At:     time.Now(),
		Expiry: time.Time{},
		MessageWithID: &queue.Publish{
			Message: &gmqtt.Message{
				Dup:      false,
				QoS:      1,
				Retained: false,
				Topic:    "/drop_front",
				Payload:  []byte("drop_front"),
			},
		},
	}
	// drop the expired non-inflight message
	a.NoError(store.Add(dropFront))
	// queue: 7(qos2),8(qos2),9(qos2), 0(qos0/t_qos0), 0(qos1/drop_front)
	assertDrop(a, dropExpired, queue.ErrDropExpired)
	assertQueueLen(a, 3, 5)

	// drop qos0 message
	a.Nil(store.Add(&queue.Elem{
		At:     time.Now(),
		Expiry: time.Time{},
		MessageWithID: &queue.Publish{
			Message: &gmqtt.Message{
				Dup:      false,
				QoS:      packets.Qos1,
				Retained: false,
				Topic:    "/t_qos1",
				Payload:  []byte("/t_qos1"),
			},
		},
	}))
	// queue: 7(qos2),8(qos2),9(qos2), 0(qos1/drop_front), 0(qos1/t_qos1)
	assertDrop(a, dropQoS0, queue.ErrDropQueueFull)
	assertQueueLen(a, 3, 5)

	expiredPub := &queue.Elem{
		At:     time.Now(),
		Expiry: time.Now().Add(TestServerConfig.MQTT.InflightExpiry),
		MessageWithID: &queue.Publish{
			Message: &gmqtt.Message{
				Dup:      false,
				QoS:      1,
				Retained: false,
				Topic:    "/t",
				Payload:  []byte("/t"),
			},
		},
	}
	a.NoError(store.Add(expiredPub))
	// drop the front message
	assertDrop(a, dropFront, queue.ErrDropQueueFull)
	// queue: 7(qos2),8(qos2),9(qos2), 0(qos1/t_qos1), 0(qos1/t)
	assertQueueLen(a, 3, 5)
	// replace with an expired pubrel
	expiredPubrel := &queue.Elem{
		At:     time.Now(),
		Expiry: time.Now().Add(-1 * time.Second),
		MessageWithID: &queue.Pubrel{
			PacketID: 7,
		},
	}
	r, err := store.Replace(expiredPubrel)
	a.True(r)
	a.NoError(err)
	assertQueueLen(a, 3, 5)
	// queue: 7(qos2-pubrel),8(qos2),9(qos2), 0(qos1/t_qos1), 0(qos1/t)
	a.NoError(store.Add(&queue.Elem{
		At:     time.Now(),
		Expiry: time.Time{},
		MessageWithID: &queue.Publish{
			Message: &gmqtt.Message{
				Dup:      false,
				QoS:      1,
				Retained: false,
				Topic:    "/t1",
				Payload:  []byte("/t1"),
			},
		},
	}))
	// queue: 8(qos2),9(qos2), 0(qos1/t_qos1), 0(qos1/t), 0(qos1/t1)
	assertDrop(a, expiredPubrel, queue.ErrDropExpiredInflight)
	assertQueueLen(a, 2, 5)
	drop := &queue.Elem{
		At: time.Now(),
		MessageWithID: &queue.Publish{
			Message: &gmqtt.Message{
				Dup:      false,
				QoS:      0,
				Retained: false,
				Topic:    "/t2",
				Payload:  []byte("/t2"),
			},
		},
	}
	a.NoError(store.Add(drop))
	assertDrop(a, drop, queue.ErrDropQueueFull)
	assertQueueLen(a, 2, 5)

	a.NoError(store.Remove(8))
	a.NoError(store.Remove(9))
	// queue:  0(qos1/t_qos1), 0(qos1/t), 0(qos1/t1)
	assertQueueLen(a, 0, 3)
	// wait qos1/t to expire.
	time.Sleep(TestServerConfig.MQTT.InflightExpiry)
	e, err = store.Read([]packets.PacketID{1, 2, 3})
	a.NoError(err)
	a.Len(e, 2)
	assertQueueLen(a, 2, 2)
	a.NoError(store.Remove(1))
	a.NoError(store.Remove(2))
	assertQueueLen(a, 0, 0)
}

func testRead(a *assert.Assertions, store queue.Store) {
	// 2 inflight
	e, err := store.ReadInflight(1)
	a.Nil(err)
	a.Len(e, 1)
	assertMsgEqual(a, initElems[0], e[0])

	e, err = store.ReadInflight(2)
	a.Len(e, 1)
	assertMsgEqual(a, initElems[1], e[0])
	pids := []packets.PacketID{3, 4, 5}
	e, err = store.Read(pids)
	a.Len(e, 3)

	// must consume packet id in order and do not skip packet id if there are qos0 messages.
	a.EqualValues(3, e[0].MessageWithID.ID())
	a.EqualValues(0, e[1].MessageWithID.ID())
	a.EqualValues(4, e[2].MessageWithID.ID())

	assertQueueLen(a, 4, 4)

	err = store.Remove(3)
	a.NoError(err)
	err = store.Remove(4)
	a.NoError(err)
	assertQueueLen(a, 2, 2)

}

func testReplace(a *assert.Assertions, store queue.Store) {

	var elems []*queue.Elem
	elems = append(elems, &queue.Elem{
		At:     time.Now(),
		Expiry: time.Time{},
		MessageWithID: &queue.Publish{
			Message: &gmqtt.Message{
				QoS:     2,
				Topic:   "/t_replace",
				Payload: []byte("t_replace"),
			},
		},
	}, &queue.Elem{
		At:     time.Now(),
		Expiry: time.Time{},
		MessageWithID: &queue.Publish{
			Message: &gmqtt.Message{
				QoS:     2,
				Topic:   "/t_replace",
				Payload: []byte("t_replace"),
			},
		},
	}, &queue.Elem{
		At:     time.Now(),
		Expiry: time.Time{},
		MessageWithID: &queue.Publish{
			Message: &gmqtt.Message{
				QoS:      2,
				Topic:    "/t_unread",
				Payload:  []byte("t_unread"),
				PacketID: 3,
			},
		},
	})
	for i := 0; i < 2; i++ {
		elems = append(elems, &queue.Elem{
			At:     time.Now(),
			Expiry: time.Time{},
			MessageWithID: &queue.Publish{
				Message: &gmqtt.Message{
					QoS:     2,
					Topic:   "/t_replace",
					Payload: []byte("t_replace"),
				},
			},
		})
		a.NoError(store.Add(elems[i]))
	}
	assertQueueLen(a, 0, 2)

	e, err := store.Read([]packets.PacketID{1, 2})
	// queue: 1(qos2),2(qos2)
	a.NoError(err)
	a.Len(e, 2)
	assertQueueLen(a, 2, 2)
	r, err := store.Replace(&queue.Elem{
		At:     time.Now(),
		Expiry: time.Time{},
		MessageWithID: &queue.Pubrel{
			PacketID: 1,
		},
	})
	a.True(r)
	a.NoError(err)

	r, err = store.Replace(&queue.Elem{
		At:     time.Now(),
		Expiry: time.Time{},
		MessageWithID: &queue.Pubrel{
			PacketID: 3,
		},
	})
	a.False(r)
	a.NoError(err)
	a.NoError(store.Add(elems[2]))
	TestNotifier.inflightLen++
	// queue: 1(qos2-pubrel),2(qos2), 3(qos2)

	r, err = store.Replace(&queue.Elem{
		At:     time.Now(),
		Expiry: time.Time{},
		MessageWithID: &queue.Pubrel{
			PacketID: packets.PacketID(3),
		}})
	a.False(r, "must not replace unread packet")
	a.NoError(err)
	assertQueueLen(a, 3, 3)

	reconnect(a, false, store)

	inflight, err := store.ReadInflight(5)
	a.NoError(err)
	a.Len(inflight, 3)
	a.Equal(&queue.Pubrel{
		PacketID: 1,
	}, inflight[0].MessageWithID)

	elems[1].MessageWithID.SetID(2)
	elems[2].MessageWithID.SetID(3)
	assertMsgEqual(a, elems[1], inflight[1])
	assertMsgEqual(a, elems[2], inflight[2])
	assertQueueLen(a, 3, 3)

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
				Payload:  make([]byte, 100),
			},
		},
	}
	a.NoError(store.Add(exceeded))
	assertQueueLen(a, 0, 1)
	e, err := store.Read([]packets.PacketID{1})
	a.NoError(err)
	a.Len(e, 0)
	assertDrop(a, exceeded, queue.ErrDropExceedsMaxPacketSize)
	assertQueueLen(a, 0, 0)
}

func testCleanStart(a *assert.Assertions, store queue.Store) {
	reconnect(a, true, store)
	rs, err := store.ReadInflight(10)
	a.NoError(err)
	a.Len(rs, 0)
	initDrop()
	initNotifierLen()
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
	case <-result:
		a.Fail("Read must be blocked before Close")
	case <-t:
	}
	a.NoError(store.Close())
	timeout := time.After(5 * time.Second)
	select {
	case <-timeout:
		a.Fail("Read must be unblocked after Close")
	case r := <-result:
		a.Zero(r.len)
		a.Equal(queue.ErrClosed, r.err)
	}
}
