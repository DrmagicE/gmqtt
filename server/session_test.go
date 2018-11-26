package server

import (
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"testing"
)

const test_max_inflight_len = 20
const test_max_msgQueue_len = 20

//mock client,only for session_test.go
func mockClient() *Client {
	b := &Server{
		config: &config{
			maxInflightMessages: test_max_inflight_len,
			maxQueueMessages:    test_max_msgQueue_len,
		},
	}
	c := b.newClient(nil)
	c.opts.CleanSession = true
	c.newSession()
	return c
}
func fullInflightSessionQos1() *Client {
	c := mockClient()
	for i := 1; i <= test_max_inflight_len; i++ {
		pub := &packets.Publish{PacketId: packets.PacketId(i), Qos: packets.QOS_1}
		c.setInflight(pub)
	}
	return c
}
func TestQos1Inflight(t *testing.T) {
	c := mockClient()
	for i := 1; i <= test_max_inflight_len; i++ {
		pub := &packets.Publish{PacketId: packets.PacketId(i), Qos: packets.QOS_1}
		if !c.setInflight(pub) {
			t.Fatalf("setInflight error, want true, but false")
		}
		if c.session.msgQueue.Len() != 0 {
			t.Fatalf("msgQueue.Len() error, want 00, but %d", c.session.msgQueue.Len())
		}
	}
	for i := 1; i <= test_max_inflight_len/2; i++ {
		puback := &packets.Puback{PacketId: packets.PacketId(i)}
		c.unsetInflight(puback)
		if c.session.inflight.Len() != test_max_inflight_len-i {
			t.Fatalf("msgQueue.Len() error, want %d, but %d", test_max_inflight_len-i, c.session.inflight.Len())
		}
		if c.session.msgQueue.Len() != 0 {
			t.Fatalf("msgQueue.Len() error, want 00, but %d", c.session.msgQueue.Len())
		}
	}

	//test pubrel & pubcomp
	for i := test_max_inflight_len/2 + 1; i <= test_max_inflight_len; i++ {
		pubrel := &packets.Pubrel{PacketId: packets.PacketId(i)}
		pubcomp := &packets.Pubcomp{PacketId: packets.PacketId(i)}
		c.unsetInflight(pubrel)
		c.unsetInflight(pubcomp)
		if c.session.inflight.Len() != test_max_inflight_len/2 {
			t.Fatalf("inflight.Len() error, want %d, but %d", test_max_inflight_len/2, c.session.inflight.Len())
		}
		if c.session.msgQueue.Len() != 0 {
			t.Fatalf("msgQueue.Len() error, want 00, but %d", c.session.msgQueue.Len())
		}
	}
	//test imcompatible pid
	for i := test_max_inflight_len * 2; i <= test_max_inflight_len*3; i++ {
		puback := &packets.Puback{PacketId: packets.PacketId(i)}
		c.unsetInflight(puback)
		if c.session.inflight.Len() != test_max_inflight_len/2 {
			t.Fatalf("inflight.Len() error, want %d, but %d", test_max_inflight_len/2, c.session.inflight.Len())
		}
		if c.session.msgQueue.Len() != 0 {
			t.Fatalf("msgQueue.Len() error, want 00, but %d", c.session.msgQueue.Len())
		}
	}
	for i := test_max_inflight_len/2 + 1; i <= test_max_inflight_len; i++ {
		puback := &packets.Puback{PacketId: packets.PacketId(i)}
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
	for i := 1; i <= test_max_inflight_len; i++ {
		pub := &packets.Publish{PacketId: packets.PacketId(i), Qos: packets.QOS_2}
		if !c.setInflight(pub) {
			t.Fatalf("setInflight error, want true, but false")
		}
		if c.session.msgQueue.Len() != 0 {
			t.Fatalf("msgQueue.Len() error, want 00, but %d", c.session.msgQueue.Len())
		}
	}
	for i := 1; i <= test_max_inflight_len; i++ {
		pubrec := &packets.Pubrec{PacketId: packets.PacketId(i)}
		c.unsetInflight(pubrec)
		puback := &packets.Puback{PacketId: packets.PacketId(i)}
		c.unsetInflight(puback)
		if c.session.inflight.Len() != test_max_inflight_len {
			t.Fatalf("inflight.Len() error, want %d, but %d", test_max_inflight_len, c.session.inflight.Len())
		}
		if c.session.msgQueue.Len() != 0 {
			t.Fatalf("msgQueue.Len() error, want 0, but %d", c.session.msgQueue.Len())
		}
	}
	for e := c.session.inflight.Front(); e != nil; e = e.Next() {
		elem := e.Value.(*InflightElem)
		if elem.Step != 1 {
			t.Fatalf("InflightElem.Step error, want 1, but %d", elem.Step)
		}
	}

	for i := 1; i <= test_max_inflight_len; i++ {
		pubcomp := &packets.Pubcomp{PacketId: packets.PacketId(i)}
		c.unsetInflight(pubcomp)
	}
	if c.session.inflight.Len() != 0 {
		t.Fatalf("inflight.Len() error, want %d, but %d", 0, c.session.inflight.Len())
	}
	if c.session.msgQueue.Len() != 0 {
		t.Fatalf("msgQueue.Len() error, want 0, but %d", c.session.msgQueue.Len())
	}
}
func TestMsgQueue(t *testing.T) {
	c := fullInflightSessionQos1()
	beginPid := test_max_inflight_len + 1
	j := 0
	for i := beginPid; i < test_max_msgQueue_len+beginPid; i++ {
		j++
		pub := &packets.Publish{PacketId: packets.PacketId(i), Qos: packets.QOS_1}
		if c.setInflight(pub) {
			t.Fatalf("setInflight error, want fase, but true")
		}

		if c.session.msgQueue.Len() != j {
			t.Fatalf("msgQueue.Len() error, want %d, but %d", j, c.session.msgQueue.Len())
		}
	}
	msgQueueLen := c.session.msgQueue.Len()
	for i := 1; i <= test_max_inflight_len; i++ {
		pubrec := &packets.Pubrec{PacketId: packets.PacketId(i)}
		c.unsetInflight(pubrec)
		if c.session.msgQueue.Len() != msgQueueLen {
			t.Fatalf("msgQueue.Len() error, want %d, but %d", msgQueueLen, c.session.msgQueue.Len())
		}
	}

	for i := 1; i <= test_max_inflight_len; i++ {
		pubcomp := &packets.Pubcomp{PacketId: packets.PacketId(i)}
		c.unsetInflight(pubcomp)
		if c.session.msgQueue.Len() != msgQueueLen-i {
			t.Fatalf("msgQueue.Len() error, want %d, but %d", msgQueueLen, c.session.msgQueue.Len())
		}
		if c.session.inflight.Len() != test_max_inflight_len {
			t.Fatalf("inflight.Len() error, want %d, but %d", test_max_inflight_len, c.session.inflight.Len())
		}
	}
	if c.session.msgQueue.Len() != 0 {
		t.Fatalf("msgQueue.Len() error, want %d, but %d", 0, c.session.msgQueue.Len())
	}

	for e := c.session.inflight.Front(); e != nil; e = e.Next() {
		elem := e.Value.(*InflightElem)
		if elem.Pid != packets.PacketId(beginPid) {
			t.Fatalf("InflightElem.Pid error, want %d, but %d", beginPid, elem.Pid)
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
	c.session.maxQueueMessages = 3
	pub1 := &packets.Publish{PacketId: packets.PacketId(1), Qos: packets.QOS_1}
	c.msgEnQueue(pub1)
	pub2 := &packets.Publish{PacketId: packets.PacketId(2), Qos: packets.QOS_2}
	c.msgEnQueue(pub2)
	pub3 := &packets.Publish{PacketId: packets.PacketId(3), Qos: packets.QOS_0}
	c.msgEnQueue(pub3)
	//msgQueue: pid:1;qos:1 | pid:2;qos:2 | pid:3;qos:0 |
	pub4 := &packets.Publish{PacketId: packets.PacketId(4), Qos: packets.QOS_1}
	c.msgEnQueue(pub4)
	i := 1
	for e := c.session.msgQueue.Front(); e != nil; e = e.Next() { //drop qos0
		if elem, ok := e.Value.(*packets.Publish); ok {
			if i == 1 && elem.PacketId != 1 {
				t.Fatalf("msgQueue dropping priority  error, want %d ,got %d", i, elem.PacketId)
			}
			if i == 2 && elem.PacketId != 2 {
				t.Fatalf("msgQueue dropping priority  error, want %d ,got %d", i, elem.PacketId)
			}
			if i == 3 && elem.PacketId != 4 {
				t.Fatalf("msgQueue dropping priority  error, want 4 ,got %d", elem.PacketId)
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
	c2.session.maxQueueMessages = 3
	pub21 := &packets.Publish{PacketId: packets.PacketId(1), Qos: packets.QOS_1}
	c2.msgEnQueue(pub21)
	pub22 := &packets.Publish{PacketId: packets.PacketId(2), Qos: packets.QOS_2}
	c2.msgEnQueue(pub22)
	pub23 := &packets.Publish{PacketId: packets.PacketId(3), Qos: packets.QOS_1}
	c2.msgEnQueue(pub23)
	//msgQueue: pid:1;qos:1 | pid:2;qos:2 | pid:3;qos:1 |
	pub24 := &packets.Publish{PacketId: packets.PacketId(4), Qos: packets.QOS_0}
	c2.msgEnQueue(pub24)
	i = 1
	for e := c2.session.msgQueue.Front(); e != nil; e = e.Next() {
		if elem, ok := e.Value.(*packets.Publish); ok {
			if i == 1 && elem.PacketId != 1 {
				t.Fatalf("msgQueue dropping priority  error, want %d ,got %d", i, elem.PacketId)
			}
			if i == 2 && elem.PacketId != 2 {
				t.Fatalf("msgQueue dropping priority  error, want %d ,got %d", i, elem.PacketId)
			}
			if i == 3 && elem.PacketId != 3 {
				t.Fatalf("msgQueue dropping priority  error, want %d ,got %d", i, elem.PacketId)
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
	c3.session.maxQueueMessages = 3
	pub31 := &packets.Publish{PacketId: packets.PacketId(1), Qos: packets.QOS_1}
	c3.msgEnQueue(pub31)
	pub32 := &packets.Publish{PacketId: packets.PacketId(2), Qos: packets.QOS_2}
	c3.msgEnQueue(pub32)
	pub33 := &packets.Publish{PacketId: packets.PacketId(3), Qos: packets.QOS_1}
	c3.msgEnQueue(pub33)
	//msgQueue: pid:1;qos:1 | pid:2;qos:2 | pid:3;qos:1 |

	pub34 := &packets.Publish{PacketId: packets.PacketId(4), Qos: packets.QOS_1}
	c3.msgEnQueue(pub34)
	i = 1
	//当缓存队列满
	for e := c3.session.msgQueue.Front(); e != nil; e = e.Next() { //drop qos0
		if elem, ok := e.Value.(*packets.Publish); ok {
			if i == 1 && elem.PacketId != 2 {
				t.Fatalf("msgQueue dropping priority  error, want 2 ,got %d", elem.PacketId)
			}
			if i == 2 && elem.PacketId != 3 {
				t.Fatalf("msgQueue dropping priority  error, want 3 ,got %d", elem.PacketId)
			}
			if i == 3 && elem.PacketId != 4 {
				t.Fatalf("msgQueue dropping priority  error, want 4 ,got %d", elem.PacketId)
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
