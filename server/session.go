package server

import (
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/pkg/packets"
)

//type queueListElem struct {
//	id uint64
//	q  *QueueElem
//}
//type messageQueue struct {
//	nextID uint64
//	l      *list.List
//	index  map[uint64]*list.Element
//	store  QueueStore
//}
//func (m *messageQueue) init() (err error) {
//	// load data from persistence backend
//	if m.store != nil {
//		err = m.store.Init(func(id uint64, q *QueueElem) bool {
//			m.index[id] = m.l.PushBack(&queueListElem{
//				id: id,
//				q:  q,
//			})
//			return true
//		})
//	}
//	return
//}
//func (m *messageQueue) len() int {
//	return m.l.Len()
//}
//func (m *messageQueue) pushBack(elem *QueueElem) (qle *queueListElem, err error) {
//	id := m.nextID
//	qle = &queueListElem{
//		id: id,
//		q:  elem,
//	}
//	e := m.l.PushBack(qle)
//	m.index[id] = e
//	if m.store != nil {
//		err = m.store.PushBack(id, elem)
//	}
//	m.nextID++
//	return
//}
//func (m *messageQueue) remove(id uint64) (err error) {
//	if m.store != nil {
//		if err = m.store.Remove(id); err != nil {
//			return err
//		}
//	}
//	e := m.index[id]
//	if e != nil {
//		m.l.Remove(e)
//	}
//	return nil
//}
//func (m *messageQueue) iterate(fn func(q *queueListElem) bool) {
//	for e := m.l.Front(); e != nil; e = e.Next() {
//		q := e.Value.(*queueListElem)
//		if !fn(q) {
//			return
//		}
//	}
//}
//func (m *messageQueue) front() (e *queueListElem) {
//	return m.l.Front().Value.(*queueListElem)
//}

type session struct {

	//QOS=2 的情况下，判断报文是否是客户端重发报文，如果重发，则不分发.
	// 确保[MQTT-4.3.3-2]中：在收发送PUBREC报文确认任何到对应的PUBREL报文之前，接收者必须后续的具有相同标识符的PUBLISH报文。
	// 在这种情况下，它不能重复分发消息给任何后续的接收者
	unackpublish map[packets.PacketID]bool //[MQTT-4.3.3-2]
	pidMu        sync.RWMutex              //gard lockedPid & freeID
	lockedPid    map[packets.PacketID]bool //Pid inuse
	freePid      packets.PacketID          //下一个可以使用的freeID
}

//awaitRelElem is the element type in awaitRel queue
type awaitRelElem struct {
	//At is the entry time
	at time.Time
	//pid is the packetID
	pid packets.PacketID
}

type QueueElem struct {
	// 入队时间
	At      time.Time
	Message *gmqtt.Message
}

func (client *client) isQueueElemExpiry(now time.Time, elem *QueueElem) bool {
	if client.version == packets.Version5 && elem.Message.MessageExpiry != 0 {
		expiry := time.Duration(elem.Message.MessageExpiry)
		if now.Add(-expiry * time.Second).After(elem.At) {
			return true
		}
	}
	return false
}

//setAwaitRel 入队,
//func (client *client) setAwaitRel(pid packets.PacketID) {
//	s := client.session
//	s.awaitRelMu.Lock()
//	defer s.awaitRelMu.Unlock()
//	elem := &awaitRelElem{
//		at:  time.Now(),
//		pid: pid,
//	}
//	if s.awaitRel.Len() >= client.config.MaxAwaitRel && client.config.MaxAwaitRel != 0 { //加入缓存队列
//		removeMsg := s.awaitRel.Front()
//		s.awaitRel.Remove(removeMsg)
//		zaplog.Info("awaitRel window is full, removing the front elem",
//			zap.String("clientID", client.opts.ClientID),
//			zap.Int16("pid", int16(pid)))
//	} else {
//		client.statsManager.addAwaitCurrent(1)
//	}
//	s.awaitRel.PushBack(elem)
//
//}
//
////unsetAwaitRel
//func (client *client) unsetAwaitRel(pid packets.PacketID) {
//	s := client.session
//	s.awaitRelMu.Lock()
//	defer s.awaitRelMu.Unlock()
//	for e := s.awaitRel.Front(); e != nil; e = e.Next() {
//		if el, ok := e.Value.(*awaitRelElem); ok {
//			if el.pid == pid {
//				s.awaitRel.Remove(e)
//				client.statsManager.decAwaitCurrent(1)
//				s.freePacketID(pid)
//				return
//			}
//		}
//	}
//}

//当入队发现缓存队列满的时候：
//按照以下优先级丢弃一个publish报文
//1.缓存队列中QOS0的报文
//2.如果准备入队的报文qos=0,丢弃
//3.丢弃最先进入缓存队列的报文

