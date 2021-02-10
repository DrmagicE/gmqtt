package server

import (
	"bytes"
	"container/list"
	"io"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/config"
	"github.com/DrmagicE/gmqtt/persistence/queue"
	"github.com/DrmagicE/gmqtt/persistence/subscription"
	"github.com/DrmagicE/gmqtt/persistence/subscription/mem"
	"github.com/DrmagicE/gmqtt/persistence/unack"
	unack_mem "github.com/DrmagicE/gmqtt/persistence/unack/mem"
	"github.com/DrmagicE/gmqtt/pkg/codes"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/DrmagicE/gmqtt/retained"
)

const testRedeliveryInternal = 10 * time.Second

type dummyAddr string

type testListener struct {
	conn        list.List
	acceptReady chan struct{}
}

var srv *server

func (l *testListener) Accept() (c net.Conn, err error) {
	<-l.acceptReady
	if l.conn.Len() != 0 {
		e := l.conn.Front()
		c = e.Value.(net.Conn)
		err = nil
		l.conn.Remove(e)
	} else {
		c = nil
		err = io.EOF
	}
	return
}

func (l *testListener) Close() error {
	return nil
}

func (l *testListener) Addr() net.Addr {
	return dummyAddr("test-address")
}

func (a dummyAddr) Network() string {
	return string(a)
}

func (a dummyAddr) String() string {
	return string(a)
}

type noopConn struct{}

func (noopConn) Read(b []byte) (n int, err error) {
	return 0, nil
}
func (noopConn) Write(b []byte) (n int, err error) {
	return 0, nil
}
func (noopConn) Close() error {
	return nil
}
func (noopConn) RemoteAddr() net.Addr {
	return dummyAddr("dummy")
}
func (noopConn) LocalAddr() net.Addr { return dummyAddr("local-addr") }

func (noopConn) SetDeadline(t time.Time) error      { return nil }
func (noopConn) SetReadDeadline(t time.Time) error  { return nil }
func (noopConn) SetWriteDeadline(t time.Time) error { return nil }

func TestClient_subscribeHandler_common(t *testing.T) {
	var tt = []struct {
		name     string
		clientID string
		version  packets.Version
		in       *packets.Subscribe
		out      *packets.Suback
		err      *codes.Error
	}{
		{
			name:     "success_v5",
			clientID: "cid",
			version:  packets.Version5,
			in: &packets.Subscribe{
				Version:  packets.Version5,
				PacketID: 1,
				Topics: []packets.Topic{
					{
						SubOptions: packets.SubOptions{
							Qos:               1,
							RetainHandling:    0,
							NoLocal:           false,
							RetainAsPublished: false,
						},
						Name: "/topic/A",
					}, {
						SubOptions: packets.SubOptions{
							Qos:               2,
							RetainHandling:    0,
							NoLocal:           false,
							RetainAsPublished: false,
						},
						Name: "/topic/B",
					},
				},
				Properties: &packets.Properties{},
			},
			err: nil,
			out: &packets.Suback{
				Version:    packets.Version5,
				Properties: &packets.Properties{},
				Payload: []codes.Code{
					codes.GrantedQoS1, codes.GrantedQoS2,
				},
			},
		},
		{
			name:     "success_v3",
			clientID: "cid",
			in: &packets.Subscribe{
				Version:  packets.Version311,
				PacketID: 2,
				Topics: []packets.Topic{
					{

						SubOptions: packets.SubOptions{
							Qos: 1,
						},
						Name: "/topic/A",
					}, {
						SubOptions: packets.SubOptions{
							Qos: 2,
						},
						Name: "/topic/B",
					},
				},
				// no properties for v3.1.1
				Properties: nil,
			},
			err: nil,
			out: &packets.Suback{
				Version:    packets.Version311,
				Properties: &packets.Properties{},
				PacketID:   2,
				Payload: []codes.Code{
					codes.GrantedQoS1, codes.GrantedQoS2,
				},
			},
		},
		{
			name:     "subscription_identifier",
			clientID: "cid",
			version:  packets.Version5,
			in: &packets.Subscribe{
				Version:  packets.Version5,
				PacketID: 1,
				Topics: []packets.Topic{
					{
						SubOptions: packets.SubOptions{
							Qos:               1,
							RetainHandling:    0,
							NoLocal:           false,
							RetainAsPublished: false,
						},
						Name: "/topic/A",
					}, {
						SubOptions: packets.SubOptions{
							Qos:               2,
							RetainHandling:    0,
							NoLocal:           false,
							RetainAsPublished: false,
						},
						Name: "/topic/B",
					},
				},
				Properties: &packets.Properties{
					SubscriptionIdentifier: []uint32{
						5,
					},
				},
			},
			err: nil,
			out: &packets.Suback{
				Version:    packets.Version5,
				Properties: &packets.Properties{},
				PacketID:   1,
				Payload: []codes.Code{
					codes.GrantedQoS1, codes.GrantedQoS2,
				},
			},
		},
	}
	for _, v := range tt {
		t.Run(v.name, func(t *testing.T) {
			a := assert.New(t)
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			subDB := subscription.NewMockStore(ctrl)
			retainedDB := retained.NewMockStore(ctrl)

			srv := &server{
				config:          config.DefaultConfig(),
				subscriptionsDB: subDB,
				retainedDB:      retainedDB,
			}
			c, er := srv.newClient(noopConn{})
			a.Nil(er)
			c.opts.ClientID = v.clientID
			c.opts.SubIDAvailable = true
			c.version = v.version
			for _, topic := range v.in.Topics {

				sub := &gmqtt.Subscription{
					TopicFilter:       topic.Name,
					QoS:               topic.Qos,
					NoLocal:           topic.NoLocal,
					RetainAsPublished: topic.RetainAsPublished,
					RetainHandling:    topic.RetainHandling,
				}
				if v.in.Properties != nil && len(v.in.Properties.SubscriptionIdentifier) != 0 {
					sub.ID = v.in.Properties.SubscriptionIdentifier[0]
				}
				subDB.EXPECT().Subscribe(v.clientID, sub).Return(subscription.SubscribeResult{
					{
						Subscription:   sub,
						AlreadyExisted: false,
					},
				}, nil)
				// We are not going to test retained logic in this test case.
				retainedDB.EXPECT().GetMatchedMessages(sub.TopicFilter).Return(nil)
			}

			err := c.subscribeHandler(v.in)
			a.Equal(v.err, err)
			select {
			case p := <-c.out:
				suback := p.(*packets.Suback)
				a.Equal(v.in.PacketID, suback.PacketID)
				a.Equal(v.out.Payload, suback.Payload)
				a.Equal(v.out.Version, suback.Version)
				a.Equal(v.out.Properties, suback.Properties)
			default:
				t.Fatal("missing output")
			}

		})
	}

}

