package fifo

import (
	"strconv"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/DrmagicE/gmqtt/config"
	"github.com/DrmagicE/gmqtt/pkg/packets"
)

func TestQueue(t *testing.T) {
	a := assert.New(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cid := "clientID"
	max := uint16(10)
	q := New(config.DefaultConfig(), max, cid).(*Queue)
	for i := uint16(1); i <= max; i++ {
		alias, ok := q.Check(&packets.Publish{
			TopicName: []byte(strconv.Itoa(int(i))),
		})
		a.Equal(i, alias)
		a.False(ok)
	}
	alias := uint16(1)
	for e := q.topicAlias.alias.Front(); e != nil; e = e.Next() {
		elem := e.Value.(*aliasElem)
		a.Equal(alias, elem.alias)
		a.Equal(strconv.Itoa(int(alias)), elem.topic)
		alias++
	}
	a.Equal(10, q.topicAlias.alias.Len())

	// alias exist
	alias, ok := q.Check(&packets.Publish{TopicName: []byte("1")})
	a.True(ok)
	a.EqualValues(1, alias)

	alias, ok = q.Check(&packets.Publish{TopicName: []byte("not exist")})
	a.False(ok)
	a.EqualValues(1, alias)

}
