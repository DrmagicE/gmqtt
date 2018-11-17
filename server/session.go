package server

import (
	"container/list"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"sync"
	"time"
)

type session struct {
	topicsMu   sync.RWMutex
	subTopics  map[string]packets.Topic //all subscribed topics 所有订阅的主题
	inflightMu sync.Mutex               //gard inflight & inflightQueue
	inflight   *list.List               //传输中等待确认的报文
	msgQueueMu sync.Mutex               //gard msgQueue
	msgQueue   *list.List               //缓存数据 publish or pubrel
	//QOS=2 的情况下，判断报文是否是客户端重发报文，如果重发，则不分发.
	// 确保[MQTT-4.3.3-2]中：在收发送PUBREC报文确认任何到对应的PUBREL报文之前，接收者必须后续的具有相同标识符的PUBLISH报文。
	// 在这种情况下，它不能重复分发消息给任何后续的接收者
	unackpublish map[packets.PacketId]bool //[MQTT-4.3.3-2]
	pidMu        sync.RWMutex                //gard LockedPid & nextFreeId
	lockedPid          map[packets.PacketId]bool //Pid inuse
	freePid	packets.PacketId  //下一个可以使用的FreeId

	//config
	maxInflightMessages int
	maxQueueMessages    int
}

type SessionPersistence struct {
	ClientId     string
	SubTopics    map[string]packets.Topic
	Inflight     []*InflightElem
	UnackPublish map[packets.PacketId]bool
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
		ClientId:     client.opts.ClientId,
		SubTopics:    s.subTopics,
		Inflight:     inflight,
		UnackPublish: s.unackpublish,
	}
}

type InflightElem struct {
	At     time.Time //进入时间
	Pid    packets.PacketId
	Packet *packets.Publish
	Step   int
}

//当入队发现缓存队列满的时候：
//按照以下优先级丢弃一个publish报文
//1.缓存队列中QOS0的报文
//2.如果准备入队的报文qos=0,丢弃
//3.丢弃最先进入缓存队列的报文
func (client *Client) msgEnQueue(publish *packets.Publish) {
	s := client.session
	s.msgQueueMu.Lock()
	defer s.msgQueueMu.Unlock()
	if s.msgQueue.Len() >= s.maxQueueMessages && s.maxQueueMessages != 0 {
		if client.server.Monitor != nil {
			client.server.Monitor.MsgQueueDropped(client.opts.ClientId)
		}
		if log != nil {
			log.Printf("%-15s[%s]", "msg queue is overflow, removing msg. ", client.ClientOptions().ClientId)
		}
		var removeMsg *list.Element
		for e := s.msgQueue.Front(); e != nil; e = e.Next() {
			if pub, ok := e.Value.(*packets.Publish); ok {
				if pub.Qos == packets.QOS_0 {
					removeMsg = e
					break
				}
			}
		}
		if removeMsg != nil {
			s.msgQueue.Remove(removeMsg)
			if log != nil {
				log.Printf("%-15s[%s],packet: %s ", "qos 0 msg removed", client.ClientOptions().ClientId, removeMsg.Value.(packets.Packet))
			}
		} else if publish.Qos != packets.QOS_0 {
			e := s.msgQueue.Front()
			s.msgQueue.Remove(e)
			if log != nil {
				p := e.Value.(packets.Packet)
				log.Printf("%-15s[%s],packet: %s ", "first msg removed", client.ClientOptions().ClientId, p)
			}
		}

	}
	s.msgQueue.PushBack(publish)
	if client.server.Monitor != nil {
		client.server.Monitor.MsgEnQueue(client.opts.ClientId)
	}

}

func (client *Client) msgDequeue() *packets.Publish {
	s := client.session
	s.msgQueueMu.Lock()
	defer s.msgQueueMu.Unlock()

	if s.msgQueue.Len() > 0 {
		queueElem := s.msgQueue.Front()
		if log != nil {
			log.Printf("%-15s[%s],packet: %s ", "sending queued msg ", client.ClientOptions().ClientId, queueElem.Value.(*packets.Publish))
		}
		s.msgQueue.Remove(queueElem)
		if client.server.Monitor != nil {
			client.server.Monitor.MsgDeQueue(client.opts.ClientId)
		}
		return queueElem.Value.(*packets.Publish)
	}
	return nil

}

