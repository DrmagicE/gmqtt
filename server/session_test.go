package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DrmagicE/gmqtt/pkg/packets"
)

const testMaxInflightLen = 20
const testMaxAwaitRelLen = 20
const testMaxMsgQueueLen = 20
const testV5ReceiveMaximumQuota = 20

//mock client,only for session_test.go
func mockClient() *client {
	srv := defaultServer()
	c := srv.newClient(noopConn{})
	c.config.MaxInflight = testMaxInflightLen
	c.config.MaxAwaitRel = testMaxAwaitRelLen
	c.version = packets.Version5
	c.clientReceiveMaximumQuota = testV5ReceiveMaximumQuota
	c.opts.ClientReceiveMax = testV5ReceiveMaximumQuota
	return c
}
func fullInflightSessionQos1() *client {
	c := mockClient()
	for i := 1; i <= testMaxInflightLen; i++ {
		pub := &packets.Publish{PacketID: packets.PacketID(i), Qos: packets.Qos1}
		c.setInflight(pub)
	}
	return c
}
func TestQos1Inflight(t *testing.T) {
	a := assert.New(t)
	c := mockClient()
	for i := 1; i <= testMaxInflightLen; i++ {
		quota := c.clientReceiveMaximumQuota
		pub := &packets.Publish{PacketID: packets.PacketID(i), Qos: packets.Qos1}
		a.True(c.setInflight(pub))
		a.EqualValues(quota-1, c.clientReceiveMaximumQuota)
	}
	a.EqualValues(0, c.clientReceiveMaximumQuota)
	// inflight is not full, msgQueue len should be 0
	a.Equal(0, c.session.msgQueue.Len())
	for i := 1; i <= testMaxInflightLen/2; i++ {
		puback := &packets.Puback{PacketID: packets.PacketID(i)}
		c.unsetInflight(puback)
		a.Equal(testMaxInflightLen-i, c.session.inflight.Len())
		a.Equal(0, c.session.msgQueue.Len())
	}
	//test incompatible pid
	for i := testMaxInflightLen * 2; i <= testMaxInflightLen*3; i++ {
		puback := &packets.Puback{PacketID: packets.PacketID(i)}
		c.unsetInflight(puback)

		a.Equal(testMaxInflightLen/2, c.session.inflight.Len())
		a.Equal(0, c.session.msgQueue.Len())
	}
	for i := testMaxInflightLen/2 + 1; i <= testMaxInflightLen; i++ {
		puback := &packets.Puback{PacketID: packets.PacketID(i)}
		c.unsetInflight(puback)
		a.Equal(0, c.session.msgQueue.Len())
	}
	a.Equal(0, c.session.inflight.Len())
}

func TestQos2Inflight(t *testing.T) {
	a := assert.New(t)
	c := mockClient()
	for i := 1; i <= testMaxInflightLen; i++ {
		quota := c.clientReceiveMaximumQuota
		pub := &packets.Publish{PacketID: packets.PacketID(i), Qos: packets.Qos2}
		a.True(c.setInflight(pub))
		a.Equal(0, c.session.msgQueue.Len())
		a.EqualValues(quota-1, c.clientReceiveMaximumQuota)
	}
	for i := 1; i <= testMaxInflightLen; i++ {
		pubrec := &packets.Pubrec{PacketID: packets.PacketID(i)}
		c.unsetInflight(pubrec)
		a.Equal(0, c.session.msgQueue.Len())
	}
	a.Equal(0, c.session.inflight.Len())

	for i := 1; i <= testMaxInflightLen; i++ {
		pubcomp := &packets.Pubcomp{PacketID: packets.PacketID(i)}
		c.unsetInflight(pubcomp)
	}
	a.Equal(0, c.session.inflight.Len())
	a.Equal(0, c.session.msgQueue.Len())
}

