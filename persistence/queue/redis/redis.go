package redis

import (
	"sync"

	redigo "github.com/gomodule/redigo/redis"

	"github.com/DrmagicE/gmqtt/pkg/codes"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/DrmagicE/gmqtt/server"

	"github.com/DrmagicE/gmqtt/persistence/queue"
)

var _ queue.Store = (*Queue)(nil)

func New(config server.Config, client server.Client, pool *redigo.Pool, dropped server.OnMsgDropped) (*Queue, error) {
	return &Queue{
		cond:            sync.NewCond(&sync.Mutex{}),
		client:          client,
		clientID:        client.ClientOptions().ClientID,
		max:             0,
		len:             0,
		pool:            pool,
		closed:          false,
		inflightDrained: false,
		current:         0,
		onMsgDropped:    dropped,
	}, nil
}

type Queue struct {
	cond            *sync.Cond
	client          server.Client
	clientID        string
	max             int
	len             int // the length of the list
	pool            *redigo.Pool
	closed          bool
	inflightDrained bool
	current         int // the current read index of Queue list.
	readCache       map[packets.PacketID][]byte
	err             error
	onMsgDropped    server.OnMsgDropped
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

func (r *Queue) Close() error {
	r.cond.L.Lock()
	defer func() {
		r.cond.L.Unlock()
		r.cond.Signal()
	}()
	r.closed = true
	return nil
}

func (r *Queue) Init(cleanStart bool) error {
	r.cond.L.Lock()
	defer r.cond.L.Unlock()
	conn := r.pool.Get()
	defer conn.Close()

	if cleanStart {
		_, err := conn.Do("del", r.clientID)
		if err != nil {
			return wrapError(err)
		}
	}
	b, err := conn.Do("llen", r.clientID)
	if err != nil {
		return err
	}
	r.len = int(b.(int64))
	r.closed = false
	r.inflightDrained = false
	r.current = 0
	r.readCache = make(map[packets.PacketID][]byte)
	r.cond.Signal()
	return nil
}

func (r *Queue) Clean() error {
	conn := r.pool.Get()
	defer conn.Close()
	_, err := conn.Do("del", r.clientID)
	return err
}

func (r *Queue) Add(elem *queue.Elem) error {
	conn := r.pool.Get()
	r.cond.L.Lock()
	defer func() {
		conn.Close()
		r.len++
		r.cond.L.Unlock()
		r.cond.Signal()
	}()
	_, err := conn.Do("rpush", r.clientID, elem.Encode())
	if err != nil {
		return err
	}
	return nil
}

func (r *Queue) Replace(elem *queue.Elem) (replaced bool, err error) {
	conn := r.pool.Get()
	r.cond.L.Lock()
	defer func() {
		conn.Close()
		r.cond.L.Unlock()
	}()
	id := elem.ID()
	eb := elem.Encode()
	stop := r.current - 1
	if stop < 0 {
		stop = 0
	}
	rs, err := redigo.Values(conn.Do("lrange", r.clientID, 0, stop))
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
			_, err = conn.Do("lset", r.clientID, k, eb)
			if err != nil {
				return false, err
			}
			r.readCache[id] = eb
		}
	}

	return false, nil
}

func (r *Queue) Read(pids []packets.PacketID) (elems []*queue.Elem, err error) {
	r.cond.L.Lock()
	defer r.cond.L.Unlock()
	conn := r.pool.Get()
	defer conn.Close()
	if !r.inflightDrained {
		panic("must call ReadInflight to drain all inflight messages before Read")
	}
	for (r.current >= r.len) && !r.closed {
		r.cond.Wait()
	}
	if r.closed {
		return nil, queue.ErrClosed
	}
	rs, err := redigo.Values(conn.Do("lrange", r.clientID, r.current, r.current+len(pids)-1))
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
		if e.MessageWithID.(*queue.Publish).QoS == 0 {
			err = conn.Send("lrem", r.clientID, 1, b)
			r.len--
			if err != nil {
				return nil, err
			}
		} else {
			e.MessageWithID.SetID(pids[pflag])
			pflag++
			nb := e.Encode()
			err = conn.Send("lset", r.clientID, r.current, nb)
			r.current++
			r.readCache[e.MessageWithID.ID()] = nb
		}
		elems = append(elems, e)
	}
	err = conn.Flush()
	return
}

func (r *Queue) ReadInflight(maxSize uint) (elems []*queue.Elem, err error) {
	r.cond.L.Lock()
	defer r.cond.L.Unlock()
	conn := r.pool.Get()
	defer conn.Close()
	rs, err := redigo.Values(conn.Do("lrange", r.clientID, r.current, r.current+int(maxSize)-1))
	if len(rs) == 0 {
		r.inflightDrained = true
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
			r.readCache[id] = b
			r.current++
		} else {
			r.inflightDrained = true
			return elems, nil
		}
	}
	return
}

func (r *Queue) Remove(pid packets.PacketID) error {
	r.cond.L.Lock()
	defer r.cond.L.Unlock()
	conn := r.pool.Get()
	defer conn.Close()
	if b, ok := r.readCache[pid]; ok {
		_, err := conn.Do("lrem", r.clientID, 1, b)
		if err != nil {
			return err
		}
		delete(r.readCache, pid)
		r.len--
		r.current--
	}
	return nil
}
