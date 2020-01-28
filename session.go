package gmqtt

import (
	"container/list"
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/DrmagicE/gmqtt/pkg/packets"
)

type session struct {
	inflightMu sync.Mutex //gard inflight
	inflight   *list.List //传输中等待确认的报文

	awaitRelMu sync.Mutex // gard awaitRel
	awaitRel   *list.List // 未确认的awaitRel报文

	msgQueueMu sync.Mutex //gard msgQueue
	msgQueue   *list.List //缓存数据，缓存publish报文

	//QOS=2 的情况下，判断报文是否是客户端重发报文，如果重发，则不分发.
	// 确保[MQTT-4.3.3-2]中：在收发送PUBREC报文确认任何到对应的PUBREL报文之前，接收者必须后续的具有相同标识符的PUBLISH报文。
	// 在这种情况下，它不能重复分发消息给任何后续的接收者
	unackpublish map[packets.PacketID]bool //[MQTT-4.3.3-2]
	pidMu        sync.RWMutex              //gard lockedPid & freeID
	lockedPid    map[packets.PacketID]bool //Pid inuse
	freePid      packets.PacketID          //下一个可以使用的freeID

	config *Config
}

//inflightElem is the element type in inflight queue
type inflightElem struct {
	//at is the entry time
	at time.Time
	//packet represents Publish packet
	packet *packets.Publish
}

//awaitRelElem is the element type in awaitRel queue
type awaitRelElem struct {
	//at is the entry time
	at time.Time
	//pid is the packetID
	pid packets.PacketID
}

//setAwaitRel 入队,
func (client *client) setAwaitRel(pid packets.PacketID) {
	s := client.session
	s.awaitRelMu.Lock()
	defer s.awaitRelMu.Unlock()
	elem := &awaitRelElem{
		at:  time.Now(),
		pid: pid,
	}
	if s.awaitRel.Len() >= s.config.MaxAwaitRel && s.config.MaxAwaitRel != 0 { //加入缓存队列
		removeMsg := s.awaitRel.Front()
		s.awaitRel.Remove(removeMsg)
		zaplog.Info("awaitRel window is full, removing the front elem",
			zap.String("clientID", client.opts.clientID),
			zap.Int16("pid", int16(pid)))
	} else {
		client.statsManager.addAwaitCurrent(1)
	}
	s.awaitRel.PushBack(elem)

}