func TestClient_subscribeHandler_shareSubscription(t *testing.T) {
	var tt = []struct {
		name               string
		clientID           string
		version            packets.Version
		in                 *packets.Subscribe
		out                *packets.Suback
		err                *codes.Error
		sharedSubAvailable bool
	}{
		{
			name:     "success_v5",
			clientID: "cid",
			version:  packets.Version5,
			in: &packets.Subscribe{
				Version:  packets.Version5,
				PacketID: 1,
				Topics: []packets.Topic{
					{
						SubOptions: packets.SubOptions{
							Qos:               1,
							RetainHandling:    0,
							NoLocal:           false,
							RetainAsPublished: false,
						},
						Name: "$share/topic/A",
					}, {
						SubOptions: packets.SubOptions{
							Qos:               2,
							RetainHandling:    0,
							NoLocal:           false,
							RetainAsPublished: false,
						},
						Name: "$share/topic/B",
					},
				},
				Properties: &packets.Properties{},
			},
			err: nil,
			out: &packets.Suback{
				Version:    packets.Version5,
				Properties: &packets.Properties{},
				Payload: []codes.Code{
					codes.GrantedQoS1, codes.GrantedQoS2,
				},
			},
			sharedSubAvailable: true,
		},
		{
			name:     "shared_not_supported_v5",
			clientID: "cid",
			version:  packets.Version5,
			in: &packets.Subscribe{
				Version:  packets.Version5,
				PacketID: 1,
				Topics: []packets.Topic{
					{
						SubOptions: packets.SubOptions{
							Qos:               1,
							RetainHandling:    0,
							NoLocal:           false,
							RetainAsPublished: false,
						},
						Name: "$share/topic/A",
					}, {
						SubOptions: packets.SubOptions{
							Qos:               2,
							RetainHandling:    0,
							NoLocal:           false,
							RetainAsPublished: false,
						},
						Name: "$share/topic/B",
					},
				},
				Properties: &packets.Properties{},
			},
			err: nil,
			out: &packets.Suback{
				Version:    packets.Version5,
				Properties: &packets.Properties{},
				Payload: []codes.Code{
					codes.SharedSubNotSupported, codes.SharedSubNotSupported,
				},
			},
			sharedSubAvailable: false,
		},
	}
	for _, v := range tt {
		t.Run(v.name, func(t *testing.T) {
			a := assert.New(t)
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			subDB := subscription.NewMockStore(ctrl)
			retainedDB := retained.NewMockStore(ctrl)
			srv := &server{
				config:          config.DefaultConfig(),
				subscriptionsDB: subDB,
				retainedDB:      retainedDB,
			}
			c, er := srv.newClient(noopConn{})
			a.Nil(er)
			c.opts.ClientID = v.clientID
			c.opts.SharedSubAvailable = v.sharedSubAvailable
			c.version = v.version
			for _, topic := range v.in.Topics {
				var shareName, topicFilter string

				shared := strings.SplitN(topic.Name, "/", 3)
				shareName = shared[1]
				topicFilter = shared[2]

				sub := &gmqtt.Subscription{
					ShareName:         shareName,
					TopicFilter:       topicFilter,
					QoS:               topic.Qos,
					NoLocal:           topic.NoLocal,
					RetainAsPublished: topic.RetainAsPublished,
					RetainHandling:    topic.RetainHandling,
				}
				if v.in.Properties != nil && len(v.in.Properties.SubscriptionIdentifier) != 0 {
					sub.ID = v.in.Properties.SubscriptionIdentifier[0]
				}

				if v.sharedSubAvailable {
					subDB.EXPECT().Subscribe(v.clientID, sub).Return(subscription.SubscribeResult{
						{
							Subscription:   sub,
							AlreadyExisted: false,
						},
					}, nil)
				}
			}

			err := c.subscribeHandler(v.in)
			a.Equal(v.err, err)
			select {
			case p := <-c.out:
				suback := p.(*packets.Suback)
				a.Equal(v.in.PacketID, suback.PacketID)
				a.Equal(v.out.Payload, suback.Payload)
				a.Equal(v.out.Version, suback.Version)
				a.Equal(v.out.Properties, suback.Properties)
			default:
				t.Fatal("missing output")
			}
		})
	}

}

