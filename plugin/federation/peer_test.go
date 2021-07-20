package federation

import (
	"reflect"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hashicorp/serf/serf"
	"github.com/stretchr/testify/assert"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/persistence/subscription/mem"
	"github.com/DrmagicE/gmqtt/retained"
	"github.com/DrmagicE/gmqtt/retained/trie"
)

func TestPeer_initStream_CleanStart(t *testing.T) {
	a := assert.New(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueue := NewMockqueue(ctrl)

	ls := &localSubStore{}
	ls.init(mem.NewStore())

	retained := trie.NewStore()
	p := &peer{
		fed: &Federation{
			localSubStore: ls,
			retainedStore: retained,
		},
		localName: "",
		member: serf.Member{
			Name: "node2",
		},
		exit:      nil,
		sessionID: "sessionID",
		queue:     mockQueue,
		stream:    nil,
	}
	ls.subscribe("c1", "topicA")
	ls.subscribe("c2", "topicB")

	m1 := &gmqtt.Message{
		Topic: "topicA",
	}
	m2 := &gmqtt.Message{
		Topic: "topicB",
	}
	retained.AddOrReplace(m1)
	retained.AddOrReplace(m2)

	client := NewMockFederationClient(ctrl)

	client.EXPECT().Hello(gomock.Any(), &ClientHello{
		SessionId: p.sessionID,
	}).Return(&ServerHello{
		CleanStart:  true,
		NextEventId: 0,
	}, nil)

	gomock.InOrder(
		mockQueue.EXPECT().clear(),
		mockQueue.EXPECT().setReadPosition(uint64(0)),
		mockQueue.EXPECT().open(),
	)

	// The order of the events is not significant and also is not grantee to be sorted in any way.
	// So we had to collect them into map.
	subEvents := make(map[string]string)
	msgEvents := make(map[string]string)

	expectedSubEvents := map[string]*Event{
		"topicA": {
			Event: &Event_Subscribe{
				Subscribe: &Subscribe{
					TopicFilter: "topicA",
				},
			},
		},
		"topicB": {
			Event: &Event_Subscribe{
				Subscribe: &Subscribe{
					TopicFilter: "topicB",
				},
			},
		},
	}
	expectedMsgEvents := map[string]*Event{
		"topicA": {
			Event: &Event_Message{
				Message: messageToEvent(m1),
			},
		},
		"topicB": {
			Event: &Event_Message{
				Message: messageToEvent(m2),
			},
		},
	}
	mockQueue.EXPECT().add(gomock.Any()).Do(func(event *Event) {
		switch event.Event.(type) {
		case *Event_Subscribe:
			sub := event.Event.(*Event_Subscribe)
			subEvents[sub.Subscribe.TopicFilter] = event.String()
		case *Event_Message:
			msg := event.Event.(*Event_Message)
			msgEvents[msg.Message.TopicName] = event.String()
		default:
			a.FailNow("unexpected event type: %s", reflect.TypeOf(event.Event))
		}
	}).Times(4)

	client.EXPECT().EventStream(gomock.Any())
	_, err := p.initStream(client, nil)

	a.NoError(err)
	for k, v := range msgEvents {
		a.Equal(expectedMsgEvents[k].String(), v)
	}
	for k, v := range subEvents {
		a.Equal(expectedSubEvents[k].String(), v)
	}

}

func TestPeer_initStream_CleanStartFalse(t *testing.T) {
	a := assert.New(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueue := NewMockqueue(ctrl)

	ls := &localSubStore{}
	ls.init(mem.NewStore())

	rt := retained.NewMockStore(ctrl)
	p := &peer{
		fed: &Federation{
			localSubStore: ls,
			retainedStore: rt,
		},
		localName: "",
		member: serf.Member{
			Name: "node2",
		},
		exit:      nil,
		sessionID: "sessionID",
		queue:     mockQueue,
		stream:    nil,
	}

	client := NewMockFederationClient(ctrl)
	client.EXPECT().Hello(gomock.Any(), &ClientHello{
		SessionId: p.sessionID,
	}).Return(&ServerHello{
		CleanStart:  false,
		NextEventId: 10,
	}, nil)

	gomock.InOrder(
		mockQueue.EXPECT().setReadPosition(uint64(10)),
		mockQueue.EXPECT().open(),
	)

	client.EXPECT().EventStream(gomock.Any())

	_, err := p.initStream(client, nil)
	a.NoError(err)

}
