package retained

import (
	"github.com/DrmagicE/gmqtt/pkg/packets"
)

type IterateFn func(message packets.Message) bool

type Store interface {
	GetRetainedMessage(topicName string) packets.Message
	ClearAll()
	AddOrReplace(message packets.Message)
	Remove(topicName string)
	GetMatchedMessages(topicFilter string) []packets.Message
	Iterate(fn IterateFn)
}
