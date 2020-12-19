package admin

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/persistence/subscription"
	"github.com/DrmagicE/gmqtt/pkg/packets"
)

type subscriptionService struct {
	a *Admin
}

func (s *subscriptionService) mustEmbedUnimplementedSubscriptionServiceServer() {
	return
}

// List lists subscriptions in the broker.
func (s *subscriptionService) List(ctx context.Context, req *ListSubscriptionRequest) (*ListSubscriptionResponse, error) {
	page, pageSize := GetPage(req.Page, req.PageSize)
	subs, total, err := s.a.store.GetSubscriptions(page, pageSize)
	if err != nil {
		return &ListSubscriptionResponse{}, err
	}
	return &ListSubscriptionResponse{
		Subscriptions: subs,
		TotalCount:    total,
	}, nil
}

// Filter filters subscriptions with the request params.
// Paging is not supported, and the results are not sorted in any way.
// Using huge req.Limit can impact performance.
func (s *subscriptionService) Filter(ctx context.Context, req *FilterSubscriptionRequest) (resp *FilterSubscriptionResponse, err error) {
	var iterType subscription.IterationType
	iterOpts := subscription.IterationOptions{
		ClientID:  req.ClientId,
		TopicName: req.TopicName,
	}
	if req.FilterType == "" {
		iterType = subscription.TypeAll
	} else {
		types := strings.Split(req.FilterType, ",")
		for _, v := range types {
			if v == "" {
				continue
			}
			i, err := strconv.Atoi(v)

			if err != nil {
				return nil, ErrInvalidArgument("filter_type", err.Error())
			}
			switch SubFilterType(i) {

			case SubFilterType_SUB_FILTER_TYPE_SYS:
				iterType |= subscription.TypeSYS
			case SubFilterType_SUB_FILTER_TYPE_SHARED:
				iterType |= subscription.TypeShared
			case SubFilterType_SUB_FILTER_TYPE_NON_SHARED:
				iterType |= subscription.TypeNonShared
			default:
				return nil, ErrInvalidArgument("filter_type", "")
			}
		}
	}

	iterOpts.Type = iterType

	if req.MatchType == SubMatchType_SUB_MATCH_TYPE_MATCH_NAME {
		iterOpts.MatchType = subscription.MatchName
	} else if req.MatchType == SubMatchType_SUB_MATCH_TYPE_MATCH_FILTER {
		iterOpts.MatchType = subscription.MatchFilter
	}
	if iterOpts.TopicName == "" && iterOpts.MatchType != 0 {
		return nil, ErrInvalidArgument("topic_name", "cannot be empty while match_type Set")
	}
	if iterOpts.TopicName != "" && iterOpts.MatchType == 0 {
		return nil, ErrInvalidArgument("match_type", "cannot be empty while topic_name Set")
	}
	if iterOpts.TopicName != "" {
		if !packets.ValidV5Topic([]byte(iterOpts.TopicName)) {
			return nil, ErrInvalidArgument("topic_name", "")
		}
	}

	if req.Limit > 1000 {
		return nil, ErrInvalidArgument("limit", fmt.Sprintf("limit too large, must <= 1000"))
	}
	if req.Limit == 0 {
		req.Limit = 20
	}
	resp = &FilterSubscriptionResponse{
		Subscriptions: make([]*Subscription, 0),
	}
	i := int32(0)
	s.a.store.subscriptionService.Iterate(func(clientID string, sub *gmqtt.Subscription) bool {
		if i != req.Limit {
			resp.Subscriptions = append(resp.Subscriptions, &Subscription{
				TopicName:         subscription.GetFullTopicName(sub.ShareName, sub.TopicFilter),
				Id:                sub.ID,
				Qos:               uint32(sub.QoS),
				NoLocal:           sub.NoLocal,
				RetainAsPublished: sub.RetainAsPublished,
				RetainHandling:    uint32(sub.RetainHandling),
				ClientId:          clientID,
			})
		}
		i++
		return true
	}, iterOpts)

	return resp, nil
}

// Subscribe makes subscriptions for the given client.
func (s *subscriptionService) Subscribe(ctx context.Context, req *SubscribeRequest) (resp *SubscribeResponse, err error) {
	if req.ClientId == "" {
		return nil, ErrInvalidArgument("client_id", "cannot be empty")
	}
	if len(req.Subscriptions) == 0 {
		return nil, ErrInvalidArgument("subIndexer", "zero length subIndexer")
	}
	var subs []*gmqtt.Subscription
	for k, v := range req.Subscriptions {
		shareName, name := subscription.SplitTopic(v.TopicName)
		sub := &gmqtt.Subscription{
			ShareName:         shareName,
			TopicFilter:       name,
			ID:                v.Id,
			QoS:               uint8(v.Qos),
			NoLocal:           v.NoLocal,
			RetainAsPublished: v.RetainAsPublished,
			RetainHandling:    byte(v.RetainHandling),
		}
		err := sub.Validate()
		if err != nil {
			return nil, ErrInvalidArgument(fmt.Sprintf("subIndexer[%d]", k), err.Error())
		}
		subs = append(subs, sub)
	}
	rs, err := s.a.store.subscriptionService.Subscribe(req.ClientId, subs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to subscribe: %s", err.Error())
	}
	resp = &SubscribeResponse{
		New: make([]bool, 0),
	}
	for _, v := range rs {
		resp.New = append(resp.New, !v.AlreadyExisted)
	}
	return resp, nil

}

// Unsubscribe unsubscribe topic for the given client.
func (s *subscriptionService) Unsubscribe(ctx context.Context, req *UnsubscribeRequest) (resp *empty.Empty, err error) {
	if req.ClientId == "" {
		return nil, ErrInvalidArgument("client_id", "cannot be empty")
	}
	if len(req.Topics) == 0 {
		return nil, ErrInvalidArgument("topics", "zero length topics")
	}

	for k, v := range req.Topics {
		if !packets.ValidV5Topic([]byte(v)) {
			return nil, ErrInvalidArgument(fmt.Sprintf("topics[%d]", k), "")
		}
	}
	err = s.a.store.subscriptionService.Unsubscribe(req.ClientId, req.Topics...)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to unsubscribe: %s", err.Error()))
	}
	return &empty.Empty{}, nil
}
