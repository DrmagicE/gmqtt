package fifo

import (
	"strconv"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/DrmagicE/gmqtt/server"
)

func TestQueue(t *testing.T) {
	a := assert.New(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	q := New()
	cid := "clientID"
	max := uint16(10)
	mockClient := server.NewMockClient(ctrl)
	mockClient.EXPECT().ClientOptions().Return(&server.ClientOptions{
		ClientID:            cid,
		ClientTopicAliasMax: max,
	}).AnyTimes()
	q.Create(mockClient)
	for i := uint16(1); i <= max; i++ {
		alias, ok := q.Check(mockClient, &packets.Publish{
			TopicName: []byte(strconv.Itoa(int(i))),
		})
		a.Equal(i, alias)
		a.False(ok)
	}
	alias := uint16(1)
	for e := q.topicAlias[cid].alias.Front(); e != nil; e = e.Next() {
		elem := e.Value.(*aliasElem)
		a.Equal(alias, elem.alias)
		a.Equal(strconv.Itoa(int(alias)), elem.topic)
		alias++
	}
	a.Equal(10, q.topicAlias[cid].alias.Len())

	// alias exist
	alias, ok := q.Check(mockClient, &packets.Publish{TopicName: []byte("1")})
	a.True(ok)
	a.EqualValues(1, alias)

	alias, ok = q.Check(mockClient, &packets.Publish{TopicName: []byte("not exist")})
	a.False(ok)
	a.EqualValues(1, alias)

	q.Delete(mockClient)

	a.Nil(q.topicAlias[cid])

}
