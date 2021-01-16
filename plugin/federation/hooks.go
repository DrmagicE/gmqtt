package federation

import (
	"context"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/persistence/subscription"
	"github.com/DrmagicE/gmqtt/server"
)

func (f *Federation) HookWrapper() server.HookWrapper {
	return server.HookWrapper{
		OnSubscribedWrapper:   f.OnSubscribedWrapper,
		OnUnsubscribedWrapper: f.OnUnsubscribedWrapper,
		OnMsgArrivedWrapper:   f.OnMsgArrivedWrapper,
	}
}

func (f *Federation) OnSubscribedWrapper(pre server.OnSubscribed) server.OnSubscribed {
	return func(ctx context.Context, client server.Client, subscription *gmqtt.Subscription) {
		pre(ctx, client, subscription)
		if subscription != nil {
			_, _ = f.localSubStore.Subscribe(f.nodeName, subscription)
			f.mu.Lock()
			defer f.mu.Unlock()
			for _, v := range f.peers {
				sub := subscriptionToEvent(subscription)
				v.queue.add(&Event{
					Event: &Event_Subscribe{
						Subscribe: sub,
					}})
			}
		}
	}
}

func (f *Federation) OnUnsubscribedWrapper(pre server.OnUnsubscribed) server.OnUnsubscribed {
	return func(ctx context.Context, client server.Client, topicName string) {
		pre(ctx, client, topicName)
		_ = f.localSubStore.Unsubscribe(f.nodeName, topicName)
		f.mu.Lock()
		defer f.mu.Unlock()
		for _, v := range f.peers {
			unsub := &Unsubscribe{
				TopicName: topicName,
			}
			v.queue.add(&Event{
				Event: &Event_Unsubscribe{
					Unsubscribe: unsub,
				}})
		}
	}
}

func (f *Federation) OnMsgArrivedWrapper(pre server.OnMsgArrived) server.OnMsgArrived {
	return func(ctx context.Context, client server.Client, req *server.MsgArrivedRequest) error {
		err := pre(ctx, client, req)
		if err != nil {
			return err
		}
		if req.Message != nil {
			f.mu.Lock()
			defer f.mu.Unlock()
			// If it is a retained message, broadcasts the message to all nodes to update their local retained store.
			if req.Message.Retained {
				msg := messageToEvent(req.Message)
				for _, v := range f.peers {
					v.queue.add(&Event{
						Event: &Event_Message{
							Message: msg,
						}})
				}
				return nil
			}
			// For not retained message , send it to the nodes which have matched topics.
			// TODO The delivery mode is Overlap, make it configurable.
			f.feSubStore.Iterate(func(clientID string, sub *gmqtt.Subscription) bool {
				if p, ok := f.peers[clientID]; ok {
					msg := messageToEvent(req.Message)
					p.queue.add(&Event{
						Event: &Event_Message{
							Message: msg,
						}})
				}
				return true
			}, subscription.IterationOptions{
				Type:      subscription.TypeAll,
				TopicName: req.Message.Topic,
				MatchType: subscription.MatchFilter,
			})

		}
		return nil
	}

}
