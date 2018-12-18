package gmqtt

import (
	"container/list"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"sync"
	"time"
)

const maxInflightMessages = 65535

type session struct {
	inflightMu sync.Mutex //gard inflight
	inflight   *list.List //传输中等待确认的报文
	msgQueueMu sync.Mutex //gard msgQueue
	msgQueue   *list.List //缓存数据，缓存publish报文
	//QOS=2 的情况下，判断报文是否是客户端重发报文，如果重发，则不分发.
	// 确保[MQTT-4.3.3-2]中：在收发送PUBREC报文确认任何到对应的PUBREL报文之前，接收者必须后续的具有相同标识符的PUBLISH报文。
	// 在这种情况下，它不能重复分发消息给任何后续的接收者
	unackpublish map[packets.PacketID]bool //[MQTT-4.3.3-2]
	pidMu        sync.RWMutex              //gard lockedPid & freeID
	lockedPid    map[packets.PacketID]bool //Pid inuse
	freePid      packets.PacketID          //下一个可以使用的freeID
	//config
	maxInflightMessages int
	maxQueueMessages    int
}

//InflightElem is the element type in inflight queue
type InflightElem struct {
	//At is the entry time
	At time.Time
	//Pid is the packetID
	Pid packets.PacketID
	//Packet represents Publish packet
	Packet *packets.Publish
	Step   int
}

//当入队发现缓存队列满的时候：
//按照以下优先级丢弃一个publish报文
//1.缓存队列中QOS0的报文
//2.如果准备入队的报文qos=0,丢弃
//3.丢弃最先进入缓存队列的报文

//When the len of msgQueueu is reaching the maximum setting, message will be dropped according to the following priorities：
//1. qos0 message in the msgQueue
//2. qos0 message that is going to enqueue
//3. the front message of msgQueue
func (client *Client) msgEnQueue(publish *packets.Publish) {
	s := client.session
	s.msgQueueMu.Lock()
	defer s.msgQueueMu.Unlock()
	if s.msgQueue.Len() >= s.maxQueueMessages && s.maxQueueMessages != 0 {
		if client.server.Monitor != nil {
			client.server.Monitor.msgQueueDropped(client.opts.ClientID)
		}
		if log != nil {
			log.Printf("%-15s[%s]", "msg queue is overflow, removing msg. ", client.ClientOptions().ClientID)
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
		if removeMsg != nil { //case1: removing qos0 message in the msgQueue
			s.msgQueue.Remove(removeMsg)
			if log != nil {
				log.Printf("%-15s[%s],packet: %s ", "qos 0 msg removed", client.ClientOptions().ClientID, removeMsg.Value.(packets.Packet))
			}
		} else if publish.Qos == packets.QOS_0 { //case2: removing qos0 message that is going to enqueue
			return
		} else { //case3: removing the front message of msgQueue
			e := s.msgQueue.Front()
			s.msgQueue.Remove(e)
			if log != nil {
				p := e.Value.(packets.Packet)
				log.Printf("%-15s[%s],packet: %s ", "first msg removed", client.ClientOptions().ClientID, p)
			}
		}
	} else if client.server.Monitor != nil {
		client.server.Monitor.msgEnQueue(client.opts.ClientID)
	}
	s.msgQueue.PushBack(publish)

}

func (client *Client) msgDequeue() *packets.Publish {
	s := client.session
	s.msgQueueMu.Lock()
	defer s.msgQueueMu.Unlock()

	if s.msgQueue.Len() > 0 {
		queueElem := s.msgQueue.Front()
		if log != nil {
			log.Printf("%-15s[%s],packet: %s ", "sending queued msg ", client.ClientOptions().ClientID, queueElem.Value.(*packets.Publish))
		}
		s.msgQueue.Remove(queueElem)
		if client.server.Monitor != nil {
			client.server.Monitor.msgDeQueue(client.opts.ClientID)
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
			client.server.Monitor.addInflight(client.opts.ClientID)
		}
	}()
	elem := &InflightElem{
		At:     time.Now(),
		Pid:    publish.PacketID,
		Packet: publish,
		Step:   0,
	}
	if s.inflight.Len() >= s.maxInflightMessages && s.maxInflightMessages != 0 { //加入缓存队列
		if log != nil {
			log.Printf("%-15s[%s],packet: %s ", "inflight window is overflow, saving msg into msgQueue", client.ClientOptions().ClientID, elem.Packet)
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
	var freeID bool
	var pid packets.PacketID
	var isRemove bool
	for e := s.inflight.Front(); e != nil; e = e.Next() {
		if el, ok := e.Value.(*InflightElem); ok {
			switch packet.(type) {
			case *packets.Puback: //QOS1
				if el.Packet.Qos != packets.QOS_1 {
					continue
				}
				pid = packet.(*packets.Puback).PacketID
				freeID = true
				isRemove = true
			case *packets.Pubrec: //QOS2
				if el.Packet.Qos != packets.QOS_2 && el.Step != 0 {
					continue
				}
				pid = packet.(*packets.Pubrec).PacketID
			case *packets.Pubcomp: //QOS2
				if el.Packet.Qos != packets.QOS_2 && el.Step != 1 {
					continue
				}
				freeID = true //[MQTT-4.3.3-1]. 一旦发送者收到PUBCOMP报文，这个报文标识符就可以重用。
				isRemove = true
				pid = packet.(*packets.Pubcomp).PacketID
			}
			if pid == el.Pid {
				if isRemove {
					s.inflight.Remove(e)
					if log != nil {
						log.Printf("%-15s[%s],packet: %s ", "inflight msg released ", client.ClientOptions().ClientID, packet)
					}
					publish := client.msgDequeue()
					if publish != nil {
						elem := &InflightElem{
							At:     time.Now(),
							Pid:    publish.PacketID,
							Packet: publish,
							Step:   0,
						}
						s.inflight.PushBack(elem)
						client.out <- publish
					} else if client.server.Monitor != nil {
						client.server.Monitor.delInflight(client.opts.ClientID)

					}
				} else {
					el.Step++
				}
				if freeID {
					s.freePacketID(pid)
				}
				return
			}
		}
	}

}

func (s *session) freePacketID(id packets.PacketID) {
	s.pidMu.Lock()
	defer s.pidMu.Unlock()
	s.lockedPid[id] = false
}

func (s *session) setPacketID(id packets.PacketID) {
	s.pidMu.Lock()
	defer s.pidMu.Unlock()
	s.lockedPid[id] = true
}

func (s *session) getPacketID() packets.PacketID {
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