func TestQos2AwaitRel(t *testing.T) {
	a := assert.New(t)
	c := mockClient()
	for i := 1; i <= testMaxInflightLen; i++ {
		c.setAwaitRel(packets.PacketID(i))
	}
	a.EqualValues(testMaxAwaitRelLen, c.session.awaitRel.Len())

	for i := 1; i <= testMaxInflightLen; i++ {
		c.unsetAwaitRel(packets.PacketID(i))
	}
	a.Equal(0, c.session.awaitRel.Len())
}

func TestMsgQueue(t *testing.T) {
	a := assert.New(t)
	c := fullInflightSessionQos1()
	beginPid := testMaxInflightLen + 1
	j := 0
	for i := beginPid; i < testMaxMsgQueueLen+beginPid; i++ {
		j++
		pub := &packets.Publish{PacketID: packets.PacketID(i), Qos: packets.Qos1}
		a.False(c.setInflight(pub))
		a.EqualValues(j, c.session.msgQueue.Len())
	}
	for i := 1; i <= testMaxInflightLen; i++ {
		puback := &packets.Puback{PacketID: packets.PacketID(i)}
		c.unsetInflight(puback)
	}
	a.Equal(0, c.session.msgQueue.Len())

	for e := c.session.inflight.Front(); e != nil; e = e.Next() {
		elem := e.Value.(*inflightElem)
		a.EqualValues(beginPid, elem.packet.PacketID)
		beginPid++
	}
}

func TestMonitor_MsgQueueDroppedPriority_DropQoS0InQueue(t *testing.T) {
	a := assert.New(t)
	//case 1: removing qos0 message in msgQueue
	c := fullInflightSessionQos1()
	c.config.MaxQueuedMsg = 3
	pub1 := &packets.Publish{PacketID: packets.PacketID(1), Qos: packets.Qos1, Properties: &packets.Properties{}}
	c.msgEnQueue(pub1)
	pub2 := &packets.Publish{PacketID: packets.PacketID(2), Qos: packets.Qos2, Properties: &packets.Properties{}}
	c.msgEnQueue(pub2)
	pub3 := &packets.Publish{PacketID: packets.PacketID(3), Qos: packets.Qos0, Properties: &packets.Properties{}}
	c.msgEnQueue(pub3)
	//msgQueue: pid:1;qos:1 | pid:2;qos:2 | pid:3;qos:0 |
	pub4 := &packets.Publish{PacketID: packets.PacketID(4), Qos: packets.Qos1, Properties: &packets.Properties{}}
	c.msgEnQueue(pub4)
	i := 1
	for e := c.session.msgQueue.Front(); e != nil; e = e.Next() { //drop qos0
		if elem, ok := e.Value.(*queueElem); ok {
			if i == 1 {
				a.EqualValues(1, elem.publish.PacketID)
			}
			if i == 2 {
				a.EqualValues(2, elem.publish.PacketID)
			}
			if i == 3 {
				a.EqualValues(4, elem.publish.PacketID)
			}
			i++
		} else {
			t.Fatalf("unexpected error")
		}
	}
	a.Equal(3, c.session.msgQueue.Len())
}

func TestMonitor_MsgQueueDroppedPriority_DropExpired(t *testing.T) {
	a := assert.New(t)
	c := fullInflightSessionQos1()
	c.config.MaxQueuedMsg = 3
	pub1 := &packets.Publish{PacketID: packets.PacketID(1), Qos: packets.Qos1, Properties: &packets.Properties{
		MessageExpiry: uint32P(1),
	}}
	c.msgEnQueue(pub1)
	pub2 := &packets.Publish{PacketID: packets.PacketID(2), Qos: packets.Qos2, Properties: &packets.Properties{}}
	c.msgEnQueue(pub2)
	pub3 := &packets.Publish{PacketID: packets.PacketID(3), Qos: packets.Qos0, Properties: &packets.Properties{}}
	c.msgEnQueue(pub3)
	time.Sleep(2 * time.Second)
	pub4 := &packets.Publish{PacketID: packets.PacketID(4), Qos: packets.Qos1, Properties: &packets.Properties{}}
	c.msgEnQueue(pub4)
	i := 1
	for e := c.session.msgQueue.Front(); e != nil; e = e.Next() { //drop qos0
		if elem, ok := e.Value.(*queueElem); ok {
			if i == 1 {
				a.EqualValues(2, elem.publish.PacketID)
			}
			if i == 2 {
				a.EqualValues(3, elem.publish.PacketID)
			}
			if i == 3 {
				a.EqualValues(4, elem.publish.PacketID)
			}
			i++
		} else {
			t.Fatalf("unexpected error")
		}
	}
	a.Equal(3, c.session.msgQueue.Len())

}

