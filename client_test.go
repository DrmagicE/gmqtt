package gmqtt

import (
	"container/list"
	"io"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/DrmagicE/gmqtt/pkg/codes"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/DrmagicE/gmqtt/retained"
	"github.com/DrmagicE/gmqtt/subscription"
)

func uint32P(v uint32) *uint32 {
	return &v
}
func uint16P(v uint16) *uint16 {
	return &v
}
func byteP(v byte) *byte {
	return &v
}

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
			for _, topic := range v.in.Topics {
				var options []subscription.SubOptions
				options = append(options,
					subscription.NoLocal(topic.NoLocal),
					subscription.RetainAsPublished(topic.RetainAsPublished),
					subscription.RetainHandling(topic.RetainHandling))
				if v.in.Properties != nil {
					for _, id := range v.in.Properties.SubscriptionIdentifier {
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
			for _, topic := range v.in.Topics {
				var options []subscription.SubOptions
				options = append(options,
					subscription.NoLocal(topic.NoLocal),
					subscription.RetainAsPublished(topic.RetainAsPublished),
					subscription.RetainHandling(topic.RetainHandling))
				if v.in.Properties != nil {
					for _, id := range v.in.Properties.SubscriptionIdentifier {
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
			for _, topic := range v.in.Topics {
				var options []subscription.SubOptions
				options = append(options,
					subscription.NoLocal(topic.NoLocal),
					subscription.RetainAsPublished(topic.RetainAsPublished),
					subscription.RetainHandling(topic.RetainHandling))
				if v.in.Properties != nil {
					for _, id := range v.in.Properties.SubscriptionIdentifier {
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
			if v.shouldSendRetained {
				a.True(publishHandlerCalled)
			} else {
				a.False(publishHandlerCalled)
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
			srv := &server{
				config:     DefaultConfig,
				retainedDB: retainedDB,
			}

			var deliverMessageCalled bool

			srv.deliverMessageHandler = func(srcClientID string, totalBytes uint32, msg packets.Message) (matched bool) {
				a.Equal(v.clientID, srcClientID)
				a.Equal(messageFromPublish(v.in), msg)
				deliverMessageCalled = true
				return v.topicMatched
			}

			c := srv.newClient(noopConn{})
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
					a.Equal(v.in.NewPuback(codes.Success), p)
				case packets.Qos2:
					a.Equal(v.in.NewPubrec(codes.Success), p)
					a.True(c.session.unackpublish[v.in.PacketID])
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
				config:     DefaultConfig,
				retainedDB: retainedDB,
			}
			srv.deliverMessageHandler = func(srcClientID string, totalBytes uint32, msg packets.Message) (matched bool) {
				a.Equal(v.clientID, srcClientID)
				a.Equal(messageFromPublish(v.in), msg)
				return v.topicMatched
			}
			c := srv.newClient(noopConn{})
			c.opts.ClientID = v.clientID
			c.version = v.version
			c.opts.RetainAvailable = v.retainedAvailable

			if v.retainedAvailable {
				if len(v.in.Payload) == 0 {
					retainedDB.EXPECT().Remove(string(v.in.TopicName))
				} else {
					retainedDB.EXPECT().AddOrReplace(messageFromPublish(v.in))
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
					a.Equal(v.in.NewPuback(codes.Success), p)
				case packets.Qos2:
					a.Equal(v.in.NewPubrec(codes.Success), p)
					a.True(c.session.unackpublish[v.in.PacketID])
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
				config: DefaultConfig,
			}
			srv.deliverMessageHandler = func(srcClientID string, totalBytes uint32, msg packets.Message) (matched bool) {
				a.Equal(v.clientID, srcClientID)
				a.Equal(messageFromPublish(v.in), msg)
				return true
			}
			c := srv.newClient(noopConn{})

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
		config: DefaultConfig,
	}

	var deliveredMsg []packets.Message
	srv.deliverMessageHandler = func(srcClientID string, totalBytes uint32, msg packets.Message) (matched bool) {
		a.Equal("cid", srcClientID)
		deliveredMsg = append(deliveredMsg, msg)
		return true
	}
	serverTopicAliasMax := uint16(5)
	c := srv.newClient(noopConn{})
	c.aliasMapper.server = make([][]byte, serverTopicAliasMax+1)
	c.opts.ClientID = "cid"
	c.version = packets.Version5
	c.opts.ServerTopicAliasMax = serverTopicAliasMax

	err := c.publishHandler(first)
	a.Nil(err)

	err = c.publishHandler(second)
	a.Nil(err)

	a.Len(deliveredMsg, 2)
	a.Equal(messageFromPublish(delivered), deliveredMsg[0])
	a.Equal(messageFromPublish(delivered), deliveredMsg[1])
}

func TestClient_pubackHandler(t *testing.T) {
	var tt = []struct {
		name             string
		clientID         string
		version          packets.Version
		in               *packets.Puback
		clientReceiveMax uint16
	}{
		{
			name:     "v5",
			clientID: "cid",
			version:  packets.Version5,
			in: &packets.Puback{
				Version:    packets.Version5,
				PacketID:   1,
				Code:       codes.Success,
				Properties: &packets.Properties{},
			},
			clientReceiveMax: 10,
		},
		{
			name:     "v311",
			clientID: "cid",
			version:  packets.Version311,
			in: &packets.Puback{
				Version:    packets.Version311,
				PacketID:   1,
				Code:       codes.Success,
				Properties: nil,
			},
		},
	}
	for _, v := range tt {
		t.Run(v.name, func(t *testing.T) {
			a := assert.New(t)
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			NewServer()
			srv := defaultServer()
			c := srv.newClient(noopConn{})
			c.opts.ClientID = v.clientID
			c.version = v.version
			if v.version == packets.Version5 {
				c.opts.ClientReceiveMax = v.clientReceiveMax
				c.clientReceiveMaximumQuota = 1
			}
			c.setInflight(&packets.Publish{
				Version:    v.version,
				Dup:        false,
				Qos:        1,
				Retain:     false,
				TopicName:  []byte("/topic/A"),
				PacketID:   1,
				Payload:    []byte("b"),
				Properties: nil,
			})
			a.Equal(1, c.session.inflight.Len())
			c.pubackHandler(v.in)
			if v.version == packets.Version5 {
				a.EqualValues(1, c.clientReceiveMaximumQuota)
			}
			a.Equal(0, c.session.inflight.Len())
		})
	}

}

func TestClient_pubrelHandler(t *testing.T) {
	a := assert.New(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	NewServer()
	srv := defaultServer()
	c := srv.newClient(noopConn{})
	c.opts.ClientID = "cid"
	c.version = packets.Version5
	pubrel := &packets.Pubrel{
		PacketID:   1,
		Code:       codes.Success,
		Properties: &packets.Properties{},
	}
	c.pubrelHandler(pubrel)

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
	NewServer()
	srv := defaultServer()
	c := srv.newClient(noopConn{})
	c.opts.ClientID = "cid"
	c.version = packets.Version5
	c.opts.ClientReceiveMax = 10
	c.clientReceiveMaximumQuota = 2
	pubrec := &packets.Pubrec{
		PacketID:   1,
		Code:       codes.UnspecifiedError,
		Properties: &packets.Properties{},
	}
	c.pubrecHandler(pubrec)
	a.EqualValues(3, c.clientReceiveMaximumQuota)
	a.Equal(0, c.session.awaitRel.Len())
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
	NewServer()
	srv := defaultServer()
	c := srv.newClient(noopConn{})
	c.opts.ClientID = "cid"
	c.version = packets.Version5
	c.opts.ClientReceiveMax = 10
	c.clientReceiveMaximumQuota = 2
	pubrec := &packets.Pubrec{
		PacketID:   1,
		Code:       codes.Success,
		Properties: &packets.Properties{},
	}
	c.pubrecHandler(pubrec)
	a.EqualValues(2, c.clientReceiveMaximumQuota)

	a.Equal(1, c.session.awaitRel.Len())
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
	NewServer()
	srv := defaultServer()
	c := srv.newClient(noopConn{})
	c.opts.ClientID = "cid"
	c.version = packets.Version5
	c.opts.ClientReceiveMax = 10
	c.clientReceiveMaximumQuota = 2
	pubcomp := &packets.Pubcomp{
		PacketID:   1,
		Code:       codes.Success,
		Properties: &packets.Properties{},
	}
	c.pubcompHandler(pubcomp)
	a.EqualValues(3, c.clientReceiveMaximumQuota)
	select {
	case p := <-c.out:
		a.IsType(&packets.Pubcomp{}, p)
		a.Equal(pubcomp.PacketID, p.(*packets.Pubrel).PacketID)
	default:
		t.Fatal("missing output")
	}
}

func TestClient_pingreqHandler(t *testing.T) {
	a := assert.New(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	NewServer()
	srv := defaultServer()
	c := srv.newClient(noopConn{})
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
	c := srv.newClient(noopConn{})
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
}
