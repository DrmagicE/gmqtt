package mem

import (
	"container/list"
	"errors"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/DrmagicE/gmqtt/persistence/queue"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/DrmagicE/gmqtt/server"
)

var _ queue.Store = (*Queue)(nil)

type Options struct {
	MaxQueuedMsg int
	ClientID     string
	DropHandler  server.OnMsgDropped
}

type Queue struct {
	cond            *sync.Cond
	clientID        string
	version         packets.Version
	readBytesLimit  uint32
	l               *list.List
	current         *list.Element
	inflightDrained bool
	closed          bool
	max             int
	log             *zap.Logger
	onMsgDropped    queue.OnMsgDropped
}

func New(opts Options) (*Queue, error) {
	return &Queue{
		clientID:     opts.ClientID,
		cond:         sync.NewCond(&sync.Mutex{}),
		l:            list.New(),
		max:          opts.MaxQueuedMsg,
		onMsgDropped: opts.DropHandler,
		log:          server.LoggerWithField(zap.String("queue", "memory")),
	}, nil
}

func (q *Queue) Close() error {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	q.closed = true
	q.cond.Signal()
	return nil
}

func (q *Queue) Init(opts *queue.InitOptions) error {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	q.closed = false
	q.inflightDrained = false
	if opts.CleanStart {
		q.l = list.New()
	}
	q.readBytesLimit = opts.ReadBytesLimit
	q.version = opts.Version
	q.current = q.l.Front()
	q.cond.Signal()
	return nil
}

func (*Queue) Clean() error {
	return nil
}

func (q *Queue) Add(elem *queue.Elem) (err error) {
	now := time.Now()
	var dropErr error
	var dropElem *list.Element
	var drop bool
	q.cond.L.Lock()
	defer func() {
		q.cond.L.Unlock()
		q.cond.Signal()
	}()
	defer func() {
		if drop {
			if dropElem == nil {
				queue.Drop(q.onMsgDropped, q.log, q.clientID, elem.MessageWithID.(*queue.Publish).Message, dropErr)
				return
			} else {
				if dropElem == q.current {
					q.current = q.current.Next()
				}
				q.l.Remove(dropElem)
			}
			queue.Drop(q.onMsgDropped, q.log, q.clientID, dropElem.Value.(*queue.Elem).MessageWithID.(*queue.Publish).Message, dropErr)
		}
		e := q.l.PushBack(elem)
		if q.current == nil {
			q.current = e
		}
	}()
	if q.l.Len() >= q.max {
		// set default drop error
		dropErr = queue.ErrDropQueueFull
		drop = true
		// drop the current elem if there is no more non-inflight messages.
		if q.inflightDrained && q.current == nil {
			return
		}
		for e := q.current; e != nil; e = e.Next() {
			pub := e.Value.(*queue.Elem).MessageWithID.(*queue.Publish)
			// drop expired message
			if pub.ID() == 0 &&
				queue.ElemExpiry(now, e.Value.(*queue.Elem)) {
				dropElem = e
				dropErr = queue.ErrDropExpired
				return
			}
			// drop qos0 message in the queue
			if pub.ID() == 0 && pub.QoS == packets.Qos0 && dropElem == nil {
				dropElem = e
			}
		}
		if dropElem != nil {
			return
		}
		if elem.MessageWithID.(*queue.Publish).QoS == packets.Qos0 {
			return
		}
		if q.inflightDrained {
			// drop the front message
			dropElem = q.current
			return
		}

		// the the messages in the queue are all inflight messages, drop the current elem

		return
	}
	return nil
}

func (q *Queue) Replace(elem *queue.Elem) (replaced bool, err error) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	unread := q.current
	for e := q.l.Front(); e != nil && e != unread; e = e.Next() {
		if e.Value.(*queue.Elem).ID() == elem.ID() {
			e.Value = elem
			return true, nil
		}
	}
	return false, nil
}

func (q *Queue) Read(pids []packets.PacketID) (rs []*queue.Elem, err error) {
	now := time.Now()
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	if !q.inflightDrained {
		panic("must call ReadInflight to drain all inflight messages before Read")
	}
	for (q.l.Len() == 0 || q.current == nil) && !q.closed {
		q.cond.Wait()
	}
	if q.closed {
		return nil, queue.ErrClosed
	}
	length := q.l.Len()
	if len(pids) < length {
		length = len(pids)
	}
	var pflag int
	for i := 0; i < length && q.current != nil; i++ {
		v := q.current
		// remove expired message
		if queue.ElemExpiry(now, v.Value.(*queue.Elem)) {
			q.current = q.current.Next()
			queue.Drop(q.onMsgDropped, q.log, q.clientID, v.Value.(*queue.Elem).MessageWithID.(*queue.Publish).Message, queue.ErrDropExpired)
			q.l.Remove(v)
			continue
		}
		// remove message which exceeds maximum packet size
		pub := v.Value.(*queue.Elem).MessageWithID.(*queue.Publish)
		if size := pub.TotalBytes(q.version); size > q.readBytesLimit {
			q.current = q.current.Next()
			q.l.Remove(v)
			queue.Drop(q.onMsgDropped, q.log, q.clientID, pub.Message, queue.ErrDropExceedsMaxPacketSize)
			continue
		}

		// remove qos 0 message after read
		if pub.QoS == 0 {
			q.current = q.current.Next()
			q.l.Remove(v)
		} else {
			pub.SetID(pids[pflag])
			pflag++
			q.current = q.current.Next()
		}
		rs = append(rs, v.Value.(*queue.Elem))
	}
	return rs, nil
}

func (q *Queue) ReadInflight(maxSize uint) (rs []*queue.Elem, err error) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	length := q.l.Len()
	if length == 0 || q.current == nil {
		q.inflightDrained = true
		return nil, nil
	}
	if int(maxSize) < length {
		length = int(maxSize)
	}
	for i := 0; i < length && q.current != nil; i++ {
		if q.current.Value.(*queue.Elem).ID() != 0 {
			rs = append(rs, q.current.Value.(*queue.Elem))
			q.current = q.current.Next()
		} else {
			q.inflightDrained = true
			break
		}
	}
	return rs, nil
}

func (q *Queue) Remove(pid packets.PacketID) error {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	// 不允许删除还没读过的元素
	unread := q.current
	for e := q.l.Front(); e != nil && e != unread; e = e.Next() {
		if e.Value.(*queue.Elem).ID() == pid {
			q.l.Remove(e)
			return nil
		}
	}
	return nil
}

//GetMessageIDAndTopic 通过PacketID 获取消息的MessageID和Topic
func (q *Queue) GetMessageIDAndTopic(pid packets.PacketID) (messageID []byte, topic string, err error) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	for e := q.l.Front(); e != nil; e = e.Next() {
		if e.Value.(*queue.Elem).ID() == pid {
			pub := e.Value.(*queue.Elem).MessageWithID.(*queue.Publish)
			return pub.MessageID, pub.Topic, nil
		}
	}
	return []byte{}, "", errors.New("NoElement")
}
