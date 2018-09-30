package server

import (
	"container/list"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"sync"
	"time"
)

type session struct {
	sync.Mutex    //gard needStore
	needStore     bool
	topicsMu      sync.Mutex
	subTopics     map[string]packets.Topic //all subscribed topics 所有订阅的主题
	inflightMu    sync.Mutex               //gard inflight
	inflight      *list.List               //传输中等待确认的报文
	inflightToken chan struct{}            //inflight达到最大值的时候阻塞
	//QOS=2 的情况下，判断报文是否是客户端重发报文，如果重发，则不分发.
	// 确保[MQTT-4.3.3-2]中：在收发送PUBREC报文确认任何到对应的PUBREL报文之前，接收者必须后续的具有相同标识符的PUBLISH报文。
	// 在这种情况下，它不能重复分发消息给任何后续的接收者
	unackpublish map[packets.PacketId]bool //[MQTT-4.3.3-2]
	pidMu        sync.Mutex                //gard pid
	pid          map[packets.PacketId]bool //可以使用的pid
	//离线队列 相关
	offlineQueueMu sync.Mutex //gard offlineQueue
	offlineQueue   *list.List //offline msg 保存离线消息
	offlineAt      time.Time  //离线时间
}

type SessionPersistence struct {
	ClientId     string
	SubTopics    map[string]packets.Topic
	Inflight     []*InflightElem
	UnackPublish map[packets.PacketId]bool
	Pid          map[packets.PacketId]bool
}

func (client *Client) NewPersistence() *SessionPersistence {
	s := client.session
	inflight := make([]*InflightElem, 0, s.inflight.Len())
	for {
		if s.inflight.Front() == nil {
			break
		}
		inflight = append(inflight, s.inflight.Remove(s.inflight.Front()).(*InflightElem))
	}
	return &SessionPersistence{
		ClientId: client.opts.ClientId,
		SubTopics:    s.subTopics,
		Inflight:     inflight,
		UnackPublish: s.unackpublish,
		Pid:          s.pid,
	}
}
type InflightElem struct {
	At     time.Time //进入时间
	Pid    packets.PacketId
	Packet packets.Packet
}

//inflight 入队
func (client *Client) setInflight(elem *InflightElem) {
	s := client.session
	s.inflightMu.Lock()
	if s.inflight.Len() >= client.server.config.maxInflightMessages {
		s.inflightMu.Unlock() //释放锁,防止阻塞的时候，unsetInflight无法获得锁
		//达到了最大值,阻塞等
		select {
		case <-client.close:
			//关闭
			return
		case s.inflightToken <- struct{}{}:
			//达到最大值，阻塞。
			s.inflightMu.Lock() //释放token之后重新获得锁
		}
	}
	s.inflight.PushBack(elem)
	s.inflightMu.Unlock()

}

//inflight 出队
func (client *Client) unsetInflight(elem *InflightElem) {
	s := client.session
	s.inflightMu.Lock()
	defer s.inflightMu.Unlock()
	for e := s.inflight.Front(); e != nil; e = e.Next() {
		if el, ok := e.Value.(*InflightElem); ok {
			if elem.Pid == el.Pid {
				s.inflight.Remove(e)
				s.freePacketId(elem.Pid)
				//释放
				select {
				case <-s.inflightToken:
				default:
				}
				return
			}
		}
	}
}

func (s *session) freePacketId(id packets.PacketId) {
	s.pidMu.Lock()
	defer s.pidMu.Unlock()
	delete(s.pid, id)
}

func (s *session) getPacketId() packets.PacketId {
	s.pidMu.Lock()
	defer s.pidMu.Unlock()
	for i := packets.MIN_PACKET_ID; i < packets.MAX_PACKET_ID; i++ {
		if _, ok := s.pid[i]; !ok {
			s.pid[i] = true
			return i
		}
	}
	return 0
}
