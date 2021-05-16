package mem

import (
	"container/list"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/DrmagicE/gmqtt/persistence/queue"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/DrmagicE/gmqtt/server"
)

var _ queue.Store = (*Queue)(nil)

type Options struct {
	MaxQueuedMsg    int
	InflightExpiry  time.Duration
	ClientID        string
	DefaultNotifier queue.Notifier
}

type Queue struct {
	cond           *sync.Cond
	clientID       string
	version        packets.Version
	opts           *Options
	readBytesLimit uint32
	l              *list.List
	// current is the next element to read.
	current         *list.Element
	inflightDrained bool
	closed          bool
	// max is the maximum queue length
	max            int
	log            *zap.Logger
	inflightExpiry time.Duration
	notifier       queue.Notifier
}

func New(opts Options) (*Queue, error) {
	return &Queue{
		clientID:       opts.ClientID,
		cond:           sync.NewCond(&sync.Mutex{}),
		l:              list.New(),
		max:            opts.MaxQueuedMsg,
		inflightExpiry: opts.InflightExpiry,
		notifier:       opts.DefaultNotifier,
		log:            server.LoggerWithField(zap.String("queue", "memory")),
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
	q.notifier = opts.Notifier
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
			if dropErr == queue.ErrDropExpiredInflight {
				q.notifier.NotifyInflightAdded(-1)
			}
			if dropElem == nil {
				q.notifier.NotifyDropped(elem, dropErr)
				return
			}
			if dropElem == q.current {
				q.current = q.current.Next()
			}
			q.l.Remove(dropElem)
			q.notifier.NotifyDropped(dropElem.Value.(*queue.Elem), dropErr)
		} else {
			q.notifier.NotifyMsgQueueAdded(1)
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

		// drop expired inflight message
		if v := q.l.Front(); v != q.current &&
			v != nil &&
			queue.ElemExpiry(now, v.Value.(*queue.Elem)) {
			dropElem = v
			dropErr = queue.ErrDropExpiredInflight
			return
		}

		// drop the current elem if there is no more non-inflight messages.
		if q.inflightDrained && q.current == nil {
			return
		}
		for e := q.current; e != nil; e = e.Next() {
			pub := e.Value.(*queue.Elem).MessageWithID.(*queue.Publish)
			// drop expired non-inflight message
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
		// the messages in the queue are all inflight messages, drop the current elem
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
	var msgQueueDelta, inflightDelta int
	var pflag int
	for i := 0; i < length && q.current != nil; i++ {
		v := q.current
		// remove expired message
		if queue.ElemExpiry(now, v.Value.(*queue.Elem)) {
			q.current = q.current.Next()
			q.notifier.NotifyDropped(v.Value.(*queue.Elem), queue.ErrDropExpired)
			q.l.Remove(v)
			msgQueueDelta--
			continue
		}
		// remove message which exceeds maximum packet size
		pub := v.Value.(*queue.Elem).MessageWithID.(*queue.Publish)
		if size := pub.TotalBytes(q.version); size > q.readBytesLimit {
			q.current = q.current.Next()
			q.notifier.NotifyDropped(v.Value.(*queue.Elem), queue.ErrDropExceedsMaxPacketSize)
			q.l.Remove(v)
			msgQueueDelta--
			continue
		}

		// remove qos 0 message after read
		if pub.QoS == 0 {
			q.current = q.current.Next()
			q.l.Remove(v)
			msgQueueDelta--
		} else {
			pub.SetID(pids[pflag])
			// When the message becomes inflight message, update the expiry time.
			if q.inflightExpiry != 0 {
				v.Value.(*queue.Elem).Expiry = now.Add(q.inflightExpiry)
			}
			pflag++
			inflightDelta++
			q.current = q.current.Next()
		}
		rs = append(rs, v.Value.(*queue.Elem))
	}
	q.notifier.NotifyMsgQueueAdded(msgQueueDelta)
	q.notifier.NotifyInflightAdded(inflightDelta)
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
		if e := q.current.Value.(*queue.Elem); e.ID() != 0 {
			if q.inflightExpiry != 0 {
				e.Expiry = time.Now().Add(q.inflightExpiry)
			}
			rs = append(rs, e)
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
	// Must not remove unread messages.
	unread := q.current
	for e := q.l.Front(); e != nil && e != unread; e = e.Next() {
		if e.Value.(*queue.Elem).ID() == pid {
			q.l.Remove(e)
			q.notifier.NotifyMsgQueueAdded(-1)
			q.notifier.NotifyInflightAdded(-1)
			return nil
		}
	}
	return nil
}