func TestClient_subscribeHandler_retainedMessage(t *testing.T) {
	var tt = []struct {
		name              string
		clientID          string
		version           packets.Version
		in                *packets.Subscribe
		out               *packets.Suback
		err               *codes.Error
		retainedMsg       *gmqtt.Message
		retainedAvailable bool
		expected          struct {
			qos      uint8
			retained bool
		}
		alreadyExisted     bool
		shouldSendRetained bool
	}{
		{
			// If Retain Handling is set to 0 the Server MUST send the retained messages matching the Topic Filter of the subscription to the Client [MQTT-3.3.1-9].
			name:     "retain_handling_0",
			clientID: "cid",
			version:  packets.Version5,
			in: &packets.Subscribe{
				Version:  packets.Version5,
				PacketID: 1,
				Topics: []packets.Topic{
					{
						SubOptions: packets.SubOptions{
							Qos:               1,
							RetainHandling:    0,
							NoLocal:           false,
							RetainAsPublished: false,
						},
						Name: "/topic/A",
					},
				},
				Properties: &packets.Properties{},
			},
			err: nil,
			out: &packets.Suback{
				Version:  packets.Version5,
				PacketID: 1,
				Payload: []codes.Code{
					codes.GrantedQoS1,
				},
			},
			retainedMsg: &gmqtt.Message{
				Retained: true,
				QoS:      1,
				Topic:    "/topic/a",
				Payload:  []byte("b"),
			},
			expected: struct {
				qos      uint8
				retained bool
			}{qos: 1, retained: false},
			alreadyExisted:     false,
			shouldSendRetained: true,
		},
		{
			//  If Retain Handling is set to 1 then if the subscription did not already exist,
			//  the Server MUST send all retained message matching the Topic Filter of the subscription to the Client,
			//  and if the subscription did exist the Server MUST NOT send the retained messages. [MQTT-3.3.1-10].
			name:     "retain_handling_1",
			clientID: "cid",
			version:  packets.Version5,
			in: &packets.Subscribe{
				Version:  packets.Version5,
				PacketID: 1,
				Topics: []packets.Topic{
					{
						// this topic will be marked as alreadyExisted, should not send any retained messages
						SubOptions: packets.SubOptions{
							Qos:               1,
							RetainHandling:    1,
							NoLocal:           false,
							RetainAsPublished: false,
						},
						Name: "/topic/A",
					},
				},
				Properties: &packets.Properties{},
			},
			err: nil,
			out: &packets.Suback{
				PacketID: 1,
				Version:  packets.Version5,
				Payload: []codes.Code{
					codes.GrantedQoS1,
				},
			},
			alreadyExisted: true,
		},
		{
			// If Retain Handling is set to 2, the Server MUST NOT send the retained messages [MQTT-3.3.1-11].
			name:     "retain_handling_2",
			clientID: "cid",
			version:  packets.Version5,
			in: &packets.Subscribe{
				Version:  packets.Version5,
				PacketID: 1,
				Topics: []packets.Topic{
					{
						SubOptions: packets.SubOptions{
							Qos:               1,
							RetainHandling:    2,
							NoLocal:           false,
							RetainAsPublished: false,
						},
						Name: "/topic/A",
					},
				},
				Properties: &packets.Properties{},
			},
			err: nil,
			out: &packets.Suback{
				PacketID: 1,
				Version:  packets.Version5,
				Payload: []codes.Code{
					codes.GrantedQoS1,
				},
			},
			alreadyExisted:     false,
			shouldSendRetained: false,
		},
		{
			name:     "retain_handling_2_already_existed",
			clientID: "cid",
			version:  packets.Version5,
			in: &packets.Subscribe{
				Version:  packets.Version5,
				PacketID: 1,
				Topics: []packets.Topic{
					{
						SubOptions: packets.SubOptions{
							Qos:               1,
							RetainHandling:    2,
							NoLocal:           false,
							RetainAsPublished: false,
						},
						Name: "/topic/A",
					},
				},
				Properties: &packets.Properties{},
			},
			err: nil,
			out: &packets.Suback{
				Version:  packets.Version5,
				PacketID: 1,
				Payload: []codes.Code{
					codes.GrantedQoS1,
				},
			},
			alreadyExisted:     true,
			shouldSendRetained: false,
		},
		{
			name:     "qos",
			clientID: "cid",
			version:  packets.Version5,
			in: &packets.Subscribe{
				Version:  packets.Version5,
				PacketID: 1,
				Topics: []packets.Topic{
					{
						SubOptions: packets.SubOptions{
							Qos:               2,
							RetainHandling:    0,
							NoLocal:           false,
							RetainAsPublished: false,
						},
						Name: "/topic/A",
					},
				},
				Properties: &packets.Properties{},
			},
			err: nil,
			out: &packets.Suback{
				Version:  packets.Version5,
				PacketID: 1,
				Payload: []codes.Code{
					codes.GrantedQoS2,
				},
			},
			retainedMsg: &gmqtt.Message{
				QoS:      1,
				Retained: true,
				Topic:    "/topic/a",
				Payload:  []byte("b"),
			},
			expected: struct {
				qos      uint8
				retained bool
			}{qos: 1, retained: false},
			alreadyExisted:     false,
			shouldSendRetained: true,
		},
		{
			// If the value of Retain As Published subscription option is set to 1,
			// the Server MUST set the RETAIN flag equal to the RETAIN flag in the received PUBLISH in [MQTT-3.3.1-13].
			name:     "rap_1",
			clientID: "cid",
			version:  packets.Version5,
			in: &packets.Subscribe{
				Version:  packets.Version5,
				PacketID: 1,
				Topics: []packets.Topic{
					{
						SubOptions: packets.SubOptions{
							Qos:               2,
							RetainHandling:    0,
							NoLocal:           false,
							RetainAsPublished: true,
						},
						Name: "/topic/A",
					},
				},
				Properties: &packets.Properties{},
			},
			err: nil,
			out: &packets.Suback{
				Version:  packets.Version5,
				PacketID: 1,
				Payload: []codes.Code{
					codes.GrantedQoS2,
				},
			},
			retainedMsg: &gmqtt.Message{
				QoS:      1,
				Retained: true,
				Topic:    "/topic/a",
				Payload:  []byte("b"),
			},
			expected: struct {
				qos      uint8
				retained bool
			}{qos: 1, retained: true},
			alreadyExisted:     false,
			shouldSendRetained: true,
		},
		{
			// If the value of Retain As Published subscription option is set to 1,
			// the Server MUST set the RETAIN flag equal to the RETAIN flag in the received PUBLISH in [MQTT-3.3.1-13].
			name:     "rap_1",
			clientID: "cid",
			version:  packets.Version5,
			in: &packets.Subscribe{
				Version:  packets.Version5,
				PacketID: 1,
				Topics: []packets.Topic{
					{
						SubOptions: packets.SubOptions{
							Qos:               2,
							RetainHandling:    0,
							NoLocal:           false,
							RetainAsPublished: true,
						},
						Name: "/topic/A",
					},
				},
				Properties: &packets.Properties{},
			},
			err: nil,
			out: &packets.Suback{
				Version:  packets.Version5,
				PacketID: 1,
				Payload: []codes.Code{
					codes.GrantedQoS2,
				},
			},
			retainedMsg: &gmqtt.Message{
				QoS:      1,
				Retained: true,
				Topic:    "/topic/a",
				Payload:  []byte("b"),
			},
			expected: struct {
				qos      uint8
				retained bool
			}{qos: 1, retained: true},
			alreadyExisted:     false,
			shouldSendRetained: true,
		},
	}
	for _, v := range tt {
		t.Run(v.name, func(t *testing.T) {
			a := assert.New(t)
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			subDB := subscription.NewMockStore(ctrl)
			retainedDB := retained.NewMockStore(ctrl)
			qs := queue.NewMockStore(ctrl)
			srv := &server{
				config:          config.DefaultConfig(),
				subscriptionsDB: subDB,
				retainedDB:      retainedDB,
			}
			c, er := srv.newClient(noopConn{})
			a.Nil(er)
			c.opts.ClientID = v.clientID
			c.queueStore = qs
			srv.queueStore = make(map[string]queue.Store)
			srv.queueStore[v.clientID] = qs

			if v.shouldSendRetained {
				qs.EXPECT().Add(gomock.Any()).DoAndReturn(func(elem *queue.Elem) error {
					if v.shouldSendRetained {
						a.Equal(v.expected.qos, elem.MessageWithID.(*queue.Publish).QoS)
						a.Equal(v.expected.retained, elem.MessageWithID.(*queue.Publish).Retained)
					}
					return nil
				}).Return(nil)
			}

			c.opts.RetainAvailable = v.retainedAvailable
			c.version = v.version
			for _, topic := range v.in.Topics {
				sub := &gmqtt.Subscription{
					TopicFilter:       topic.Name,
					QoS:               topic.Qos,
					NoLocal:           topic.NoLocal,
					RetainAsPublished: topic.RetainAsPublished,
					RetainHandling:    topic.RetainHandling,
				}
				if v.in.Properties != nil && len(v.in.Properties.SubscriptionIdentifier) != 0 {
					sub.ID = v.in.Properties.SubscriptionIdentifier[0]
				}
				subDB.EXPECT().Subscribe(v.clientID, sub).Return(subscription.SubscribeResult{
					{
						Subscription:   sub,
						AlreadyExisted: v.alreadyExisted,
					},
				}, nil)
				if v.shouldSendRetained {
					retainedDB.EXPECT().GetMatchedMessages(sub.TopicFilter).Return([]*gmqtt.Message{v.retainedMsg})
				}
			}

			err := c.subscribeHandler(v.in)
			a.Equal(v.err, err)
			select {
			case p := <-c.out:
				suback := p.(*packets.Suback)
				a.Equal(v.in.PacketID, suback.PacketID)

				want := &bytes.Buffer{}
				got := &bytes.Buffer{}
				a.Nil(v.out.Pack(want))
				a.Nil(suback.Pack(got))
				a.Equal(want.Bytes(), got.Bytes())
			default:
				t.Fatal("missing output")
			}
		})
	}

}

