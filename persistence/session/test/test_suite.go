package test

import (
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/persistence/session"
)

func TestSuite(t *testing.T, store session.Store) {
	a := assert.New(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	var tt = []*gmqtt.Session{
		{
			ClientID: "client",
			Will: &gmqtt.Message{
				Topic:   "topicA",
				Payload: []byte("abc"),
			},
			WillDelayInterval: 1,
			ConnectedAt:       time.Unix(1, 0),
			ExpiryInterval:    2,
		}, {
			ClientID:          "client2",
			Will:              nil,
			WillDelayInterval: 0,
			ConnectedAt:       time.Unix(2, 0),
			ExpiryInterval:    0,
		},
	}
	for _, v := range tt {
		a.Nil(store.Set(v))
	}
	for _, v := range tt {
		sess, err := store.Get(v.ClientID)
		a.Nil(err)
		a.EqualValues(v, sess)
	}
	var sess []*gmqtt.Session
	err := store.Iterate(func(session *gmqtt.Session) bool {
		sess = append(sess, session)
		return true
	})
	a.Nil(err)
	a.ElementsMatch(sess, tt)
}
