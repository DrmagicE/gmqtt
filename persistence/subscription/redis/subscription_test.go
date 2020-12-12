package redis

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DrmagicE/gmqtt"
)

func TestEncodeDecodeSubscription(t *testing.T) {
	a := assert.New(t)
	tt := []*gmqtt.Subscription{
		{
			ShareName:         "shareName",
			TopicFilter:       "filter",
			ID:                1,
			QoS:               1,
			NoLocal:           false,
			RetainAsPublished: false,
			RetainHandling:    0,
		}, {
			ShareName:         "",
			TopicFilter:       "abc",
			ID:                0,
			QoS:               2,
			NoLocal:           false,
			RetainAsPublished: true,
			RetainHandling:    1,
		},
	}

	for _, v := range tt {
		b := EncodeSubscription(v)
		sub, err := DecodeSubscription(b)
		a.Nil(err)
		a.Equal(v, sub)
	}
}