func TestClient_publishHandler_common(t *testing.T) {
	var tt = []struct {
		name         string
		clientID     string
		version      packets.Version
		in           *packets.Publish
		out          packets.Packet
		err          *codes.Error
		topicMatched bool
	}{
		{
			name:     "qos0",
			clientID: "cid",
			version:  packets.Version5,
			in: &packets.Publish{
				Version:    packets.Version5,
				Dup:        false,
				Qos:        0,
				Retain:     false,
				TopicName:  []byte("/topic/A"),
				PacketID:   1,
				Payload:    []byte("b"),
				Properties: &packets.Properties{},
			},
			topicMatched: true,
		},
		{
			name:     "qos1",
			clientID: "cid",
			version:  packets.Version5,
			in: &packets.Publish{
				Version:    packets.Version5,
				Dup:        false,
				Qos:        1,
				Retain:     false,
				TopicName:  []byte("/topic/A"),
				PacketID:   1,
				Payload:    []byte("b"),
				Properties: &packets.Properties{},
			},
			topicMatched: true,
		},
		{
			name:     "qos2",
			clientID: "cid",
			version:  packets.Version5,
			in: &packets.Publish{
				Version:    packets.Version5,
				Dup:        false,
				Qos:        2,
				Retain:     false,
				TopicName:  []byte("/topic/A"),
				PacketID:   1,
				Payload:    []byte("b"),
				Properties: &packets.Properties{},
			},
			topicMatched: true,
		},
	}
	for _, v := range tt {
		t.Run(v.name, func(t *testing.T) {
			a := assert.New(t)
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			retainedDB := retained.NewMockStore(ctrl)
			subscriptionDB := subscription.NewMockStore(ctrl)
			srv := &server{
				config:          config.DefaultConfig(),
				retainedDB:      retainedDB,
				subscriptionsDB: subscriptionDB,
			}

			var deliverMessageCalled bool

			srv.deliverMessageHandler = func(srcClientID string, msg *gmqtt.Message, options subscription.IterationOptions) (matched bool) {
				a.Equal(v.clientID, srcClientID)
				a.Equal(gmqtt.MessageFromPublish(v.in), msg)
				deliverMessageCalled = true
				return v.topicMatched
			}

			c, er := srv.newClient(noopConn{})
			a.Nil(er)
			c.unackStore = unack_mem.New(unack_mem.Options{
				ClientID: v.clientID,
			})
			srv.unackStore = make(map[string]unack.Store)
			srv.unackStore[v.clientID] = c.unackStore

			a.Nil(er)
			c.opts.ClientID = v.clientID
			c.version = v.version

			err := c.publishHandler(v.in)
			a.Equal(v.err, err)
			a.True(deliverMessageCalled)

			select {
			case p := <-c.out:
				switch v.in.Qos {
				case packets.Qos0:
					a.Fail("qos0 should not send any ack packet")
				case packets.Qos1:
					a.Equal(v.in.NewPuback(codes.Success, nil), p)
				case packets.Qos2:
					a.Equal(v.in.NewPubrec(codes.Success, nil), p)
					bo, err := c.unackStore.Set(v.in.PacketID)
					a.Nil(err)
					a.True(bo)
				}
			default:
				if v.in.Qos != packets.Qos0 {
					t.Fatal("missing output")
				}
			}
		})
	}

}

