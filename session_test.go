package gmqtt

import (
	"fmt"
	"testing"

	"go.uber.org/zap"

	"github.com/DrmagicE/gmqtt/pkg/packets"
)

const testMaxInflightLen = 20
const testMaxAwaitRelLen = 20
const testMaxMsgQueueLen = 20

func init() {
	zaplog, _ = zap.NewProduction()
}

//mock client,only for session_test.go
func mockClient() *client {
	config := DefaultConfig
	config.MaxInflight = testMaxInflightLen
	config.MaxMsgQueue = testMaxMsgQueueLen
	config.MaxAwaitRel = testMaxAwaitRelLen
	b := NewServer(WithConfig(config))
	c := b.newClient(nil)
	c.opts.cleanSession = true
	c.newSession()
	return c
}
func fullInflightSessionQos1() *client {
	c := mockClient()
	for i := 1; i <= testMaxInflightLen; i++ {
		pub := &packets.Publish{PacketID: packets.PacketID(i), Qos: packets.QOS_1}
		c.setInflight(pub)
	}
	return c
}
func TestQos1Inflight(t *testing.T) {
	c := mockClient()
	for i := 1; i <= testMaxInflightLen; i++ {
		pub := &packets.Publish{PacketID: packets.PacketID(i), Qos: packets.QOS_1}
		if !c.setInflight(pub) {
			t.Fatalf("setInflight error, want true, but false")
		}
		if c.session.msgQueue.Len() != 0 {
			t.Fatalf("msgQueue.Len() error, want 0, but %d", c.session.msgQueue.Len())
		}
	}
	for i := 1; i <= testMaxInflightLen/2; i++ {
		puback := &packets.Puback{PacketID: packets.PacketID(i)}
		c.unsetInflight(puback)
		if c.session.inflight.Len() != testMaxInflightLen-i {
			t.Fatalf("msgQueue.Len() error, want %d, but %d", testMaxInflightLen-i, c.session.inflight.Len())
		}
		if c.session.msgQueue.Len() != 0 {
			t.Fatalf("msgQueue.Len() error, want 0, but %d", c.session.msgQueue.Len())
		}
	}

	//test incompatible pid
	for i := testMaxInflightLen * 2; i <= testMaxInflightLen*3; i++ {
		puback := &packets.Puback{PacketID: packets.PacketID(i)}
		c.unsetInflight(puback)
		if c.session.inflight.Len() != testMaxInflightLen/2 {
			t.Fatalf("inflight.Len() error, want %d, but %d", testMaxInflightLen/2, c.session.inflight.Len())
		}
		if c.session.msgQueue.Len() != 0 {
			t.Fatalf("msgQueue.Len() error, want 00, but %d", c.session.msgQueue.Len())
		}
	}
	for i := testMaxInflightLen/2 + 1; i <= testMaxInflightLen; i++ {
		puback := &packets.Puback{PacketID: packets.PacketID(i)}
		c.unsetInflight(puback)
		if c.session.msgQueue.Len() != 0 {
			t.Fatalf("msgQueue.Len() error, want 00, but %d", c.session.msgQueue.Len())
		}
	}
	if c.session.inflight.Len() != 0 {
		t.Fatalf("inflight.Len() error, want %d, but %d", 0, c.session.inflight.Len())
	}
}

