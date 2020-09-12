package trie

import (
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/DrmagicE/gmqtt/subscription"
)

var r *rand.Rand

func init() {
	r = rand.New(rand.NewSource(time.Now().Unix()))
}

// RandString 生成随机字符串
func RandString(len int) string {
	bytes := make([]byte, len)
	for i := 0; i < len; i++ {
		b := r.Intn(26) + 65
		bytes[i] = byte(b)
	}
	return string(bytes)
}

func sharedSub() []subscription.Subscription {
	var subs []subscription.Subscription
	for i := 0; i < 1000; i++ {
		subs = append(subs, subscription.New(RandString(10)+"/"+RandString(10), 1, subscription.ShareName(RandString(10))))
	}
	return subs
}

func nonShare() []subscription.Subscription {
	var subs []subscription.Subscription
	for i := 0; i < 1000; i++ {
		subs = append(subs, subscription.New(RandString(10)+"/"+RandString(10), 1))
	}
	return subs
}

func systemTopic() []subscription.Subscription {
	var subs []subscription.Subscription
	for i := 0; i < 1000; i++ {
		subs = append(subs, subscription.New("$"+RandString(10)+"/"+RandString(10), 1))
	}
	return subs
}

func BenchmarkTrieDB_Subscribeb(b *testing.B) {
	s := NewStore()

	shared := sharedSub()
	nonShare := nonShare()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		//s.Subscribe("1", shared...)
		//s.Subscribe("1", nonShare...)
		for _, v := range shared {
			s.Subscribe("1", v)
		}
		for _, v := range nonShare {
			s.Subscribe("1", v)
		}
	}
}

func BenchmarkTrieDBIteration(b *testing.B) {
	s := NewStore()

	s.Subscribe("1", subscription.New("topicA/A", 1))
	s.Subscribe("1", subscription.New("topicA/+", 2))
	s.Subscribe("1", subscription.New("+/A", 1))
	s.Subscribe("1", subscription.New("topicB/A", 1))
	s.Subscribe("1", subscription.New("topicC/A", 1))
	s.Subscribe("1", subscription.New("topicA/A", 1, subscription.ShareName("sharename")))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Iterate(func(clientID string, sub subscription.Subscription) bool {
			return true
		}, subscription.IterationOptions{
			Type:      subscription.TypeAll,
			MatchType: subscription.MatchFilter,
			TopicName: "topicA/A",
		})
	}
}

func BenchmarkABC(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = subscription.New("topicFilter", 1)
	}
}

func BenchmarkABCA(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = &sub{
			topicFilter: "topicFilter",
			qos:         1,
		}
	}
}

func BenchmarkABCAd(b *testing.B) {
	sub := subscription.New("topicFilter", 1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sub.TopicFilter()
	}
}
func BenchmarkABCAf(b *testing.B) {
	sub := &sub{
		topicFilter: "topicFilter",
		qos:         1,
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sub.topicFilter
	}
}

func BenchmarkContain(b *testing.B) {
	s := "abcdef,g+,w,a,d,d,g+"
	for i := 0; i < b.N; i++ {
		for _, c := range s {
			if c == '+' || c == '#' {
				break
			}
		}
	}
}
func BenchmarkContainAny(b *testing.B) {
	s := "abcdef,g+,w,a,d,d,g+"
	for i := 0; i < b.N; i++ {
		strings.ContainsAny(s, "+#")
	}
}
