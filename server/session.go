package server

import (
	"container/list"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"sync"
	"time"
)




type session struct {
	needStore     bool
	clientMu     sync.Mutex
	client        *Client
	topicsMu      sync.Mutex
	subTopics     map[string]packets.Topic //所有订阅的subscribe
	inflightMu    sync.Mutex
	inflight      *list.List    //传输中等待确认的报文
	inflightToken chan struct{} //inflight达到最大值的时候阻塞

	//QOS=2 的情况下，判断报文是否是客户端重发报文，如果重发，则不分发.
	// 确保[MQTT-4.3.3-2]中：在收发送PUBREC报文确认任何到对应的PUBREL报文之前，接收者必须后续的具有相同标识符的PUBLISH报文。
	// 在这种情况下，它不能重复分发消息给任何后续的接收者
	unackpublish map[packets.PacketId]bool
	pidMu        sync.Mutex                //gard pid
	pid          map[packets.PacketId]bool //可以使用的pid
	//离线队列 相关
	offlineQueueMu sync.Mutex //gard offlineQueue
	offlineQueue   *list.List //保存离线消息
	offlineAt      time.Time  //离线时间
	ready          chan struct{}
}

func (s *session) SetClient(client *Client) {
	s.clientMu.Lock()
	defer s.clientMu.Unlock()
	s.client = client
}

type inflightElem struct {
	at     time.Time //进入时间
	pid    packets.PacketId
	packet packets.Packet
}

func newSession(client *Client) *session {
	s := &session{
		client:        client,
		subTopics:     make(map[string]packets.Topic),
		unackpublish:  make(map[packets.PacketId]bool),
		inflight:      list.New(),
		inflightToken: make(chan struct{}),
		pid:           make(map[packets.PacketId]bool),
		offlineQueue:  list.New(),
		ready:         make(chan struct{}, 1),
		needStore:     !client.opts.CleanSession,
	}

	return s
}

//inflight 入队
func (s *session) setInflight(elem *inflightElem) {
	s.inflightMu.Lock()
	if s.inflight.Len() == s.client.server.config.maxInflightMessages {
		s.inflightMu.Unlock() //释放锁,防止阻塞的时候，unsetInflight无法获得锁
		//达到了最大值,阻塞等
		select {
		case <-s.client.close:
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
func (s *session) unsetInflight(elem *inflightElem) {
	s.inflightMu.Lock()
	defer s.inflightMu.Unlock()
	for e := s.inflight.Front(); e != nil; e = e.Next() {
		if el, ok := e.Value.(*inflightElem); ok {
			if elem.pid == el.pid {
				s.inflight.Remove(e)
				s.freePacketId(elem.pid)
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

func (s *session) write(packet packets.Packet) {
	if s.client.Status() == CONNECTED { //在线消息
		s.onlineWrite(packet)
	} else { //离线消息
		s.offlineWrite(packet)
	}
}

func (s *session) onlineWrite(packet packets.Packet) {
	<-s.ready //等待session准备好
	select {
	case s.client.out <- packet:
	default:
		s.client.setError(ErrWriteBufFull)
	}
}

func (s *session) offlineWrite(packet packets.Packet) {
	if pub,ok:= packet.(*packets.Publish);ok {
		if pub.Qos == packets.QOS_0 && s.client.server.config.queueQos0Messages == false {
			return
		}
	}
	log.Printf("%-15s[%s] %s ","queueing offline msg cid", s.client.ClientOption().ClientId, packet)
	s.offlineQueue.PushBack(packet)
}

//分发publish报文
func (s *session) deliver(incoming *packets.Publish, isRetain bool) {
	s.topicsMu.Lock()
	var matchTopic packets.Topic
	var isMatch bool
	once := sync.Once{}
	for _, topic := range s.subTopics {
		if packets.TopicMatch(incoming.TopicName, []byte(topic.Name)) {
			once.Do(func() {
				matchTopic = topic
				isMatch = true
			})
			if topic.Qos > matchTopic.Qos { //[MQTT-3.3.5-1]
				matchTopic = topic
			}
		}
	}
	s.topicsMu.Unlock()
	if isMatch { //匹配
		publish := incoming.CopyPublish()
		if publish.Qos > matchTopic.Qos {
			publish.Qos = matchTopic.Qos
		}
		if publish.Qos > 0 {
			publish.PacketId = s.getPacketId()
		}
		publish.Dup = false
		publish.Retain = isRetain
		s.write(publish)
	}
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