func TestQos2Inflight(t *testing.T) {
	c := mockClient()
	for i := 1; i <= testMaxInflightLen; i++ {
		pub := &packets.Publish{PacketID: packets.PacketID(i), Qos: packets.QOS_2}
		if !c.setInflight(pub) {
			t.Fatalf("setInflight error, want true, but false")
		}
		if c.session.msgQueue.Len() != 0 {
			t.Fatalf("msgQueue.Len() error, want 00, but %d", c.session.msgQueue.Len())
		}
	}
	for i := 1; i <= testMaxInflightLen; i++ {
		pubrec := &packets.Pubrec{PacketID: packets.PacketID(i)}
		c.unsetInflight(pubrec)
		if c.session.msgQueue.Len() != 0 {
			t.Fatalf("msgQueue.Len() error, want 0, but %d", c.session.msgQueue.Len())
		}
	}
	if c.session.inflight.Len() != 0 {
		t.Fatalf("inflight.Len() error, want 0, but %d", c.session.inflight.Len())
	}

	/*	for i := 1; i <= testMaxInflightLen; i++ {
			pubcomp := &packets.Pubcomp{PacketID: packets.PacketID(i)}
			c.unsetInflight(pubcomp)
		}
		if c.session.inflight.Len() != 0 {
			t.Fatalf("inflight.Len() error, want %d, but %d", 0, c.session.inflight.Len())
		}
		if c.session.msgQueue.Len() != 0 {
			t.Fatalf("msgQueue.Len() error, want 0, but %d", c.session.msgQueue.Len())
		}*/
}
func TestQos2AwaitRel(t *testing.T) {
	c := mockClient()
	for i := 1; i <= testMaxInflightLen; i++ {
		c.setAwaitRel(packets.PacketID(i))
	}
	if c.session.awaitRel.Len() != testMaxAwaitRelLen {
		t.Fatalf("awaitRel.Len() error, want %d, but %d", testMaxAwaitRelLen, c.session.awaitRel.Len())
	}
	for i := 1; i <= testMaxInflightLen; i++ {
		c.unsetAwaitRel(packets.PacketID(i))
	}
	if c.session.awaitRel.Len() != 0 {
		t.Fatalf("awaitRel.Len() error, want %d, but %d", 0, c.session.awaitRel.Len())
	}
}

func TestMsgQueue(t *testing.T) {
	c := fullInflightSessionQos1()
	beginPid := testMaxInflightLen + 1
	j := 0
	for i := beginPid; i < testMaxMsgQueueLen+beginPid; i++ {
		j++
		pub := &packets.Publish{PacketID: packets.PacketID(i), Qos: packets.QOS_1}
		if c.setInflight(pub) {
			t.Fatalf("setInflight error, want fase, but true")
		}
		if c.session.msgQueue.Len() != j {
			t.Fatalf("msgQueue.Len() error, want %d, but %d", j, c.session.msgQueue.Len())
		}
	}
	for i := 1; i <= testMaxInflightLen; i++ {
		puback := &packets.Puback{PacketID: packets.PacketID(i)}
		c.unsetInflight(puback)
	}
	if c.session.msgQueue.Len() != 0 {
		t.Fatalf("msgQueue.Len() error, want %d, but %d", 0, c.session.msgQueue.Len())
	}
	for e := c.session.inflight.Front(); e != nil; e = e.Next() {
		elem := e.Value.(*inflightElem)
		if elem.packet.PacketID != packets.PacketID(beginPid) {
			t.Fatalf("inflightElem.pid error, want %d, but %d", beginPid, elem.packet.PacketID)
		}
		beginPid++
	}
}

