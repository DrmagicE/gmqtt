package server

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/config"
	"github.com/DrmagicE/gmqtt/persistence/queue"
	"github.com/DrmagicE/gmqtt/persistence/subscription/mem"
	"github.com/DrmagicE/gmqtt/pkg/packets"
)

type testDeliverMsg struct {
	srv *server
}

func newTestDeliverMsg(ctrl *gomock.Controller, subscriber string) *testDeliverMsg {
	sub := mem.NewStore()
	srv := &server{
		subscriptionsDB: sub,
		queueStore:      make(map[string]queue.Store),
		config:          config.DefaultConfig(),
		statsManager:    newStatsManager(sub),
	}
	mockQueue := queue.NewMockStore(ctrl)
	srv.queueStore[subscriber] = mockQueue
	return &testDeliverMsg{
		srv: srv,
	}
}

func TestServer_deliverMessage(t *testing.T) {
	a := assert.New(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	subscriber := "subCli"
	ts := newTestDeliverMsg(ctrl, subscriber)
	srcCli := "srcCli"
	msg := &gmqtt.Message{
		Topic:   "/abc",
		Payload: []byte("abc"),
		QoS:     2,
	}
	srv := ts.srv
	srv.subscriptionsDB.Subscribe(subscriber, &gmqtt.Subscription{
		ShareName:   "",
		TopicFilter: "/abc",
		QoS:         1,
	}, &gmqtt.Subscription{
		ShareName:   "",
		TopicFilter: "/+",
		QoS:         2,
	})

	mockQueue := srv.queueStore[subscriber].(*queue.MockStore)
	// test only once
	srv.config.MQTT.DeliveryMode = OnlyOnce
	mockQueue.EXPECT().Add(gomock.Any()).Do(func(elem *queue.Elem) {
		a.EqualValues(elem.MessageWithID.(*queue.Publish).QoS, 2)
	})

	a.True(srv.deliverMessage(srcCli, msg, defaultIterateOptions(msg.Topic)))

	// test overlap
	srv.config.MQTT.DeliveryMode = Overlap
	qos := map[byte]int{
		packets.Qos1: 0,
		packets.Qos2: 0,
	}
	mockQueue.EXPECT().Add(gomock.Any()).Do(func(elem *queue.Elem) {
		_, ok := qos[elem.MessageWithID.(*queue.Publish).QoS]
		a.True(ok)
		qos[elem.MessageWithID.(*queue.Publish).QoS]++
	}).Times(2)

	a.True(srv.deliverMessage(srcCli, msg, defaultIterateOptions(msg.Topic)))

	a.Equal(1, qos[packets.Qos1])
	a.Equal(1, qos[packets.Qos2])

	msg = &gmqtt.Message{
		Topic: "abcd",
	}
	a.False(srv.deliverMessage(srcCli, msg, defaultIterateOptions(msg.Topic)))

}

func TestServer_deliverMessage_sharedSubscription(t *testing.T) {
	a := assert.New(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	subscriber := "subCli"
	ts := newTestDeliverMsg(ctrl, subscriber)
	srcCli := "srcCli"
	msg := &gmqtt.Message{
		Topic:   "/abc",
		Payload: []byte("abc"),
		QoS:     2,
	}
	srv := ts.srv
	// add 2 shared and 2 non-shared subscription which both match the message topic: /abc
	srv.subscriptionsDB.Subscribe(subscriber, &gmqtt.Subscription{
		ShareName:   "abc",
		TopicFilter: "/abc",
		QoS:         1,
	}, &gmqtt.Subscription{
		ShareName:   "abc",
		TopicFilter: "/+",
		QoS:         2,
	}, &gmqtt.Subscription{
		TopicFilter: "#",
		QoS:         2,
	}, &gmqtt.Subscription{
		TopicFilter: "/abc",
		QoS:         1,
	})

	mockQueue := srv.queueStore[subscriber].(*queue.MockStore)
	// test only once
	qos := map[byte]int{
		packets.Qos1: 0,
		packets.Qos2: 0,
	}
	srv.config.MQTT.DeliveryMode = OnlyOnce
	mockQueue.EXPECT().Add(gomock.Any()).Do(func(elem *queue.Elem) {
		_, ok := qos[elem.MessageWithID.(*queue.Publish).QoS]
		a.True(ok)
		qos[elem.MessageWithID.(*queue.Publish).QoS]++

	}).Times(3)

	a.True(srv.deliverMessage(srcCli, msg, defaultIterateOptions(msg.Topic)))
	a.Equal(1, qos[packets.Qos1])
	a.Equal(2, qos[packets.Qos2])

	// test overlap
	srv.config.MQTT.DeliveryMode = Overlap
	qos = map[byte]int{
		packets.Qos1: 0,
		packets.Qos2: 0,
	}
	mockQueue.EXPECT().Add(gomock.Any()).Do(func(elem *queue.Elem) {
		_, ok := qos[elem.MessageWithID.(*queue.Publish).QoS]
		a.True(ok)
		qos[elem.MessageWithID.(*queue.Publish).QoS]++
	}).Times(4)
	a.True(srv.deliverMessage(srcCli, msg, defaultIterateOptions(msg.Topic)))
	a.Equal(2, qos[packets.Qos1])
	a.Equal(2, qos[packets.Qos2])

}