func TestClient_publishHandler_retainedMessage(t *testing.T) {
	var tt = []struct {
		name              string
		clientID          string
		version           packets.Version
		in                *packets.Publish
		out               packets.Packet
		err               *codes.Error
		topicMatched      bool
		retainedAvailable bool
	}{
		{
			name:     "addOrReplace",
			clientID: "cid",
			version:  packets.Version5,
			in: &packets.Publish{
				Version:    packets.Version5,
				Dup:        false,
				Qos:        0,
				Retain:     true,
				TopicName:  []byte("/topic/A"),
				PacketID:   1,
				Payload:    []byte("b"),
				Properties: &packets.Properties{},
			},
			topicMatched:      true,
			retainedAvailable: true,
		},
		{
			name:     "remove",
			clientID: "cid",
			version:  packets.Version5,
			in: &packets.Publish{
				Version:    packets.Version5,
				Dup:        false,
				Qos:        0,
				Retain:     true,
				TopicName:  []byte("/topic/A"),
				PacketID:   1,
				Payload:    []byte{},
				Properties: &packets.Properties{},
			},
			topicMatched:      true,
			retainedAvailable: true,
		},
		{
			name:     "notAvailable",
			clientID: "cid",
			version:  packets.Version5,
			in: &packets.Publish{
				Version:    packets.Version5,
				Dup:        false,
				Qos:        0,
				Retain:     true,
				TopicName:  []byte("/topic/A"),
				PacketID:   1,
				Payload:    []byte{},
				Properties: &packets.Properties{},
			},
			topicMatched:      true,
			retainedAvailable: false,
			err:               codes.NewError(codes.RetainNotSupported),
		},
	}
	for _, v := range tt {
		t.Run(v.name, func(t *testing.T) {
			a := assert.New(t)
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			retainedDB := retained.NewMockStore(ctrl)
			srv := &server{
				config:     config.DefaultConfig(),
				retainedDB: retainedDB,
			}
			srv.deliverMessageHandler = func(srcClientID string, msg *gmqtt.Message, options subscription.IterationOptions) (matched bool) {
				a.Equal(v.clientID, srcClientID)
				a.Equal(gmqtt.MessageFromPublish(v.in), msg)
				return v.topicMatched
			}
			c, er := srv.newClient(noopConn{})
			a.Nil(er)
			c.unackStore = unack_mem.New(unack_mem.Options{
				ClientID: v.clientID,
			})
			srv.unackStore = make(map[string]unack.Store)
			srv.unackStore[v.clientID] = c.unackStore
			c.opts.ClientID = v.clientID
			c.version = v.version
			c.opts.RetainAvailable = v.retainedAvailable

			if v.retainedAvailable {
				if len(v.in.Payload) == 0 {
					retainedDB.EXPECT().Remove(string(v.in.TopicName))
				} else {
					retainedDB.EXPECT().AddOrReplace(gmqtt.MessageFromPublish(v.in))
				}
			}
			err := c.publishHandler(v.in)
			a.Equal(v.err, err)
			select {
			case p := <-c.out:
				switch v.in.Qos {
				case packets.Qos0:
					a.Fail("qos0 should not send any ack packet")
				case packets.Qos1:
					a.Equal(v.in.NewPuback(codes.Success, nil), p)
				case packets.Qos2:
					a.Equal(v.in.NewPubrec(codes.Success, nil), p)
					bo, err := c.unackStore.Set(v.in.PacketID)
					a.Nil(err)
					a.True(bo)
				}
			default:
				if v.in.Qos != packets.Qos0 {
					t.Fatal("missing output")
				}
			}
		})
	}

}

