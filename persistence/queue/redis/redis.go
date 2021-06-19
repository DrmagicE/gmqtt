package redis

import (
	"sync"
	"time"

	redigo "github.com/gomodule/redigo/redis"
	"go.uber.org/zap"

	"github.com/DrmagicE/gmqtt/pkg/codes"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/DrmagicE/gmqtt/server"

	"github.com/DrmagicE/gmqtt/persistence/queue"
)

const (
	queuePrefix = "queue:"
)

var _ queue.Store = (*Queue)(nil)

func getKey(clientID string) string {
	return queuePrefix + clientID
}

type Options struct {
	MaxQueuedMsg    int
	ClientID        string
	InflightExpiry  time.Duration
	Pool            *redigo.Pool
	DefaultNotifier queue.Notifier
}

type Queue struct {
	cond           *sync.Cond
	clientID       string
	version        packets.Version
	readBytesLimit uint32
	// max is the maximum queue length
	max int
	// len is the length of the list
	len             int
	pool            *redigo.Pool
	closed          bool
	inflightDrained bool
	// current is the current read index of Queue list.
	current        int
	readCache      map[packets.PacketID][]byte
	err            error
	log            *zap.Logger
	inflightExpiry time.Duration
	notifier       queue.Notifier
}

func New(opts Options) (*Queue, error) {
	return &Queue{
		cond:            sync.NewCond(&sync.Mutex{}),
		clientID:        opts.ClientID,
		max:             opts.MaxQueuedMsg,
		len:             0,
		pool:            opts.Pool,
		closed:          false,
		inflightDrained: false,
		current:         0,
		inflightExpiry:  opts.InflightExpiry,
		notifier:        opts.DefaultNotifier,
		log:             server.LoggerWithField(zap.String("queue", "redis")),
	}, nil
}

func wrapError(err error) *codes.Error {
	return &codes.Error{
		Code: codes.UnspecifiedError,
		ErrorDetails: codes.ErrorDetails{
			ReasonString:   []byte(err.Error()),
			UserProperties: nil,
		},
	}
}

func (q *Queue) Close() error {
	q.cond.L.Lock()
	defer func() {
		q.cond.L.Unlock()
		q.cond.Signal()
	}()
	q.closed = true
	return nil
}

func (q *Queue) setLen(conn redigo.Conn) error {
	l, err := conn.Do("llen", getKey(q.clientID))
	if err != nil {
		return err
	}
	q.len = int(l.(int64))
	return nil
}

func (q *Queue) Init(opts *queue.InitOptions) error {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	conn := q.pool.Get()
	defer conn.Close()

	if opts.CleanStart {
		_, err := conn.Do("del", getKey(q.clientID))
		if err != nil {
			return wrapError(err)
		}
	}
	err := q.setLen(conn)
	if err != nil {
		return err
	}
	q.version = opts.Version
	q.readBytesLimit = opts.ReadBytesLimit
	q.closed = false
	q.inflightDrained = false
	q.current = 0
	q.readCache = make(map[packets.PacketID][]byte)
	q.notifier = opts.Notifier
	q.cond.Signal()
	return nil
}

func (q *Queue) Clean() error {
	conn := q.pool.Get()
	defer conn.Close()
	_, err := conn.Do("del", getKey(q.clientID))
	return err
}

func (q *Queue) Add(elem *queue.Elem) (err error) {
	now := time.Now()
	conn := q.pool.Get()
	q.cond.L.Lock()
	var dropErr error
	var dropBytes []byte
	var dropElem *queue.Elem
	var drop bool
	defer func() {
		conn.Close()
		q.cond.L.Unlock()
		q.cond.Signal()
	}()

	defer func() {
		if drop {
			if dropErr == queue.ErrDropExpiredInflight {
				q.notifier.NotifyInflightAdded(-1)
				q.current--
			}
			if dropBytes == nil {
				q.notifier.NotifyDropped(elem, dropErr)
				return
			} else {
				err = conn.Send("lrem", getKey(q.clientID), 1, dropBytes)
			}
			q.notifier.NotifyDropped(dropElem, dropErr)
		} else {
			q.notifier.NotifyMsgQueueAdded(1)
			q.len++
		}
		_ = conn.Send("rpush", getKey(q.clientID), elem.Encode())
		err = conn.Flush()
	}()
	if q.len >= q.max {
		// set default drop error
		dropErr = queue.ErrDropQueueFull
		drop = true
		var rs []interface{}
		// drop expired inflight message
		rs, err = redigo.Values(conn.Do("lrange", getKey(q.clientID), 0, q.len))
		if err != nil {
			return
		}
		var frontBytes []byte
		var frontElem *queue.Elem
		for i := 0; i < len(rs); i++ {
			b := rs[i].([]byte)
			e := &queue.Elem{}
			err = e.Decode(b)
			if err != nil {
				return
			}
			// inflight message
			if i < q.current && queue.ElemExpiry(now, e) {
				dropBytes = b
				dropElem = e
				dropErr = queue.ErrDropExpiredInflight
				return
			}
			// non-inflight message
			if i >= q.current {
				if i == q.current {
					frontBytes = b
					frontElem = e
				}
				// drop qos0 message in the queue
				pub := e.MessageWithID.(*queue.Publish)
				// drop expired non-inflight message
				if pub.ID() == 0 && queue.ElemExpiry(now, e) {
					dropBytes = b
					dropElem = e
					dropErr = queue.ErrDropExpired
					return
				}
				if pub.ID() == 0 && pub.QoS == packets.Qos0 && dropElem == nil {
					dropBytes = b
					dropElem = e
				}
			}
		}
		// drop the current elem if there is no more non-inflight messages.
		if q.inflightDrained && q.current >= q.len {
			return
		}
		rs, err = redigo.Values(conn.Do("lrange", getKey(q.clientID), q.current, q.len))
		if err != nil {
			return err
		}
		if dropElem != nil {
			return
		}
		if elem.MessageWithID.(*queue.Publish).QoS == packets.Qos0 {
			return
		}
		if frontElem != nil {
			// drop the front message
			dropBytes = frontBytes
			dropElem = frontElem
		}
		// the the messages in the queue are all inflight messages, drop the current elem
		return
	}
	return nil
}

