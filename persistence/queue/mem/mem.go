package mem

import (
	"container/list"
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/DrmagicE/gmqtt/persistence/queue"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/DrmagicE/gmqtt/server"
)

func New(config server.Config, client server.Client, dropped server.OnMsgDropped) (*Queue, error) {
	return &Queue{
		client:       client,
		cond:         sync.NewCond(&sync.Mutex{}),
		l:            list.New(),
		max:          config.MaxQueuedMsg,
		onMsgDropped: dropped,
		log:          server.LoggerWithField(zap.String("queue", "memory")),
	}, nil
}

type Queue struct {
	cond            *sync.Cond
	l               *list.List
	current         *list.Element
	inflightDrained bool
	closed          bool
	max             int
	log             *zap.Logger
	onMsgDropped    server.OnMsgDropped
	client          server.Client
}

func (m *Queue) Close() error {
	m.cond.L.Lock()
	defer m.cond.L.Unlock()
	m.closed = true
	m.cond.Signal()
	return nil
}

func (m *Queue) Init(cleanStart bool) error {
	m.cond.L.Lock()
	defer m.cond.L.Unlock()
	m.closed = false
	m.inflightDrained = false
	if cleanStart {
		m.l = list.New()
	}
	m.current = m.l.Front()
	m.cond.Signal()
	return nil
}

func (*Queue) Clean() error {
	return nil
}

func (m *Queue) Add(elem *queue.Elem) (err error) {
	now := time.Now()
	m.cond.L.Lock()
	var dropElem *list.Element
	var drop bool
	defer func() {
		if drop {
			m.log.Warn("message queue is full, drop message",
				zap.String("clientID", m.client.ClientOptions().ClientID),
			)
			if dropElem == nil {
				if m.onMsgDropped != nil {
					m.onMsgDropped(context.Background(), m.client, elem.MessageWithID.(*queue.Publish).Message)
				}
				m.cond.L.Unlock()
				m.cond.Signal()
				return
			} else {
				if dropElem == m.current {
					m.current = m.current.Next()
				}
				m.l.Remove(dropElem)
			}
			if m.onMsgDropped != nil {
				m.onMsgDropped(context.Background(), m.client, dropElem.Value.(*queue.Elem).MessageWithID.(*queue.Publish).Message)
			}
		}

		e := m.l.PushBack(elem)
		if m.current == nil {
			m.current = e
		}

		m.cond.L.Unlock()
		m.cond.Signal()
	}()
	if m.l.Len() >= m.max {
		// drop the current elem if there is no more non-inflight messages.
		if m.current == nil {
			drop = true
			return
		}
		// drop expired message
		for e := m.current; e != nil; e = e.Next() {
			if e.Value.(*queue.Elem).MessageWithID.ID() == 0 &&
				queue.ElemExpiry(now, e.Value.(*queue.Elem)) {
				dropElem = e
				drop = true
				return
			}
		}
		// drop qos0 message in the queue
		for e := m.current; e != nil; e = e.Next() {
			if e.Value.(*queue.Elem).MessageWithID.ID() == 0 {
				if e.Value.(*queue.Elem).MessageWithID.(*queue.Publish).QoS == packets.Qos0 {
					dropElem = e
					drop = true
					return
				}
			}
		}
		if elem.MessageWithID.(*queue.Publish).QoS == packets.Qos0 {
			drop = true
			return
		}
		// drop the front message
		dropElem = m.current
		drop = true
		return
	}
	return nil
}

func (m *Queue) Replace(elem *queue.Elem) (replaced bool, err error) {
	m.cond.L.Lock()
	defer m.cond.L.Unlock()
	unread := m.current
	for e := m.l.Front(); e != nil && e != unread; e = e.Next() {
		if e.Value.(*queue.Elem).ID() == elem.ID() {
			e.Value = elem
			return true, nil
		}
	}
	return false, nil
}

func (m *Queue) Read(pids []packets.PacketID) (rs []*queue.Elem, err error) {
	m.cond.L.Lock()
	defer m.cond.L.Unlock()
	if !m.inflightDrained {
		panic("must call ReadInflight to drain all inflight messages before Read")
	}
	for (m.l.Len() == 0 || m.current == nil) && !m.closed {
		m.cond.Wait()
	}
	if m.closed {
		return nil, queue.ErrClosed
	}
	length := m.l.Len()
	if len(pids) < length {
		length = len(pids)
	}
	var pflag int
	for i := 0; i < length && m.current != nil; i++ {
		v := m.current
		// remove qos 0 message after read
		if m.current.Value.(*queue.Elem).MessageWithID.(*queue.Publish).QoS == 0 {
			m.current = m.current.Next()
			m.l.Remove(v)
		} else {
			v.Value.(*queue.Elem).MessageWithID.SetID(pids[pflag])
			pflag++
			m.current = m.current.Next()
		}
		rs = append(rs, v.Value.(*queue.Elem))
	}
	return rs, nil
}

func (m *Queue) ReadInflight(maxSize uint) (rs []*queue.Elem, err error) {
	m.cond.L.Lock()
	defer m.cond.L.Unlock()
	length := m.l.Len()
	if length == 0 || m.current == nil {
		m.inflightDrained = true
		return nil, nil
	}
	if int(maxSize) < length {
		length = int(maxSize)
	}
	for i := 0; i < length && m.current != nil; i++ {
		if m.current.Value.(*queue.Elem).ID() != 0 {
			rs = append(rs, m.current.Value.(*queue.Elem))
			m.current = m.current.Next()
		} else {
			m.inflightDrained = true
			break
		}
	}
	return rs, nil
}

func (m *Queue) Remove(pid packets.PacketID) error {
	m.cond.L.Lock()
	defer m.cond.L.Unlock()
	// 不允许删除还没读过的元素
	unread := m.current
	for e := m.l.Front(); e != nil && e != unread; e = e.Next() {
		if e.Value.(*queue.Elem).ID() == pid {
			m.l.Remove(e)
			return nil
		}
	}
	return nil
}