//当入队发现缓存队列满的时候：
//按照以下优先级丢弃publish报文
//1.缓存队列中QOS0的报文
//2.丢弃报文QOS=0的当前需要入队的报文
//3.丢弃最先进入缓存队列的报文
func TestMonitor_MsgQueueDroppedPriority(t *testing.T) {
	//case 1: removing qos0 message in msgQueue
	c := fullInflightSessionQos1()
	c.session.config.MaxMsgQueue = 3
	pub1 := &packets.Publish{PacketID: packets.PacketID(1), Qos: packets.QOS_1}
	c.msgEnQueue(pub1)
	pub2 := &packets.Publish{PacketID: packets.PacketID(2), Qos: packets.QOS_2}
	c.msgEnQueue(pub2)
	pub3 := &packets.Publish{PacketID: packets.PacketID(3), Qos: packets.QOS_0}
	c.msgEnQueue(pub3)
	//msgQueue: pid:1;qos:1 | pid:2;qos:2 | pid:3;qos:0 |
	pub4 := &packets.Publish{PacketID: packets.PacketID(4), Qos: packets.QOS_1}
	c.msgEnQueue(pub4)
	i := 1
	for e := c.session.msgQueue.Front(); e != nil; e = e.Next() { //drop qos0
		if elem, ok := e.Value.(*packets.Publish); ok {
			fmt.Println(elem)
			if i == 1 && elem.PacketID != 1 {
				t.Fatalf("msgQueue dropping priority  error, want %d ,got %d", i, elem.PacketID)
			}
			if i == 2 && elem.PacketID != 2 {
				t.Fatalf("msgQueue dropping priority  error, want %d ,got %d", i, elem.PacketID)
			}
			if i == 3 && elem.PacketID != 4 {
				t.Fatalf("msgQueue dropping priority  error, want 4 ,got %d", elem.PacketID)
			}
			i++
		} else {
			t.Fatalf("unexpected error")
		}
	}
	if c.session.msgQueue.Len() != 3 {
		t.Fatalf("msgQueue.Len() error, want 3,but got %d", c.session.msgQueue.Len())
	}

	//case 2: dropping current qos0 message
	c2 := fullInflightSessionQos1()
	c2.session.config.MaxMsgQueue = 3
	pub21 := &packets.Publish{PacketID: packets.PacketID(1), Qos: packets.QOS_1}
	c2.msgEnQueue(pub21)
	pub22 := &packets.Publish{PacketID: packets.PacketID(2), Qos: packets.QOS_2}
	c2.msgEnQueue(pub22)
	pub23 := &packets.Publish{PacketID: packets.PacketID(3), Qos: packets.QOS_1}
	c2.msgEnQueue(pub23)
	//msgQueue: pid:1;qos:1 | pid:2;qos:2 | pid:3;qos:1 |
	pub24 := &packets.Publish{PacketID: packets.PacketID(4), Qos: packets.QOS_0}
	c2.msgEnQueue(pub24)
	i = 1
	for e := c2.session.msgQueue.Front(); e != nil; e = e.Next() {
		if elem, ok := e.Value.(*packets.Publish); ok {
			if i == 1 && elem.PacketID != 1 {
				t.Fatalf("msgQueue dropping priority  error, want %d ,got %d", i, elem.PacketID)
			}
			if i == 2 && elem.PacketID != 2 {
				t.Fatalf("msgQueue dropping priority  error, want %d ,got %d", i, elem.PacketID)
			}
			if i == 3 && elem.PacketID != 3 {
				t.Fatalf("msgQueue dropping priority  error, want %d ,got %d", i, elem.PacketID)
			}
			i++
		} else {
			t.Fatalf("unexpected error")
		}
	}
	if c2.session.msgQueue.Len() != 3 {
		t.Fatalf("msgQueue.Len() error, want 3,but got %d", c2.session.msgQueue.Len())
	}

	//case 3:removing the front message of msgQueue
	c3 := fullInflightSessionQos1()
	c3.session.config.MaxMsgQueue = 3
	pub31 := &packets.Publish{PacketID: packets.PacketID(1), Qos: packets.QOS_1}
	c3.msgEnQueue(pub31)
	pub32 := &packets.Publish{PacketID: packets.PacketID(2), Qos: packets.QOS_2}
	c3.msgEnQueue(pub32)
	pub33 := &packets.Publish{PacketID: packets.PacketID(3), Qos: packets.QOS_1}
	c3.msgEnQueue(pub33)
	//msgQueue: pid:1;qos:1 | pid:2;qos:2 | pid:3;qos:1 |

	pub34 := &packets.Publish{PacketID: packets.PacketID(4), Qos: packets.QOS_1}
	c3.msgEnQueue(pub34)
	i = 1
	//当缓存队列满
	for e := c3.session.msgQueue.Front(); e != nil; e = e.Next() { //drop qos0
		if elem, ok := e.Value.(*packets.Publish); ok {
			if i == 1 && elem.PacketID != 2 {
				t.Fatalf("msgQueue dropping priority  error, want 2 ,got %d", elem.PacketID)
			}
			if i == 2 && elem.PacketID != 3 {
				t.Fatalf("msgQueue dropping priority  error, want 3 ,got %d", elem.PacketID)
			}
			if i == 3 && elem.PacketID != 4 {
				t.Fatalf("msgQueue dropping priority  error, want 4 ,got %d", elem.PacketID)
			}
			i++
		} else {
			t.Fatalf("unexpected error")
		}
	}
	if c3.session.msgQueue.Len() != 3 {
		t.Fatalf("msgQueue.Len() error, want 3,but got %d", c3.session.msgQueue.Len())
	}

}