//unsetAwaitRel
func (client *client) unsetAwaitRel(pid packets.PacketID) {
	s := client.session
	s.awaitRelMu.Lock()
	defer s.awaitRelMu.Unlock()
	for e := s.awaitRel.Front(); e != nil; e = e.Next() {
		if el, ok := e.Value.(*awaitRelElem); ok {
			if el.pid == pid {
				s.awaitRel.Remove(e)
				client.statsManager.decAwaitCurrent(1)
				s.freePacketID(pid)
				return
			}
		}
	}
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
func (client *client) msgEnQueue(publish *packets.Publish) {
	s := client.session
	srv := client.server
	s.msgQueueMu.Lock()
	defer s.msgQueueMu.Unlock()
	if s.msgQueue.Len() >= s.config.MaxMsgQueue && s.config.MaxMsgQueue != 0 {
		var removeMsg *list.Element
		// onMessageDropped hook
		if srv.hooks.OnMsgDropped != nil {
			defer func() {
				cs := context.Background()
				if removeMsg != nil {
					srv.hooks.OnMsgDropped(cs, client, messageFromPublish(removeMsg.Value.(*packets.Publish)))
				} else {
					srv.hooks.OnMsgDropped(cs, client, messageFromPublish(publish))
				}
			}()
		}
		for e := s.msgQueue.Front(); e != nil; e = e.Next() {
			if pub, ok := e.Value.(*packets.Publish); ok {
				if pub.Qos == packets.QOS_0 {
					removeMsg = e
					break
				}
			}
		}
		if removeMsg != nil { //case1: removing qos0 message in the msgQueue
			zaplog.Info("message queue is full, removing msg",
				zap.String("clientID", client.opts.clientID),
				zap.String("type", "QOS_0 in queue"),
				zap.String("packet", removeMsg.Value.(packets.Packet).String()),
			)
			s.msgQueue.Remove(removeMsg)
			client.server.statsManager.messageDropped(0)
			client.statsManager.messageDropped(0)
		} else if publish.Qos == packets.QOS_0 { //case2: removing qos0 message that is going to enqueue
			zaplog.Info("message queue is full, removing msg",
				zap.String("clientID", client.opts.clientID),
				zap.String("type", "QOS_0 enqueue"),
				zap.String("packet", publish.String()),
			)
			client.server.statsManager.messageDropped(0)
			client.statsManager.messageDropped(0)
			return
		} else { //case3: removing the front message of msgQueue
			removeMsg = s.msgQueue.Front()
			s.msgQueue.Remove(removeMsg)
			zaplog.Info("message queue is full, removing msg",
				zap.String("clientID", client.opts.clientID),
				zap.String("type", "front"),
				zap.String("packet", removeMsg.Value.(packets.Packet).String()),
			)
			client.server.statsManager.messageDropped(removeMsg.Value.(*packets.Publish).Qos)
			client.statsManager.messageDropped(removeMsg.Value.(*packets.Publish).Qos)
		}
	} else {
		client.server.statsManager.messageEnqueue(1)
		client.statsManager.messageEnqueue(1)
	}
	s.msgQueue.PushBack(publish)
}

func (client *client) msgDequeue() *packets.Publish {
	s := client.session
	s.msgQueueMu.Lock()
	defer s.msgQueueMu.Unlock()

	if s.msgQueue.Len() > 0 {
		queueElem := s.msgQueue.Front()
		zaplog.Debug("msg dequeued",
			zap.String("clientID", client.opts.clientID),
			zap.String("packet", queueElem.Value.(*packets.Publish).String()))

		s.msgQueue.Remove(queueElem)
		client.statsManager.messageDequeue(1)
		client.server.statsManager.messageDequeue(1)
		return queueElem.Value.(*packets.Publish)
	}
	return nil

}

//inflight 入队,inflight队列满，放入缓存队列，缓存队列满，删除最早进入缓存队列的内容
func (client *client) setInflight(publish *packets.Publish) (enqueue bool) {
	s := client.session
	s.inflightMu.Lock()
	defer func() {
		s.inflightMu.Unlock()
		if enqueue {
			client.statsManager.addInflightCurrent(1)
		}
	}()
	elem := &inflightElem{
		at:     time.Now(),
		packet: publish,
	}
	if s.inflight.Len() >= s.config.MaxInflight && s.config.MaxInflight != 0 { //加入缓存队列
		zaplog.Info("inflight window full, saving msg into msgQueue",
			zap.String("clientID", client.opts.clientID),
			zap.String("packet", elem.packet.String()),
		)
		client.msgEnQueue(publish)
		enqueue = false
		return
	}
	zaplog.Debug("set inflight", zap.String("clientID", client.opts.clientID), zap.String("packet", elem.packet.String()))
	s.inflight.PushBack(elem)
	enqueue = true
	return
}

//unsetInflight 出队
//packet: puback(QOS1),pubrec(QOS2)  or pubcomp(QOS2)
func (client *client) unsetInflight(packet packets.Packet) {
	s := client.session
	srv := client.server
	s.inflightMu.Lock()
	defer s.inflightMu.Unlock()
	var freeID bool
	var pid packets.PacketID
	for e := s.inflight.Front(); e != nil; e = e.Next() {
		if el, ok := e.Value.(*inflightElem); ok {
			switch packet.(type) {
			case *packets.Puback: //QOS1
				if el.packet.Qos != packets.QOS_1 {
					continue
				}
				pid = packet.(*packets.Puback).PacketID
				freeID = true
			case *packets.Pubrec: //QOS2
				if el.packet.Qos != packets.QOS_2 {
					continue
				}
				pid = packet.(*packets.Pubrec).PacketID
			}
			if el.packet.PacketID == pid {
				s.inflight.Remove(e)
				client.statsManager.decInflightCurrent(1)
				zaplog.Debug("unset inflight", zap.String("clientID", client.opts.clientID),
					zap.String("packet", packet.String()),
				)
				if freeID {
					s.freePacketID(pid)
				}
				// onAcked hook
				if srv.hooks.OnAcked != nil {
					srv.hooks.OnAcked(context.Background(), client, messageFromPublish(e.Value.(*inflightElem).packet))
				}
				publish := client.msgDequeue()
				if publish != nil {
					elem := &inflightElem{
						at:     time.Now(),
						packet: publish,
					}
					s.inflight.PushBack(elem)
					client.sendMsg(publish)
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