func (q *Queue) Replace(elem *queue.Elem) (replaced bool, err error) {
	conn := q.pool.Get()
	q.cond.L.Lock()
	defer func() {
		conn.Close()
		q.cond.L.Unlock()
	}()
	id := elem.ID()
	eb := elem.Encode()
	stop := q.current - 1
	if stop < 0 {
		stop = 0
	}
	rs, err := redigo.Values(conn.Do("lrange", getKey(q.clientID), 0, stop))
	if err != nil {
		return false, err
	}
	for k, v := range rs {
		b := v.([]byte)
		e := &queue.Elem{}
		err = e.Decode(b)
		if err != nil {
			return false, err
		}
		if e.ID() == elem.ID() {
			_, err = conn.Do("lset", getKey(q.clientID), k, eb)
			if err != nil {
				return false, err
			}
			q.readCache[id] = eb
			return true, nil
		}
	}

	return false, nil
}

func (q *Queue) Read(pids []packets.PacketID) (elems []*queue.Elem, err error) {
	now := time.Now()
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	conn := q.pool.Get()
	defer conn.Close()
	if !q.inflightDrained {
		panic("must call ReadInflight to drain all inflight messages before Read")
	}
	for q.current >= q.len && !q.closed {
		q.cond.Wait()
	}
	if q.closed {
		return nil, queue.ErrClosed
	}
	rs, err := redigo.Values(conn.Do("lrange", getKey(q.clientID), q.current, q.current+len(pids)-1))
	if err != nil {
		return nil, wrapError(err)
	}
	var msgQueueDelta, inflightDelta int
	var pflag int
	for i := 0; i < len(rs); i++ {
		b := rs[i].([]byte)
		e := &queue.Elem{}
		err := e.Decode(b)
		if err != nil {
			return nil, err
		}
		// remove expired message
		if queue.ElemExpiry(now, e) {
			err = conn.Send("lrem", getKey(q.clientID), 1, b)
			q.len--
			if err != nil {
				return nil, err
			}
			q.notifier.NotifyDropped(e, queue.ErrDropExpired)
			msgQueueDelta--
			continue
		}

		// remove message which exceeds maximum packet size
		pub := e.MessageWithID.(*queue.Publish)
		if size := pub.TotalBytes(q.version); size > q.readBytesLimit {
			err = conn.Send("lrem", getKey(q.clientID), 1, b)
			q.len--
			if err != nil {
				return nil, err
			}
			q.notifier.NotifyDropped(e, queue.ErrDropExceedsMaxPacketSize)
			msgQueueDelta--
			continue
		}

		if e.MessageWithID.(*queue.Publish).QoS == 0 {
			err = conn.Send("lrem", getKey(q.clientID), 1, b)
			q.len--
			msgQueueDelta--
			if err != nil {
				return nil, err
			}
		} else {
			e.MessageWithID.SetID(pids[pflag])
			if q.inflightExpiry != 0 {
				e.Expiry = now.Add(q.inflightExpiry)
			}
			pflag++
			nb := e.Encode()

			err = conn.Send("lset", getKey(q.clientID), q.current, nb)
			q.current++
			inflightDelta++
			q.readCache[e.MessageWithID.ID()] = nb
		}
		elems = append(elems, e)
	}
	err = conn.Flush()
	q.notifier.NotifyMsgQueueAdded(msgQueueDelta)
	q.notifier.NotifyInflightAdded(inflightDelta)
	return
}

func (q *Queue) ReadInflight(maxSize uint) (elems []*queue.Elem, err error) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	conn := q.pool.Get()
	defer conn.Close()
	rs, err := redigo.Values(conn.Do("lrange", getKey(q.clientID), q.current, q.current+int(maxSize)-1))
	if len(rs) == 0 {
		q.inflightDrained = true
		return
	}
	if err != nil {
		return nil, wrapError(err)
	}
	beginIndex := q.current
	for index, v := range rs {
		b := v.([]byte)
		e := &queue.Elem{}
		err := e.Decode(b)
		if err != nil {
			return nil, err
		}
		id := e.MessageWithID.ID()
		if id != 0 {
			if q.inflightExpiry != 0 {
				e.Expiry = time.Now().Add(q.inflightExpiry)
				b = e.Encode()
				_, err = conn.Do("lset", getKey(q.clientID), beginIndex+index, b)
				if err != nil {
					return nil, err
				}
			}
			elems = append(elems, e)
			q.readCache[id] = b
			q.current++
		} else {
			q.inflightDrained = true
			return elems, nil
		}
	}
	return
}

func (q *Queue) Remove(pid packets.PacketID) error {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	conn := q.pool.Get()
	defer conn.Close()
	if b, ok := q.readCache[pid]; ok {
		_, err := conn.Do("lrem", getKey(q.clientID), 1, b)
		if err != nil {
			return err
		}
		q.notifier.NotifyMsgQueueAdded(-1)
		q.notifier.NotifyInflightAdded(-1)
		delete(q.readCache, pid)
		q.len--
		q.current--
	}
	return nil
}
