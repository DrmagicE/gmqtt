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

var _ queue.Store = (*Queue)(nil)

type Options struct {
	MaxQueuedMsg int
	ClientID     string
	DropHandler  server.OnMsgDropped
	Pool         *redigo.Pool
}

type Queue struct {
	cond            *sync.Cond
	clientID        string
	version         packets.Version
	readBytesLimit  uint32
	max             int
	len             int // the length of the list
	pool            *redigo.Pool
	closed          bool
	inflightDrained bool
	current         int // the current read index of Queue list.
	readCache       map[packets.PacketID][]byte
	err             error
	onMsgDropped    server.OnMsgDropped
	log             *zap.Logger
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
		log:             server.LoggerWithField(zap.String("queue", "redis")),
		onMsgDropped:    opts.DropHandler,
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

func (q *Queue) Init(opts *queue.InitOptions) error {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	conn := q.pool.Get()
	defer conn.Close()

	if opts.CleanStart {
		_, err := conn.Do("del", q.clientID)
		if err != nil {
			return wrapError(err)
		}
	}
	b, err := conn.Do("llen", q.clientID)
	if err != nil {
		return err
	}
	q.version = opts.Version
	q.readBytesLimit = opts.ReadBytesLimit
	q.len = int(b.(int64))
	q.closed = false
	q.inflightDrained = false
	q.current = 0
	q.readCache = make(map[packets.PacketID][]byte)
	q.cond.Signal()
	return nil
}

func (q *Queue) Clean() error {
	conn := q.pool.Get()
	defer conn.Close()
	_, err := conn.Do("del", q.clientID)
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
			if dropBytes == nil {
				queue.Drop(q.onMsgDropped, q.log, q.clientID, elem.MessageWithID.(*queue.Publish).Message, dropErr)
				return
			} else {
				err = conn.Send("lrem", q.clientID, 1, dropBytes)
			}
			queue.Drop(q.onMsgDropped, q.log, q.clientID, dropElem.MessageWithID.(*queue.Publish).Message, dropErr)
		}
		_ = conn.Send("rpush", q.clientID, elem.Encode())
		err = conn.Flush()
		q.len++
	}()
	if q.len >= q.max {
		// set default drop error
		dropErr = queue.ErrDropQueueFull
		drop = true
		// drop the current elem if there is no more non-inflight messages.
		if q.inflightDrained && q.current >= q.len {
			return
		}
		var rs []interface{}
		rs, err = redigo.Values(conn.Do("lrange", q.clientID, q.current, q.len))
		if err != nil {
			return err
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
			pub := e.MessageWithID.(*queue.Publish)
			if pub.ID() == 0 {
				// drop the front message
				if i == 0 {
					frontBytes = b
					frontElem = e
				}
				// drop expired message
				if queue.ElemExpiry(now, e) {
					dropErr = queue.ErrDropExpired
					dropBytes = b
					dropElem = e
					return
				}
				if pub.QoS == packets.Qos0 && dropElem == nil {
					dropBytes = b
					dropElem = e
				}
			}
		}
		// drop qos0 message in the queue
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
	rs, err := redigo.Values(conn.Do("lrange", q.clientID, 0, stop))
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
			_, err = conn.Do("lset", q.clientID, k, eb)
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
	for (q.current >= q.len) && !q.closed {
		q.cond.Wait()
	}
	if q.closed {
		return nil, queue.ErrClosed
	}
	rs, err := redigo.Values(conn.Do("lrange", q.clientID, q.current, q.current+len(pids)-1))
	if err != nil {
		return nil, wrapError(err)
	}
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
			err = conn.Send("lrem", q.clientID, 1, b)
			q.len--
			if err != nil {
				return nil, err
			}
			queue.Drop(q.onMsgDropped, q.log, q.clientID, e.MessageWithID.(*queue.Publish).Message, queue.ErrDropExpired)
			continue
		}

		// remove message which exceeds maximum packet size
		pub := e.MessageWithID.(*queue.Publish)
		if size := pub.TotalBytes(q.version); size > q.readBytesLimit {
			err = conn.Send("lrem", q.clientID, 1, b)
			q.len--
			if err != nil {
				return nil, err
			}
			queue.Drop(q.onMsgDropped, q.log, q.clientID, pub.Message, queue.ErrDropExceedsMaxPacketSize)
			continue
		}

		if e.MessageWithID.(*queue.Publish).QoS == 0 {
			err = conn.Send("lrem", q.clientID, 1, b)
			q.len--
			if err != nil {
				return nil, err
			}
		} else {
			e.MessageWithID.SetID(pids[pflag])
			pflag++
			nb := e.Encode()
			err = conn.Send("lset", q.clientID, q.current, nb)
			q.current++
			q.readCache[e.MessageWithID.ID()] = nb
		}
		elems = append(elems, e)
	}
	err = conn.Flush()
	return
}

func (q *Queue) ReadInflight(maxSize uint) (elems []*queue.Elem, err error) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	conn := q.pool.Get()
	defer conn.Close()
	rs, err := redigo.Values(conn.Do("lrange", q.clientID, q.current, q.current+int(maxSize)-1))
	if len(rs) == 0 {
		q.inflightDrained = true
		return
	}
	if err != nil {
		return nil, wrapError(err)
	}
	for _, v := range rs {
		b := v.([]byte)
		e := &queue.Elem{}
		err := e.Decode(b)
		if err != nil {
			return nil, err
		}
		id := e.MessageWithID.ID()
		if id != 0 {
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
		_, err := conn.Do("lrem", q.clientID, 1, b)
		if err != nil {
			return err
		}
		delete(q.readCache, pid)
		q.len--
		q.current--
	}
	return nil
}
