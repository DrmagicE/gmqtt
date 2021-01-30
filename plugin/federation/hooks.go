package federation

import (
	"context"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/persistence/subscription"
	"github.com/DrmagicE/gmqtt/server"
)

func (f *Federation) HookWrapper() server.HookWrapper {
	return server.HookWrapper{
		OnSubscribedWrapper:        f.OnSubscribedWrapper,
		OnUnsubscribedWrapper:      f.OnUnsubscribedWrapper,
		OnMsgArrivedWrapper:        f.OnMsgArrivedWrapper,
		OnSessionTerminatedWrapper: f.OnSessionTerminatedWrapper,
	}
}

func (f *Federation) OnSubscribedWrapper(pre server.OnSubscribed) server.OnSubscribed {
	return func(ctx context.Context, client server.Client, subscription *gmqtt.Subscription) {
		pre(ctx, client, subscription)
		if subscription != nil {
			if !f.localSubStore.subscribe(client.ClientOptions().ClientID, subscription.GetFullTopicName()) {
				return
			}
			// only send new subscription
			f.memberMu.Lock()
			defer f.memberMu.Unlock()
			for _, v := range f.peers {
				sub := &Subscribe{
					ShareName:   subscription.ShareName,
					TopicFilter: subscription.TopicFilter,
				}
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
		if !f.localSubStore.unsubscribe(client.ClientOptions().ClientID, topicName) {
			return
		}
		// only unsubscribe topic if there is no local subscriber anymore.
		f.memberMu.Lock()
		defer f.memberMu.Unlock()
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
			f.memberMu.Lock()
			defer f.memberMu.Unlock()
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
			// TODO for shared subscription, we should either only send the message to local subscriber or only send the message to one node.
			sent := make(map[string]struct{})
			f.feSubStore.Iterate(func(nodeName string, sub *gmqtt.Subscription) bool {
				if _, ok := sent[nodeName]; ok {
					return true
				}
				if p, ok := f.peers[nodeName]; ok {
					msg := messageToEvent(req.Message)
					p.queue.add(&Event{
						Event: &Event_Message{
							Message: msg,
						}})
					sent[nodeName] = struct{}{}
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

func (f *Federation) OnSessionTerminatedWrapper(pre server.OnSessionTerminated) server.OnSessionTerminated {
	return func(ctx context.Context, clientID string, reason server.SessionTerminatedReason) {
		pre(ctx, clientID, reason)
		if unsubs := f.localSubStore.unsubscribeAll(clientID); len(unsubs) != 0 {
			f.memberMu.Lock()
			defer f.memberMu.Unlock()
			for _, v := range f.peers {
				for _, topicName := range unsubs {
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
	}

}