//When the len of msgQueueu is reaching the maximum setting, message will be dropped according to the following priorities：
//1. expired message in the msgQueue (v5 only)
//2. qos0 message in the msgQueue
//3. qos0 message that is going to enqueue
//4. the front message of msgQueue
//func (client *client) msgEnQueue(msg *gmqtt.Message) (err error) {
//	// 保存进入缓存队列的时间，
//	s := client.session
//	srv := client.server
//	s.msgQueueMu.Lock()
//	defer s.msgQueueMu.Unlock()
//	now := time.Now()
//	if s.msgQueue.Len() >= client.config.MaxQueuedMsg && client.config.MaxQueuedMsg != 0 {
//		var removeID uint64
//		var removeMsg *gmqtt.Message
//		var removeElem *QueueElem
//		// onMessageDropped hook
//
//		defer func() {
//			cs := context.Background()
//			if removeElem != nil {
//				err = s.msgQueue.Remove(removeID)
//			}
//			if removeMsg != nil {
//				if srv.hooks.OnMsgDropped != nil {
//					srv.hooks.OnMsgDropped(cs, client, removeMsg)
//				}
//				client.server.statsManager.messageDropped(removeMsg.QoS)
//				client.statsManager.messageDropped(removeMsg.QoS)
//			}
//		}()
//		var front *QueueElem
//		err = s.msgQueue.Iterate(func(id uint64, q *QueueElem) bool {
//			front = q
//			//case1:  expired message in the msgQueue (v5 only)
//			if client.isQueueElemExpiry(now, q) {
//				removeElem = q
//				removeMsg = q.Message
//				zaplog.Warn("message queue is full, removing message",
//					zap.String("clientID", client.opts.ClientID),
//					zap.String("type", "expired message"),
//				)
//				return false
//			}
//			//case2: removing qos0 message in the msgQueue
//			if q.Message.QoS == packets.Qos0 {
//				removeElem = q
//				removeMsg = q.Message
//				zaplog.Warn("message queue is full, removing message",
//					zap.String("clientID", client.opts.ClientID),
//					zap.String("type", "Qos0 message"),
//				)
//				return false
//			}
//			return true
//		})
//		if err != nil {
//			return err
//		}
//
//		if removeMsg == nil {
//			if msg.QoS == packets.Qos0 { //case2: removing qos0 message that is going to enqueue
//				zaplog.Warn("message queue is full, removing message",
//					zap.String("clientID", client.opts.ClientID),
//					zap.String("type", "Qos0 message"),
//				)
//				removeMsg = msg
//				return
//			} else { //case3: removing the front message of msgQueue
//				removeElem = front
//				removeMsg = removeElem.Message
//				zaplog.Warn("message queue is full, removing message",
//					zap.String("clientID", client.opts.ClientID),
//					zap.String("type", "front"),
//				)
//			}
//		}
//	} else {
//		client.server.statsManager.messageEnqueue(1)
//		client.statsManager.messageEnqueue(1)
//	}
//	elem := &QueueElem{
//		At:      time.Now(),
//		Message: msg,
//	}
//	_, err = s.msgQueue.PushBack(elem)
//	return
//}

