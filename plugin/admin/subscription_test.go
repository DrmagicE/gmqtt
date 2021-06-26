package admin

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/persistence/subscription"
	"github.com/DrmagicE/gmqtt/server"
)

func TestSubscriptionService_List(t *testing.T) {

	a := assert.New(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ss := server.NewMockSubscriptionService(ctrl)
	client := server.NewMockClient(ctrl)
	admin := &Admin{
		store: newStore(nil, mockConfig),
	}
	sub := &subscriptionService{
		a: admin,
	}
	client.EXPECT().ClientOptions().Return(&server.ClientOptions{ClientID: "id"})
	subscribe := admin.OnSubscribedWrapper(func(ctx context.Context, client server.Client, subscription *gmqtt.Subscription) {})

	subsc := &gmqtt.Subscription{
		ShareName:         "abc",
		TopicFilter:       "t",
		ID:                1,
		QoS:               2,
		NoLocal:           true,
		RetainAsPublished: true,
		RetainHandling:    2,
	}
	subscribe(context.Background(), client, subsc)
	sub.a.store.subscriptionService = ss

	resp, err := sub.List(context.Background(), &ListSubscriptionRequest{
		PageSize: 0,
		Page:     0,
	})

	a.Nil(err)
	a.Len(resp.Subscriptions, 1)
	rs := resp.Subscriptions[0]
	a.EqualValues(subsc.QoS, rs.Qos)
	a.EqualValues(subsc.GetFullTopicName(), rs.TopicName)
	a.EqualValues(subsc.ID, rs.Id)
	a.EqualValues(subsc.RetainHandling, rs.RetainHandling)
	a.EqualValues(subsc.NoLocal, rs.NoLocal)
}

func TestSubscriptionService_Filter(t *testing.T) {
	a := assert.New(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ss := server.NewMockSubscriptionService(ctrl)
	admin := &Admin{
		store: newStore(nil, mockConfig),
	}
	sub := &subscriptionService{
		a: admin,
	}
	sub.a.store.subscriptionService = ss

	ss.EXPECT().Iterate(gomock.Any(), subscription.IterationOptions{
		Type:      subscription.TypeAll,
		ClientID:  "cid",
		TopicName: "abc",
		MatchType: subscription.MatchName,
	})

	_, err := sub.Filter(context.Background(), &FilterSubscriptionRequest{
		ClientId:   "cid",
		FilterType: "1,2,3",
		MatchType:  SubMatchType_SUB_MATCH_TYPE_MATCH_NAME,
		TopicName:  "abc",
		Limit:      1,
	})
	a.Nil(err)

	ss.EXPECT().Iterate(gomock.Any(), subscription.IterationOptions{
		Type:      subscription.TypeAll,
		ClientID:  "cid",
		TopicName: "abc",
		MatchType: subscription.MatchName,
	})

	// test default filter type
	_, err = sub.Filter(context.Background(), &FilterSubscriptionRequest{
		ClientId:   "cid",
		FilterType: "",
		MatchType:  SubMatchType_SUB_MATCH_TYPE_MATCH_NAME,
		TopicName:  "abc",
		Limit:      1,
	})
	a.Nil(err)

	ss.EXPECT().Iterate(gomock.Any(), subscription.IterationOptions{
		Type:      subscription.TypeNonShared | subscription.TypeSYS,
		ClientID:  "cid",
		TopicName: "abc",
		MatchType: subscription.MatchName,
	})

	_, err = sub.Filter(context.Background(), &FilterSubscriptionRequest{
		ClientId:   "cid",
		FilterType: "1,3",
		MatchType:  SubMatchType_SUB_MATCH_TYPE_MATCH_NAME,
		TopicName:  "abc",
		Limit:      1,
	})
	a.Nil(err)

	ss.EXPECT().Iterate(gomock.Any(), subscription.IterationOptions{
		Type:      subscription.TypeNonShared | subscription.TypeSYS,
		ClientID:  "cid",
		TopicName: "abc",
		MatchType: subscription.MatchFilter,
	})

	_, err = sub.Filter(context.Background(), &FilterSubscriptionRequest{
		ClientId:   "cid",
		FilterType: "1,3",
		MatchType:  SubMatchType_SUB_MATCH_TYPE_MATCH_FILTER,
		TopicName:  "abc",
		Limit:      1,
	})
	a.Nil(err)

}

func TestSubscriptionService_Filter_InvalidArgument(t *testing.T) {
	var tt = []struct {
		name  string
		field string
		req   *FilterSubscriptionRequest
	}{
		{
			name:  "empty_topic_name_with_match_name",
			field: "match_type",
			req: &FilterSubscriptionRequest{
				ClientId:   "",
				FilterType: "",
				MatchType:  SubMatchType_SUB_MATCH_TYPE_MATCH_NAME,
				TopicName:  "",
				Limit:      1,
			},
		},
		{
			name:  "empty_topic_name_with_match_filter",
			field: "match_type",
			req: &FilterSubscriptionRequest{
				ClientId:   "",
				FilterType: "",
				MatchType:  SubMatchType_SUB_MATCH_TYPE_MATCH_FILTER,
				TopicName:  "",
				Limit:      1,
			},
		},
		{
			name:  "invalid_topic_name",
			field: "topic_name",
			req: &FilterSubscriptionRequest{
				ClientId:   "",
				FilterType: "",
				MatchType:  0,
				TopicName:  "##",
				Limit:      1,
			},
		},
	}
	for _, v := range tt {
		t.Run(v.name, func(t *testing.T) {
			a := assert.New(t)
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ss := server.NewMockSubscriptionService(ctrl)
			admin := &Admin{
				store: newStore(nil, mockConfig),
			}
			sub := &subscriptionService{
				a: admin,
			}
			sub.a.store.subscriptionService = ss

			_, err := sub.Filter(context.Background(), v.req)
			a.NotNil(err)
			a.Contains(err.Error(), v.field)

		})
	}

}

func TestSubscriptionService_Subscribe(t *testing.T) {
	a := assert.New(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ss := server.NewMockSubscriptionService(ctrl)
	admin := &Admin{
		store: newStore(nil, mockConfig),
	}
	sub := &subscriptionService{
		a: admin,
	}
	sub.a.store.subscriptionService = ss

	subs := []*Subscription{
		{
			TopicName:         "$share/a/b",
			Id:                1,
			Qos:               2,
			NoLocal:           true,
			RetainAsPublished: true,
			RetainHandling:    2,
		}, {
			TopicName:         "abc",
			Id:                1,
			Qos:               2,
			NoLocal:           true,
			RetainAsPublished: true,
			RetainHandling:    2,
		},
	}
	var expectedSubs []*gmqtt.Subscription
	for _, v := range subs {
		shareName, filter := subscription.SplitTopic(v.TopicName)
		s := &gmqtt.Subscription{
			ShareName:         shareName,
			TopicFilter:       filter,
			ID:                v.Id,
			QoS:               byte(v.Qos),
			NoLocal:           v.NoLocal,
			RetainAsPublished: v.RetainAsPublished,
			RetainHandling:    byte(v.RetainHandling),
		}
		expectedSubs = append(expectedSubs, s)
	}

	ss.EXPECT().Subscribe("cid", expectedSubs).Return(subscription.SubscribeResult{
		{
			AlreadyExisted: true,
		}, {
			AlreadyExisted: false,
		},
	}, nil)

	resp, err := sub.Subscribe(context.Background(), &SubscribeRequest{
		ClientId:      "cid",
		Subscriptions: subs,
	})
	a.Nil(err)
	resp.New = []bool{false, true}

}

func TestSubscriptionService_Subscribe_InvalidArgument(t *testing.T) {
	var tt = []struct {
		name string
		req  *SubscribeRequest
	}{
		{
			name: "empty_client_id",
			req: &SubscribeRequest{
				ClientId:      "",
				Subscriptions: nil,
			},
		},
		{
			name: "empty_subscriptions",
			req: &SubscribeRequest{
				ClientId: "cid",
			},
		},
		{
			name: "invalid_subscriptions",
			req: &SubscribeRequest{
				ClientId: "cid",
				Subscriptions: []*Subscription{
					{
						TopicName:         "##",
						Id:                0,
						Qos:               0,
						NoLocal:           false,
						RetainAsPublished: false,
						RetainHandling:    0,
					},
				},
			},
		},
	}
	for _, v := range tt {
		t.Run(v.name, func(t *testing.T) {
			a := assert.New(t)
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ss := server.NewMockSubscriptionService(ctrl)
			admin := &Admin{
				store: newStore(nil, mockConfig),
			}
			sub := &subscriptionService{
				a: admin,
			}
			sub.a.store.subscriptionService = ss

			_, err := sub.Subscribe(context.Background(), v.req)
			a.NotNil(err)
		})
	}

}

func TestSubscriptionService_Unsubscribe(t *testing.T) {
	a := assert.New(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ss := server.NewMockSubscriptionService(ctrl)
	admin := &Admin{
		store: newStore(nil, mockConfig),
	}
	sub := &subscriptionService{
		a: admin,
	}
	sub.a.store.subscriptionService = ss

	topics := []string{
		"a", "b",
	}
	ss.EXPECT().Unsubscribe("cid", topics)
	_, err := sub.Unsubscribe(context.Background(), &UnsubscribeRequest{
		ClientId: "cid",
		Topics:   topics,
	})
	a.Nil(err)

}

func TestSubscriptionService_Unsubscribe_InvalidArgument(t *testing.T) {
	var tt = []struct {
		name string
		req  *UnsubscribeRequest
	}{
		{
			name: "empty_client_id",
			req: &UnsubscribeRequest{
				ClientId: "",
				Topics:   nil,
			},
		},
		{
			name: "invalid_topic_name",
			req: &UnsubscribeRequest{
				ClientId: "cid",
				Topics:   []string{"+", "##"},
			},
		},
	}
	for _, v := range tt {
		t.Run(v.name, func(t *testing.T) {
			a := assert.New(t)
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ss := server.NewMockSubscriptionService(ctrl)
			admin := &Admin{
				store: newStore(nil, mockConfig),
			}
			sub := &subscriptionService{
				a: admin,
			}
			sub.a.store.subscriptionService = ss

			_, err := sub.Unsubscribe(context.Background(), v.req)
			a.NotNil(err)
		})
	}

}