func TestMonitor_MsgQueueDroppedPriority_DropEnqueueingQoS0(t *testing.T) {
	a := assert.New(t)
	c := fullInflightSessionQos1()
	c.config.MaxQueuedMsg = 3
	pub1 := &packets.Publish{PacketID: packets.PacketID(1), Qos: packets.Qos1, Properties: &packets.Properties{}}
	c.msgEnQueue(pub1)
	pub2 := &packets.Publish{PacketID: packets.PacketID(2), Qos: packets.Qos2, Properties: &packets.Properties{}}
	c.msgEnQueue(pub2)
	pub3 := &packets.Publish{PacketID: packets.PacketID(3), Qos: packets.Qos1, Properties: &packets.Properties{}}
	c.msgEnQueue(pub3)
	//msgQueue: pid:1;qos:1 | pid:2;qos:2 | pid:3;qos:0 |
	pub4 := &packets.Publish{PacketID: packets.PacketID(4), Qos: packets.Qos0, Properties: &packets.Properties{}}
	c.msgEnQueue(pub4)
	i := 1
	for e := c.session.msgQueue.Front(); e != nil; e = e.Next() { //drop qos0
		if elem, ok := e.Value.(*queueElem); ok {
			if i == 1 {
				a.EqualValues(1, elem.publish.PacketID)
			}
			if i == 2 {
				a.EqualValues(2, elem.publish.PacketID)
			}
			if i == 3 {
				a.EqualValues(3, elem.publish.PacketID)
			}
			i++
		} else {
			t.Fatalf("unexpected error")
		}
	}
	a.Equal(3, c.session.msgQueue.Len())
}
func TestMonitor_MsgQueueDroppedPriority_DropFront(t *testing.T) {
	a := assert.New(t)
	//case 1: removing qos0 message in msgQueue
	c := fullInflightSessionQos1()
	c.config.MaxQueuedMsg = 3
	pub1 := &packets.Publish{PacketID: packets.PacketID(1), Qos: packets.Qos1, Properties: &packets.Properties{}}
	c.msgEnQueue(pub1)
	pub2 := &packets.Publish{PacketID: packets.PacketID(2), Qos: packets.Qos2, Properties: &packets.Properties{}}
	c.msgEnQueue(pub2)
	pub3 := &packets.Publish{PacketID: packets.PacketID(3), Qos: packets.Qos1, Properties: &packets.Properties{}}
	c.msgEnQueue(pub3)
	//msgQueue: pid:1;qos:1 | pid:2;qos:2 | pid:3;qos:0 |
	pub4 := &packets.Publish{PacketID: packets.PacketID(4), Qos: packets.Qos1, Properties: &packets.Properties{}}
	c.msgEnQueue(pub4)
	i := 1
	for e := c.session.msgQueue.Front(); e != nil; e = e.Next() { //drop qos0
		if elem, ok := e.Value.(*queueElem); ok {
			if i == 1 {
				a.EqualValues(2, elem.publish.PacketID)
			}
			if i == 2 {
				a.EqualValues(3, elem.publish.PacketID)
			}
			if i == 3 {
				a.EqualValues(4, elem.publish.PacketID)
			}
			i++
		} else {
			t.Fatalf("unexpected error")
		}
	}
	a.Equal(3, c.session.msgQueue.Len())
}
