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
