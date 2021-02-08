package federation

import (
	"context"
	"sort"

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
		OnWillPublishWrapper:       f.OnWillPublishWrapper,
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

func sendSharedMsg(fs *fedSubStore, sharedList map[string][]string, send func(nodeName string)) {
	// shared subscription
	fs.sharedMu.Lock()
	defer fs.sharedMu.Unlock()
	for topicName, v := range sharedList {
		sort.Strings(v)
		mod := fs.sharedSent[topicName] % (uint64(len(v)) + 1)
		fs.sharedSent[topicName]++
		// sends to local node, just ignores it
		if mod == 0 {
			continue
		}
		send(v[mod-1])
	}
}

func (f *Federation) sendMessage(msg *gmqtt.Message) bool {
	f.memberMu.Lock()
	defer f.memberMu.Unlock()
	// If it is a retained message, broadcasts the message to all nodes to update their local retained store.
	if msg.Retained {
		eventMsg := messageToEvent(msg)
		for _, v := range f.peers {
			v.queue.add(&Event{
				Event: &Event_Message{
					Message: eventMsg,
				}})
		}
		return true
	}
	// For none retained message , send it to the nodes which have matched topics.
	// For shared subscription, we should either only send the message to local subscriber or only send the message to one node.

	// shared topic => []nodeName.
	sharedList := make(map[string][]string)
	// store non-shared topic, key by nodeName
	nonShared := make(map[string]struct{})
	f.fedSubStore.Iterate(func(nodeName string, sub *gmqtt.Subscription) bool {
		if sub.ShareName != "" {
			fullTopic := sub.GetFullTopicName()
			sharedList[fullTopic] = append(sharedList[fullTopic], nodeName)
			return true
		}
		if _, ok := nonShared[nodeName]; ok {
			return true
		}
		nonShared[nodeName] = struct{}{}
		return true
	}, subscription.IterationOptions{
		Type:      subscription.TypeAll,
		TopicName: msg.Topic,
		MatchType: subscription.MatchFilter,
	})
	// shared subscription
	sharedSent := make(map[string]struct{})
	sendSharedMsg(f.fedSubStore, sharedList, func(nodeName string) {
		if _, ok := sharedSent[nodeName]; ok {
			return
		}
		sharedSent[nodeName] = struct{}{}
		if p, ok := f.peers[nodeName]; ok {
			eventMsg := messageToEvent(msg)
			p.queue.add(&Event{
				Event: &Event_Message{
					Message: eventMsg,
				}})
			// If the message is sent because of matching a shared subscription,
			// it should not be sent again if it also matches a non-shared one.
			delete(nonShared, nodeName)
		}
	})

	// non-shared subscription
	for nodeName := range nonShared {
		if p, ok := f.peers[nodeName]; ok {
			eventMsg := messageToEvent(msg)
			p.queue.add(&Event{
				Event: &Event_Message{
					Message: eventMsg,
				}})
		}
	}
	return true
}
func (f *Federation) OnMsgArrivedWrapper(pre server.OnMsgArrived) server.OnMsgArrived {
	return func(ctx context.Context, client server.Client, req *server.MsgArrivedRequest) error {
		err := pre(ctx, client, req)
		if err != nil {
			return err
		}
		if req.Message != nil {
			f.sendMessage(req.Message)
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

func (f *Federation) OnWillPublishWrapper(pre server.OnWillPublish) server.OnWillPublish {
	return func(ctx context.Context, clientID string, req *server.WillMsgRequest) {
		pre(ctx, clientID, req)
		if req.Message != nil {
			f.sendMessage(req.Message)
		}
	}
}
