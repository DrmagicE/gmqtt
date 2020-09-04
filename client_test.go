package gmqtt

import (
	"container/list"
	"io"
	"net"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/DrmagicE/gmqtt/pkg/codes"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/DrmagicE/gmqtt/retained"
	"github.com/DrmagicE/gmqtt/subscription"
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
		packet   *packets.Subscribe
		out      *packets.Suback
		err      *codes.Error
	}{
		{
			name:     "success_v5",
			clientID: "cid",
			version:  packets.Version5,
			packet: &packets.Subscribe{
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
				Payload: []codes.Code{
					codes.GrantedQoS1, codes.GrantedQoS2,
				},
			},
		},
		{
			name:     "success_v3",
			clientID: "cid",
			packet: &packets.Subscribe{
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
				Version:  packets.Version311,
				PacketID: 2,
				Payload: []codes.Code{
					codes.GrantedQoS1, codes.GrantedQoS2,
				},
			},
		},
		{
			name:     "subscription_identifier",
			clientID: "cid",
			version:  packets.Version5,
			packet: &packets.Subscribe{
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
				Version:  packets.Version5,
				PacketID: 1,
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
				config:          DefaultConfig,
				subscriptionsDB: subDB,
				retainedDB:      retainedDB,
			}
			c := srv.newClient(noopConn{})
			c.opts.ClientID = v.clientID
			c.opts.SubIDAvailable = true
			c.version = v.version
			for _, topic := range v.packet.Topics {
				var options []subscription.SubOptions
				options = append(options,
					subscription.NoLocal(topic.NoLocal),
					subscription.RetainAsPublished(topic.RetainAsPublished),
					subscription.RetainHandling(topic.RetainHandling))
				if v.packet.Properties != nil {
					for _, id := range v.packet.Properties.SubscriptionIdentifier {
						options = append(options, subscription.ID(id))
					}
				}
				sub := subscription.New(topic.Name, topic.Qos, options...)
				subDB.EXPECT().Subscribe(v.clientID, sub).Return(subscription.SubscribeResult{
					{
						Subscription:   sub,
						AlreadyExisted: false,
					},
				})
				// We are not going to test retained logic in this test case.
				retainedDB.EXPECT().GetMatchedMessages(sub.TopicFilter()).Return(nil)
			}

			err := c.subscribeHandler(v.packet)
			a.Equal(v.err, err)
			select {
			case p := <-c.out:
				suback := p.(*packets.Suback)
				a.Equal(v.packet.PacketID, suback.PacketID)
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
		packet             *packets.Subscribe
		out                *packets.Suback
		err                *codes.Error
		sharedSubAvailable bool
	}{
		{
			name:     "success_v5",
			clientID: "cid",
			version:  packets.Version5,
			packet: &packets.Subscribe{
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
			packet: &packets.Subscribe{
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
				config:          DefaultConfig,
				subscriptionsDB: subDB,
				retainedDB:      retainedDB,
			}
			c := srv.newClient(noopConn{})
			c.opts.ClientID = v.clientID
			c.opts.SubIDAvailable = v.sharedSubAvailable
			c.version = v.version
			for _, topic := range v.packet.Topics {
				var options []subscription.SubOptions
				options = append(options,
					subscription.NoLocal(topic.NoLocal),
					subscription.RetainAsPublished(topic.RetainAsPublished),
					subscription.RetainHandling(topic.RetainHandling))
				if v.packet.Properties != nil {
					for _, id := range v.packet.Properties.SubscriptionIdentifier {
						options = append(options, subscription.ID(id))
					}
				}
				sub := subscription.New(topic.Name, topic.Qos, options...)
				subDB.EXPECT().Subscribe(v.clientID, sub).Return(subscription.SubscribeResult{
					{
						Subscription:   sub,
						AlreadyExisted: false,
					},
				})
				// We are not going to test retained logic in this test case.
				retainedDB.EXPECT().GetMatchedMessages(sub.TopicFilter()).Return(nil)
			}

			err := c.subscribeHandler(v.packet)
			a.Equal(v.err, err)
			select {
			case p := <-c.out:
				suback := p.(*packets.Suback)
				a.Equal(v.packet.PacketID, suback.PacketID)
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
		packet            *packets.Subscribe
		out               *packets.Suback
		err               *codes.Error
		retainedMsg       packets.Message
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
			packet: &packets.Subscribe{
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
				Version: packets.Version5,
				Payload: []codes.Code{
					codes.GrantedQoS1,
				},
			},
			retainedMsg: NewMessage("/topic/A", []byte("b"), 1, Retained(true)),
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
			packet: &packets.Subscribe{
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
				Version: packets.Version5,
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
			packet: &packets.Subscribe{
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
				Version: packets.Version5,
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
			packet: &packets.Subscribe{
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
				Version: packets.Version5,
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
			packet: &packets.Subscribe{
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
				Version: packets.Version5,
				Payload: []codes.Code{
					codes.GrantedQoS2,
				},
			},
			retainedMsg: NewMessage("/topic/A", []byte("b"), 1, Retained(true)),
			expected: struct {
				qos      uint8
				retained bool
			}{qos: 1, retained: false},
			alreadyExisted:     false,
			shouldSendRetained: true,
		},
		{
			// If the value of Retain As Published subscription option is set to 1,
			// the Server MUST set the RETAIN flag equal to the RETAIN flag in the received PUBLISH packet [MQTT-3.3.1-13].
			name:     "rap_1",
			clientID: "cid",
			version:  packets.Version5,
			packet: &packets.Subscribe{
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
				Version: packets.Version5,
				Payload: []codes.Code{
					codes.GrantedQoS2,
				},
			},
			retainedMsg: NewMessage("/topic/A", []byte("b"), 1, Retained(true)),
			expected: struct {
				qos      uint8
				retained bool
			}{qos: 1, retained: true},
			alreadyExisted:     false,
			shouldSendRetained: true,
		},
		{
			// If the value of Retain As Published subscription option is set to 1,
			// the Server MUST set the RETAIN flag equal to the RETAIN flag in the received PUBLISH packet [MQTT-3.3.1-13].
			name:     "rap_1",
			clientID: "cid",
			version:  packets.Version5,
			packet: &packets.Subscribe{
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
				Version: packets.Version5,
				Payload: []codes.Code{
					codes.GrantedQoS2,
				},
			},
			retainedMsg: NewMessage("/topic/A", []byte("b"), 1, Retained(true)),
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
			srv := &server{
				config:          DefaultConfig,
				subscriptionsDB: subDB,
				retainedDB:      retainedDB,
			}
			c := srv.newClient(noopConn{})
			c.opts.ClientID = v.clientID

			var publishHandlerCalled bool
			c.publishMessageHandler = func(publish *packets.Publish) {
				if v.shouldSendRetained {
					a.Equal(v.expected.qos, publish.Qos)
					a.Equal(v.expected.retained, publish.Retain)
					publishHandlerCalled = true
				}
			}
			c.opts.RetainAvailable = v.retainedAvailable
			c.version = v.version
			for _, topic := range v.packet.Topics {
				var options []subscription.SubOptions
				options = append(options,
					subscription.NoLocal(topic.NoLocal),
					subscription.RetainAsPublished(topic.RetainAsPublished),
					subscription.RetainHandling(topic.RetainHandling))
				if v.packet.Properties != nil {
					for _, id := range v.packet.Properties.SubscriptionIdentifier {
						options = append(options, subscription.ID(id))
					}
				}
				sub := subscription.New(topic.Name, topic.Qos, options...)
				subDB.EXPECT().Subscribe(v.clientID, sub).Return(subscription.SubscribeResult{
					{
						Subscription:   sub,
						AlreadyExisted: v.alreadyExisted,
					},
				})
				if v.shouldSendRetained {
					retainedDB.EXPECT().GetMatchedMessages(sub.TopicFilter()).Return([]packets.Message{v.retainedMsg})
				}
			}

			err := c.subscribeHandler(v.packet)
			a.Equal(v.err, err)
			select {
			case p := <-c.out:
				suback := p.(*packets.Suback)
				a.Equal(v.packet.PacketID, suback.PacketID)
				a.Equal(v.out.Payload, suback.Payload)
				a.Equal(v.out.Version, suback.Version)
				a.Equal(v.out.Properties, suback.Properties)
			default:
				t.Fatal("missing output")
			}
			if v.shouldSendRetained {
				a.True(publishHandlerCalled)
			} else {
				a.False(publishHandlerCalled)
			}
		})
	}

}