//func (client *client) msgDequeue() (msg *gmqtt.Message, err error) {
//	s := client.session
//	s.msgQueueMu.Lock()
//	defer s.msgQueueMu.Unlock()
//
//	if s.msgQueue.Len() > 0 {
//		// remove the first element
//		err = s.msgQueue.Iterate(func(id uint64, q *QueueElem) bool {
//			if ce := zaplog.Check(zapcore.DebugLevel, "unset inflight"); ce != nil {
//				ce.Write(
//					zap.String("clientID", client.opts.ClientID),
//					//zap.String("in", qle.q.Publish.String()),
//				)
//			}
//			err = s.msgQueue.Remove(id)
//			client.statsManager.messageDequeue(1)
//			client.server.statsManager.messageDequeue(1)
//			return false
//		})
//		if err != nil {
//			return &gmqtt.Message{}, err
//		}
//
//	}
//	return nil, nil
//
//}
//
////inflight 入队,inflight队列满，放入缓存队列，缓存队列满，删除最早进入缓存队列的内容
////进到此方法的 publish.packetID 已经重新设置
//func (client *client) setInflight(msg *gmqtt.Message) (enqueue bool, err error) {
//	s := client.session
//	s.inflightMu.Lock()
//	defer func() {
//		s.inflightMu.Unlock()
//		if enqueue {
//			client.statsManager.addInflightCurrent(1)
//		}
//	}()
//	if client.version == packets.Version5 {
//		// MQTT v5 Receive Maximum
//		if ok := client.tryDecClientQuota(); !ok {
//			zaplog.Debug("reach client quota",
//				zap.String("client_id", client.opts.ClientID),
//				zap.String("remote_addr", client.rwc.RemoteAddr().String()),
//			)
//
//			err = client.msgEnQueue(msg)
//			return
//		}
//	}
//	elem := &InflightEle{
//
//		At:      time.Now(),
//		Message: msg,
//	}
//	if s.inflight.Len() >= client.config.MaxInflight && client.config.MaxInflight != 0 { //加入缓存队列
//		zaplog.Info("inflight window full, saving message into message queue",
//			zap.String("client_id", client.opts.ClientID),
//			zap.String("remote_addr", client.rwc.RemoteAddr().String()),
//		)
//		err = client.msgEnQueue(msg)
//		return
//	}
//
//	if ce := zaplog.Check(zapcore.DebugLevel, "set inflight"); ce != nil {
//		ce.Write(
//			zap.String("client_id", client.opts.ClientID),
//			zap.Uint16("pid", msg.PacketID),
//		)
//	}
//	_, err = s.inflight.PushBack(elem)
//	if err != nil {
//		return false, err
//	}
//	enqueue = true
//	return
//}
//
////unsetInflight 出队
////in: puback(QOS1),pubrec(QOS2)  or pubcomp(QOS2)
//func (client *client) unsetInflight(packet packets.Packet) (err error) {
//	s := client.session
//	srv := client.server
//	s.inflightMu.Lock()
//	defer s.inflightMu.Unlock()
//	var freeID bool
//	var pid packets.PacketID
//	err = s.inflight.Iterate(func(id uint64, elem *InflightElem) bool {
//		switch packet.(type) {
//		case *packets.Puback: //QOS1
//			if elem.Message.QoS != packets.Qos1 {
//				// continue
//				return true
//			}
//			pid = packet.(*packets.Puback).PacketID
//			freeID = true
//		case *packets.Pubrec: //QOS2
//			if elem.Message.QoS != packets.Qos2 {
//				// continue
//				return true
//			}
//			pid = packet.(*packets.Pubrec).PacketID
//		}
//		if elem.Message.PacketID == pid {
//			err = s.inflight.Remove(id)
//			if err != nil {
//				return false
//			}
//			client.statsManager.decInflightCurrent(1)
//			if ce := zaplog.Check(zapcore.DebugLevel, "unset inflight"); ce != nil {
//				zaplog.Debug("unset inflight", zap.String("clientID", client.opts.ClientID),
//					zap.String("packet", packet.String()),
//				)
//			}
//			if freeID {
//				s.freePacketID(pid)
//			}
//			if srv.hooks.OnAcked != nil {
//				srv.hooks.OnAcked(context.Background(), client, elem.Message)
//			}
//			var msg *gmqtt.Message
//			msg, err = client.msgDequeue()
//			if err != nil {
//				return false
//			}
//			if msg != nil {
//				elem := &InflightElem{
//					At:      time.Now(),
//					Message: msg,
//				}
//				client.statsManager.addInflightCurrent(1)
//				_, err = s.inflight.PushBack(elem)
//				if err != nil {
//					return false
//				}
//				select {
//				case <-client.close:
//					return false
//				case client.out <- gmqtt.MessageToPublish(msg, client.version):
//				}
//			}
//		}
//		return true
//	})
//
//	return nil
//}

func (s *session) freePacketID(id packets.PacketID) {
	s.pidMu.Lock()
	defer s.pidMu.Unlock()
	s.lockedPid[id] = false
}

func (s *session) freePacketIDs(ids []packets.PacketID) {
	for _, v := range ids {
		s.lockedPid[v] = false
	}
}

func (s *session) setPacketID(id packets.PacketID) {
	s.pidMu.Lock()
	defer s.pidMu.Unlock()
	s.lockedPid[id] = true
}

func (s *session) getPacketID() packets.PacketID {
	for s.lockedPid[s.freePid] {
		s.freePid++
		if s.freePid > packets.MaxPacketID {
			s.freePid = packets.MinPacketID
		}
	}
	id := s.freePid
	s.freePid++
	if s.freePid > packets.MaxPacketID {
		s.freePid = packets.MinPacketID
	}
	return id
}

func (s *session) lockPacketIDs(fn func(id packets.PacketID, used *bool, cont *bool)) {
	s.pidMu.Lock()
	defer s.pidMu.Unlock()
	// continue
	cont := true
	for cont {
		for s.lockedPid[s.freePid] {
			s.freePid++
			if s.freePid > packets.MaxPacketID {
				s.freePid = packets.MinPacketID
			}
		}
		used := true
		fn(s.freePid, &used, &cont)
		if !used {
			continue
		}
		s.freePid++
		if s.freePid > packets.MaxPacketID {
			s.freePid = packets.MinPacketID
		}
	}
}