//inflight 入队,inflight队列满，放入缓存队列，缓存队列满，删除最早进入缓存队列的内容
func (client *Client) setInflight(publish *packets.Publish) (enqueue bool) {
	s := client.session
	s.inflightMu.Lock()
	defer s.inflightMu.Unlock()
	defer func() {
		if enqueue && client.server.Monitor != nil {
			client.server.Monitor.AddInflight(client.opts.ClientId)
		}
	}()
	elem := &InflightElem{
		At:     time.Now(),
		Pid:    publish.PacketId,
		Packet: publish,
		Step:   0,
	}
	if s.inflight.Len() >= s.maxInflightMessages && s.maxInflightMessages != 0 { //加入缓存队列
		if log != nil {
			log.Printf("%-15s[%s],packet: %s ", "inflight window is overflow, saving msg into msgQueue", client.ClientOptions().ClientId, elem.Packet)
		}
		client.msgEnQueue(publish)
		enqueue = false
		return
	}

	s.inflight.PushBack(elem)
	enqueue = true
	return

}

//unsetInflight 出队
//packet: puback(QOS1),pubrec(QOS2)  or pubcomp(QOS2)
func (client *Client) unsetInflight(packet packets.Packet) {
	s := client.session
	s.inflightMu.Lock()
	defer s.inflightMu.Unlock()
	var freeId bool
	var pid packets.PacketId
	var isRemove bool
	for e := s.inflight.Front(); e != nil; e = e.Next() {
		if el, ok := e.Value.(*InflightElem); ok {
			switch packet.(type) {
			case *packets.Puback: //QOS1
				if el.Packet.Qos != packets.QOS_1 {
					continue
				}
				pid = packet.(*packets.Puback).PacketId
				freeId = true
				isRemove = true
			case *packets.Pubrec: //QOS2
				if el.Packet.Qos != packets.QOS_2 && el.Step != 0 {
					continue
				}
				pid = packet.(*packets.Pubrec).PacketId
			case *packets.Pubcomp: //QOS2
				if el.Packet.Qos != packets.QOS_2 && el.Step != 1 {
					continue
				}
				freeId = true //[MQTT-4.3.3-1]. 一旦发送者收到PUBCOMP报文，这个报文标识符就可以重用。
				isRemove = true
				pid = packet.(*packets.Pubcomp).PacketId
			}
			if pid == el.Pid {
				if isRemove {
					s.inflight.Remove(e)
					if log != nil {
						log.Printf("%-15s[%s],packet: %s ", "inflight msg released ", client.ClientOptions().ClientId, packet)
					}
					publish := client.msgDequeue()
					if publish != nil {
						elem := &InflightElem{
							At:     time.Now(),
							Pid:    publish.PacketId,
							Packet: publish,
							Step:   0,
						}
						s.inflight.PushBack(elem)
						client.out <- publish
					} else if client.server.Monitor != nil {
						client.server.Monitor.DelInflight(client.opts.ClientId)

					}
				} else {
					el.Step++
				}
				if freeId {
					s.freePacketId(pid)
				}
				return
			}
		}
	}

}

func (s *session) freePacketId(id packets.PacketId) {
	s.pidMu.Lock()
	defer s.pidMu.Unlock()
	s.lockedPid[id] = false
}

func (s *session) setPacketId(id packets.PacketId) {
	s.pidMu.Lock()
	defer s.pidMu.Unlock()
	s.lockedPid[id] = true
}


func (s *session) getPacketId() packets.PacketId {
	s.pidMu.RLock()
	defer s.pidMu.RUnlock()
	for s.lockedPid[s.freePid] {
		s.freePid++
		if s.freePid > packets.MAX_PACKET_ID {
			s.freePid = packets.MIN_PACKET_ID
		}
	}
	id := s.freePid
	s.freePid++
	if s.freePid > packets.MAX_PACKET_ID {
		s.freePid = packets.MIN_PACKET_ID
	}
	return id
}