func TestClient_publishHandler_topicAlias(t *testing.T) {
	var tt = []struct {
		name          string
		clientID      string
		version       packets.Version
		in            *packets.Publish
		out           packets.Packet
		err           *codes.Error
		topicAliasMax uint16
	}{
		{
			name:     "invalid",
			clientID: "cid",
			version:  packets.Version5,
			in: &packets.Publish{
				Version:   packets.Version5,
				Dup:       false,
				Qos:       0,
				Retain:    false,
				TopicName: []byte("/topic/A"),
				PacketID:  1,
				Payload:   []byte("b"),
				Properties: &packets.Properties{
					TopicAlias: uint16P(3),
				},
			},
			topicAliasMax: 2,
			err:           codes.NewError(codes.TopicAliasInvalid),
		},
		{
			name:     "invalidEmptyTopicName",
			clientID: "cid",
			version:  packets.Version5,
			in: &packets.Publish{
				Version:   packets.Version5,
				Dup:       false,
				Qos:       0,
				Retain:    false,
				TopicName: []byte{},
				PacketID:  1,
				Payload:   []byte("b"),
				Properties: &packets.Properties{
					TopicAlias: uint16P(3),
				},
			},
			topicAliasMax: 2,
			err:           codes.NewError(codes.TopicAliasInvalid),
		},
	}
	for _, v := range tt {
		t.Run(v.name, func(t *testing.T) {
			a := assert.New(t)
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			srv := &server{
				config: config.DefaultConfig(),
			}
			srv.deliverMessageHandler = func(srcClientID string, msg *gmqtt.Message, options subscription.IterationOptions) (matched bool) {
				a.Equal(v.clientID, srcClientID)
				a.Equal(gmqtt.MessageFromPublish(v.in), msg)
				return true
			}
			c, er := srv.newClient(noopConn{})
			a.Nil(er)

			c.opts.ClientID = v.clientID
			c.version = v.version
			c.opts.ServerTopicAliasMax = v.topicAliasMax

			err := c.publishHandler(v.in)
			a.Equal(v.err, err)

			select {
			case p := <-c.out:
				if err != nil {
					t.Fatalf("unexpected out packet %v", reflect.TypeOf(p))
				}
			default:
			}

		})
	}

}