func (s *session) getPacketIDs(max uint16) (ids []packets.PacketID) {
	s.pidMu.Lock()
	defer s.pidMu.Unlock()
	for i := 0; i < int(max); i++ {
		for s.lockedPid[s.freePid] {
			s.freePid++
			if s.freePid > packets.MaxPacketID {
				s.freePid = packets.MinPacketID
			}
		}
		ids = append(ids, s.freePid)
		s.freePid++
		if s.freePid > packets.MaxPacketID {
			s.freePid = packets.MinPacketID
		}
	}
	return ids
}

func (client *client) isSessionExpiried(now time.Time) bool {
	return now.Add(-time.Duration(client.opts.SessionExpiry) * time.Second).After(time.Unix(client.connectedAt, 0))
}

// client 是旧的client
func (client *client) shouldResumeSession(newClient *client) bool {
	if newClient.opts.CleanStart {
		return false
	}
	if client.version == packets.Version311 && !client.opts.CleanStart {
		return true
	}
	if client.version == packets.Version5 && client.isSessionExpiried(time.Now()) {
		return false
	}
	return true
}

// packetIDLimiter limit the generation of packet id to keep the number of inflight messages always less or equal than receive maximum setting of the client.
type packetIDLimiter struct {
	cond   *sync.Cond
	used   uint16 // 当前用了多少ID
	limit  uint16 // 最多同时可以用多少个个ID
	exit   bool
	client *client

	lockedPid map[packets.PacketID]bool // packet id in-use
	freePid   packets.PacketID          //下一个可以使用的freeID
}

func (client *client) newPacketIDLimiter(limit uint16) {
	client.pl = &packetIDLimiter{
		cond:      sync.NewCond(&sync.Mutex{}),
		used:      0,
		limit:     limit,
		exit:      false,
		client:    client,
		freePid:   1,
		lockedPid: make(map[packets.PacketID]bool),
	}
}
func (p *packetIDLimiter) close() {
	p.cond.L.Lock()
	p.exit = true
	p.cond.L.Unlock()
	p.cond.Signal()
}

// pollPacketIDs returns at most max number of unused packetID and marks them as used for a client.
// If there is no available id, the call will be blocked until at least one packet id is available or the limiter has been closed.
// return 0 means the limiter is closed.
// the return number = min(max, i.used).
func (p *packetIDLimiter) pollPacketIDs(max uint16) (id []packets.PacketID) {
	p.cond.L.Lock()
	defer p.cond.L.Unlock()
	for p.used >= p.limit && !p.exit {
		p.cond.Wait()
	}
	if p.exit {
		return nil
	}
	n := max
	if remain := p.limit - p.used; remain < max {
		n = remain
	}
	for j := uint16(0); j < n; j++ {
		for p.lockedPid[p.freePid] {
			p.freePid++
			if p.freePid > packets.MaxPacketID {
				p.freePid = packets.MinPacketID
			}
		}
		id = append(id, p.freePid)
		p.used++
		p.lockedPid[p.freePid] = true
		p.freePid++
		if p.freePid > packets.MaxPacketID {
			p.freePid = packets.MinPacketID
		}
	}
	return id
}

// release marks the given id list as unused
func (p *packetIDLimiter) release(id packets.PacketID) {
	p.cond.L.Lock()
	p.releaseLocked(id)
	p.cond.L.Unlock()
	p.cond.Signal()
}
func (p *packetIDLimiter) releaseLocked(id packets.PacketID) {
	if p.lockedPid[id] {
		p.lockedPid[id] = false
		p.used--
	}
	zaplog.Debug("release client quota",
		zap.String("client_id", p.client.opts.ClientID),
		zap.Uint16("quota", p.used),
		zap.String("remote_addr", p.client.rwc.RemoteAddr().String()),
	)
}

func (p *packetIDLimiter) batchRelease(id []packets.PacketID) {
	p.cond.L.Lock()
	for _, v := range id {
		p.releaseLocked(v)
	}
	p.cond.L.Unlock()
	p.cond.Signal()

}

// markInUsed marks the given id as used.
func (p *packetIDLimiter) markUsedLocked(id packets.PacketID) {
	p.used++
	p.lockedPid[id] = true
}

func (p *packetIDLimiter) lock() {
	p.cond.L.Lock()
}
func (p *packetIDLimiter) unlock() {
	p.cond.L.Unlock()
}
func (p *packetIDLimiter) unlockAndSignal() {
	p.cond.L.Unlock()
	p.cond.Signal()
}
