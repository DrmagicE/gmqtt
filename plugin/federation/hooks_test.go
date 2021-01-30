package federation

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hashicorp/serf/serf"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/config"
	"github.com/DrmagicE/gmqtt/persistence/subscription/mem"
	"github.com/DrmagicE/gmqtt/server"
)

func init() {
	log = zap.NewNop()
	servePeerEventStream = func(p *peer) {
		return
	}
}

var testConfig = config.Config{
	Plugins: map[string]config.Configuration{
		Name: &Config{
			NodeName: "test-nodename",
		},
	},
}

func TestFederation_OnMsgArrivedWrapper(t *testing.T) {
	a := assert.New(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	p, _ := New(testConfig)
	f := p.(*Federation)

	onMsgArrived := f.OnMsgArrivedWrapper(func(ctx context.Context, client server.Client, req *server.MsgArrivedRequest) error {
		return nil
	})
	mockCli := server.NewMockClient(ctrl)
	mockCli.EXPECT().ClientOptions().Return(&server.ClientOptions{
		ClientID: "client1",
	}).AnyTimes()

	// must not send the message if there are no matched topic.
	msg := &gmqtt.Message{
		QoS:     1,
		Topic:   "/topicA",
		Payload: []byte("payload"),
	}
	a.NoError(onMsgArrived(context.Background(), mockCli, &server.MsgArrivedRequest{
		Message: msg,
	}))

	f.nodeJoin(serf.MemberEvent{
		Members: []serf.Member{
			{
				Name: "node2",
			},
		},
	})

	mockQueue := NewMockqueue(ctrl)
	f.peers["node2"].queue = mockQueue

	// always send retained messages
	retainedMsg := &gmqtt.Message{
		QoS:      1,
		Topic:    "/topicA",
		Payload:  []byte("payload"),
		Retained: true,
	}
	mockQueue.EXPECT().add(&Event{
		Event: &Event_Message{
			Message: messageToEvent(retainedMsg),
		},
	})
	a.NoError(onMsgArrived(context.Background(), mockCli, &server.MsgArrivedRequest{
		Message: retainedMsg,
	}))

	// send the message only once even the message has multiple matched topics.
	f.feSubStore.Subscribe("node2", &gmqtt.Subscription{
		TopicFilter: "/topicA",
	}, &gmqtt.Subscription{
		TopicFilter: "#",
	})
	mockQueue.EXPECT().add(&Event{
		Event: &Event_Message{
			Message: messageToEvent(msg),
		},
	})
	a.NoError(onMsgArrived(context.Background(), mockCli, &server.MsgArrivedRequest{
		Message: msg,
	}))

	// send only once if a retained message also has matched topic
	mockQueue.EXPECT().add(&Event{
		Event: &Event_Message{
			Message: messageToEvent(retainedMsg),
		},
	})

	a.NoError(onMsgArrived(context.Background(), mockCli, &server.MsgArrivedRequest{
		Message: retainedMsg,
	}))

}

func TestFederation_OnSubscribedWrapper(t *testing.T) {
	a := assert.New(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	p, _ := New(testConfig)
	f := p.(*Federation)
	f.localSubStore.init(mem.NewStore())
	f.nodeJoin(serf.MemberEvent{
		Members: []serf.Member{
			{
				Name: "node2",
			},
		},
	})
	mockQueue := NewMockqueue(ctrl)
	f.peers["node2"].queue = mockQueue
	onSubscribed := f.OnSubscribedWrapper(func(ctx context.Context, client server.Client, subscription *gmqtt.Subscription) {
		return
	})

	client1 := server.NewMockClient(ctrl)
	client1.EXPECT().ClientOptions().Return(&server.ClientOptions{
		ClientID: "client1",
	}).AnyTimes()

	client2 := server.NewMockClient(ctrl)
	client2.EXPECT().ClientOptions().Return(&server.ClientOptions{
		ClientID: "client2",
	}).AnyTimes()

	// only subscribe once
	mockQueue.EXPECT().add(&Event{
		Event: &Event_Subscribe{
			Subscribe: &Subscribe{
				TopicFilter: "/topicA",
			},
		},
	})
	onSubscribed(context.Background(), client1, &gmqtt.Subscription{
		TopicFilter: "/topicA",
	})
	onSubscribed(context.Background(), client2, &gmqtt.Subscription{
		TopicFilter: "/topicA",
	})

	a.EqualValues(2, f.localSubStore.topics["/topicA"])
}

func TestFederation_OnUnsubscribedWrapper(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	p, _ := New(testConfig)
	f := p.(*Federation)
	f.localSubStore.init(mem.NewStore())
	f.nodeJoin(serf.MemberEvent{
		Members: []serf.Member{
			{
				Name: "node2",
			},
		},
	})

	mockQueue := NewMockqueue(ctrl)
	f.peers["node2"].queue = mockQueue

	// 2 subscription for /topicA
	f.localSubStore.subscribe("client1", "/topicA")
	f.localSubStore.subscribe("client2", "/topicA")

	onUnsubscribed := f.OnUnsubscribedWrapper(func(ctx context.Context, client server.Client, topicName string) {
		return
	})
	client1 := server.NewMockClient(ctrl)
	client1.EXPECT().ClientOptions().Return(&server.ClientOptions{
		ClientID: "client1",
	}).AnyTimes()

	client2 := server.NewMockClient(ctrl)
	client2.EXPECT().ClientOptions().Return(&server.ClientOptions{
		ClientID: "client2",
	}).AnyTimes()
	onUnsubscribed(context.Background(), client1, "/topicA")

	// only unsubscribe when all local subscription for /topicA have been unsubscribed
	mockQueue.EXPECT().add(&Event{
		Event: &Event_Unsubscribe{
			Unsubscribe: &Unsubscribe{
				TopicName: "/topicA",
			},
		},
	})
	onUnsubscribed(context.Background(), client2, "/topicA")
	// should not send unsubscribe event if the unsubscribed topic not exists.
	onUnsubscribed(context.Background(), client2, "/topicA")
}

func TestFederation_OnSessionTerminatedWrapper(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	p, _ := New(testConfig)
	f := p.(*Federation)
	f.localSubStore.init(mem.NewStore())
	f.nodeJoin(serf.MemberEvent{
		Members: []serf.Member{
			{
				Name: "node2",
			},
		},
	})

	mockQueue := NewMockqueue(ctrl)
	f.peers["node2"].queue = mockQueue

	// 2 subscription for /topicA
	f.localSubStore.subscribe("client1", "/topicA")
	f.localSubStore.subscribe("client2", "/topicA")
	// 1 for /topicB & /topicC
	f.localSubStore.subscribe("client3", "/topicB")
	f.localSubStore.subscribe("client3", "/topicC")

	onSessionTerminated := f.OnSessionTerminatedWrapper(func(ctx context.Context, clientID string, reason server.SessionTerminatedReason) {
		return
	})

	onSessionTerminated(context.Background(), "client1", 0)

	mockQueue.EXPECT().add(&Event{
		Event: &Event_Unsubscribe{
			Unsubscribe: &Unsubscribe{
				TopicName: "/topicA",
			},
		},
	})
	onSessionTerminated(context.Background(), "client2", 0)

	mockQueue.EXPECT().add(&Event{
		Event: &Event_Unsubscribe{
			Unsubscribe: &Unsubscribe{
				TopicName: "/topicB",
			},
		},
	})
	mockQueue.EXPECT().add(&Event{
		Event: &Event_Unsubscribe{
			Unsubscribe: &Unsubscribe{
				TopicName: "/topicC",
			},
		},
	})
	onSessionTerminated(context.Background(), "client3", 0)
}