func TestClient_publishHandler_matchTopicAlias(t *testing.T) {
	topicName := []byte("/topic/A")
	first := &packets.Publish{
		Version:   packets.Version5,
		Dup:       false,
		Qos:       0,
		Retain:    false,
		TopicName: topicName,
		PacketID:  1,
		Payload:   []byte("b"),
		Properties: &packets.Properties{
			TopicAlias: uint16P(3),
		},
	}
	delivered := first
	second := &packets.Publish{
		Version:   packets.Version5,
		Dup:       false,
		Qos:       0,
		Retain:    false,
		TopicName: []byte{},
		PacketID:  1,
		Payload:   []byte("b"),
		Properties: &packets.Properties{
			TopicAlias: uint16P(3),
		},
	}
	a := assert.New(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	srv := &server{
		config: config.DefaultConfig(),
	}
	var deliveredMsg []*gmqtt.Message
	srv.deliverMessageHandler = func(srcClientID string, msg *gmqtt.Message, options subscription.IterationOptions) (matched bool) {
		a.Equal("cid", srcClientID)
		deliveredMsg = append(deliveredMsg, msg)
		return true
	}
	serverTopicAliasMax := uint16(5)
	c, er := srv.newClient(noopConn{})
	a.Nil(er)
	c.aliasMapper = make([][]byte, serverTopicAliasMax+1)
	c.opts.ClientID = "cid"
	c.version = packets.Version5
	c.opts.ServerTopicAliasMax = serverTopicAliasMax

	err := c.publishHandler(first)
	a.Nil(err)

	err = c.publishHandler(second)
	a.Nil(err)

	a.Len(deliveredMsg, 2)
	a.Equal(gmqtt.MessageFromPublish(delivered), deliveredMsg[0])
	a.Equal(gmqtt.MessageFromPublish(delivered), deliveredMsg[1])
}

func TestClient_pubrelHandler(t *testing.T) {
	a := assert.New(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	srv := defaultServer()
	c, er := srv.newClient(noopConn{})
	a.Nil(er)
	c.opts.ClientID = "cid"
	ua := unack.NewMockStore(ctrl)
	c.unackStore = ua
	srv.unackStore = make(map[string]unack.Store)
	srv.unackStore[c.opts.ClientID] = ua

	ua.EXPECT().Remove(packets.PacketID(1))

	c.version = packets.Version5
	pubrel := &packets.Pubrel{
		PacketID:   1,
		Code:       codes.Success,
		Properties: &packets.Properties{},
	}
	a.Nil(c.pubrelHandler(pubrel))

	select {
	case p := <-c.out:
		a.IsType(&packets.Pubcomp{}, p)
		a.Equal(pubrel.PacketID, p.(*packets.Pubcomp).PacketID)
	default:
		t.Fatal("missing output")

	}
}

func TestClient_pubrecHandler_ErrorV5(t *testing.T) {
	a := assert.New(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	srv := defaultServer()
	srv.statsManager = newStatsManager(mem.NewStore())
	c, er := srv.newClient(noopConn{})
	a.Nil(er)
	c.opts.ClientID = "cid"
	c.version = packets.Version5
	c.opts.MaxInflight = 10
	c.newPacketIDLimiter(c.opts.MaxInflight)
	qs := queue.NewMockStore(ctrl)
	c.queueStore = qs
	srv.queueStore = make(map[string]queue.Store)
	srv.queueStore[c.opts.ClientID] = qs
	pubrec := &packets.Pubrec{
		PacketID:   1,
		Code:       codes.UnspecifiedError,
		Properties: &packets.Properties{},
	}
	qs.EXPECT().Remove(pubrec.PacketID)
	c.pubrecHandler(pubrec)

	select {
	case p := <-c.out:
		t.Fatalf("unexpected output: %v", p)
	default:
	}
}

func TestClient_pubrecHandler(t *testing.T) {
	a := assert.New(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	srv := defaultServer()
	c, er := srv.newClient(noopConn{})
	a.Nil(er)
	c.opts.ClientID = "cid"
	c.version = packets.Version5
	c.opts.MaxInflight = 10
	c.newPacketIDLimiter(c.opts.MaxInflight)
	qs := queue.NewMockStore(ctrl)
	c.queueStore = qs
	srv.queueStore = make(map[string]queue.Store)
	srv.queueStore[c.opts.ClientID] = qs
	pubrec := &packets.Pubrec{
		PacketID:   1,
		Code:       codes.Success,
		Properties: &packets.Properties{},
	}
	qs.EXPECT().Replace(gomock.Any()).DoAndReturn(func(elem *queue.Elem) (bool, error) {
		a.Equal(pubrec.PacketID, elem.MessageWithID.(*queue.Pubrel).PacketID)
		return true, nil
	})
	c.pubrecHandler(pubrec)
	select {
	case p := <-c.out:
		a.IsType(&packets.Pubrel{}, p)
		a.Equal(pubrec.PacketID, p.(*packets.Pubrel).PacketID)
	default:
		t.Fatal("missing output")
	}
}

func TestClient_pubcompHandler(t *testing.T) {
	a := assert.New(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	srv := defaultServer()
	srv.statsManager = newStatsManager(mem.NewStore())
	c, er := srv.newClient(noopConn{})
	a.Nil(er)
	c.opts.ClientID = "cid"
	c.version = packets.Version5
	c.opts.MaxInflight = 10
	c.newPacketIDLimiter(c.opts.MaxInflight)
	qs := queue.NewMockStore(ctrl)
	c.queueStore = qs
	srv.queueStore = make(map[string]queue.Store)
	srv.queueStore[c.opts.ClientID] = qs
	pubcomp := &packets.Pubcomp{
		PacketID:   1,
		Code:       codes.Success,
		Properties: &packets.Properties{},
	}
	qs.EXPECT().Remove(pubcomp.PacketID)
	c.pubcompHandler(pubcomp)
}

func TestClient_pingreqHandler(t *testing.T) {
	a := assert.New(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	srv := defaultServer()
	c, er := srv.newClient(noopConn{})
	a.Nil(er)
	c.opts.ClientID = "cid"
	c.version = packets.Version5
	pingreq := &packets.Pingreq{}
	c.pingreqHandler(pingreq)
	select {
	case p := <-c.out:
		a.IsType(&packets.Pingresp{}, p)
	default:
		t.Fatal("missing output")
	}
}

func TestClient_unsubscribeHandler(t *testing.T) {
	a := assert.New(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	subDB := subscription.NewMockStore(ctrl)
	srv := defaultServer()
	srv.subscriptionsDB = subDB
	c, er := srv.newClient(noopConn{})
	a.Nil(er)
	c.opts.ClientID = "cid"
	c.version = packets.Version5

	unsub := &packets.Unsubscribe{
		Version:    packets.Version5,
		PacketID:   1,
		Topics:     []string{"/topic/A", "/topic/B"},
		Properties: nil,
	}

	for _, topic := range unsub.Topics {
		subDB.EXPECT().Unsubscribe(c.opts.ClientID, topic)
	}

	c.unsubscribeHandler(unsub)
	select {
	case p := <-c.out:
		unSuback := p.(*packets.Unsuback)
		a.EqualValues(0, unSuback.Payload[0])
		a.EqualValues(0, unSuback.Payload[1])
		a.Equal(unsub.PacketID, unSuback.PacketID)
	default:
		t.Fatal("missing output")
	}
}

func TestMsg_TotalBytes(t *testing.T) {
	var tt = []struct {
		name string
		pub  *packets.Publish
	}{
		{
			name: "version5/1header",
			pub: &packets.Publish{
				Version:    packets.Version5,
				TopicName:  []byte("a"),
				Payload:    []byte("a"),
				Properties: &packets.Properties{},
			},
		},
		{
			name: "version5/2header",
			pub: &packets.Publish{
				Version:    packets.Version5,
				TopicName:  []byte("a"),
				PacketID:   10,
				Qos:        packets.Qos1,
				Payload:    make([]byte, 127),
				Properties: &packets.Properties{},
			},
		},
		{
			name: "version5/3header",
			pub: &packets.Publish{
				Version:    packets.Version5,
				TopicName:  []byte("a"),
				PacketID:   10,
				Qos:        packets.Qos2,
				Payload:    make([]byte, 16383),
				Properties: &packets.Properties{},
			},
		},
		{
			name: "version5/4header",
			pub: &packets.Publish{
				Version:    packets.Version5,
				TopicName:  []byte("a"),
				PacketID:   10,
				Payload:    make([]byte, 2097151),
				Properties: &packets.Properties{},
			},
		},
		{
			name: "version5/1propertyLen",
			pub: &packets.Publish{
				Version:    packets.Version5,
				TopicName:  []byte("a"),
				Payload:    []byte("a"),
				Properties: &packets.Properties{},
			},
		},
		{
			name: "version5/2propertyLen",
			pub: &packets.Publish{
				Version:   packets.Version5,
				TopicName: []byte("a"),
				Payload:   []byte("a"),
				Properties: &packets.Properties{
					CorrelationData: make([]byte, 127),
				},
			},
		},
		{
			name: "version5/3propertyLen",
			pub: &packets.Publish{
				Version:   packets.Version5,
				TopicName: []byte("a"),
				Payload:   []byte("a"),
				Properties: &packets.Properties{
					CorrelationData: make([]byte, 16383),
				},
			},
		},
		{
			name: "version5/4propertyLen",
			pub: &packets.Publish{
				Version:   packets.Version5,
				TopicName: []byte("a"),
				Payload:   []byte("a"),
				Properties: &packets.Properties{
					CorrelationData: make([]byte, 2097151),
				},
			},
		},
	}
	for _, v := range tt {
		t.Run(v.name, func(t *testing.T) {
			a := assert.New(t)
			buf := make([]byte, 0, 2048)
			b := bytes.NewBuffer(buf)
			err := v.pub.Pack(b)
			a.Nil(err)
			var msg *gmqtt.Message
			msg = gmqtt.MessageFromPublish(v.pub)
			a.EqualValues(len(b.Bytes()), msg.TotalBytes(v.pub.Version))
		})
	}

}
