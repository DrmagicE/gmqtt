package redis

import (
	"sync"
	"time"

	redigo "github.com/gomodule/redigo/redis"

	"github.com/DrmagicE/gmqtt/pkg/codes"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/DrmagicE/gmqtt/server"

	"github.com/DrmagicE/gmqtt/persistence/queue"
)

func init() {
	server.RegisterNewQueueStore("redis", New)
}

func newPool(addr string) *redigo.Pool {
	return &redigo.Pool{
		MaxIdle:     3,
		IdleTimeout: 240 * time.Second,
		// Dial or DialContext must be set. When both are set, DialContext takes precedence over Dial.
		Dial: func() (redigo.Conn, error) { return redigo.Dial("tcp", addr) },
	}
}

func New(config server.Config, client server.Client) (queue.Store, error) {
	return &redis{
		cond:            sync.NewCond(&sync.Mutex{}),
		client:          client,
		clientID:        client.ClientOptions().ClientID,
		max:             0,
		len:             0,
		pool:            newPool(":6379"),
		conn:            nil,
		closed:          false,
		inflightDrained: false,
		current:         0,
	}, nil
}

type redis struct {
	cond            *sync.Cond
	client          server.Client
	clientID        string
	max             int
	len             int // the length of the list
	pool            *redigo.Pool
	conn            redigo.Conn // will set after Init
	closed          bool
	inflightDrained bool
	current         int // the current read index of redis list.
	readCache       map[packets.PacketID][]byte
	err             error
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

func (r *redis) Close() error {
	r.cond.L.Lock()
	defer func() {
		r.cond.L.Unlock()
		r.cond.Signal()
	}()
	r.closed = true
	return nil
}

func (r *redis) Init(cleanStart bool) error {
	r.cond.L.Lock()
	defer r.cond.L.Unlock()
	r.conn = r.pool.Get()
	if cleanStart {
		_, err := r.conn.Do("del", r.clientID)
		if err != nil {
			return wrapError(err)
		}
	}
	b, err := r.conn.Do("llen", r.clientID)
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

func (r *redis) Clean() error {
	_, err := r.conn.Do("del", r.clientID)
	return err
}

func (r *redis) Add(elem *queue.Elem) error {
	r.cond.L.Lock()
	defer func() {
		r.len++
		r.cond.L.Unlock()
		r.cond.Signal()
	}()
	_, err := r.conn.Do("rpush", r.clientID, elem.Encode())
	//fmt.Println(rs, err)
	if err != nil {
		return err
	}
	return nil
}

func (r *redis) Replace(elem *queue.Elem) (replaced bool, err error) {
	r.cond.L.Lock()
	defer r.cond.L.Unlock()
	id := elem.ID()
	eb := elem.Encode()
	stop := r.current - 1
	if stop < 0 {
		stop = 0
	}
	rs, err := redigo.Values(r.conn.Do("lrange", r.clientID, 0, stop))
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
			_, err = r.conn.Do("lset", r.clientID, k, eb)
			if err != nil {
				return false, err
			}
			r.readCache[id] = eb
		}
	}

	return false, nil
}

func (r *redis) Read(pids []packets.PacketID) (elems []*queue.Elem, err error) {
	r.cond.L.Lock()
	defer r.cond.L.Unlock()
	if !r.inflightDrained {
		panic("must call ReadInflight to drain all inflight messages before Read")
	}
	for (r.current >= r.len) && !r.closed {
		r.cond.Wait()
	}
	if r.closed {
		return nil, queue.ErrClosed
	}
	rs, err := redigo.Values(r.conn.Do("lrange", r.clientID, r.current, r.current+len(pids)-1))
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
			err = r.conn.Send("lrem", r.clientID, 1, b)
			r.len--
			if err != nil {
				return nil, err
			}
		} else {
			e.MessageWithID.SetID(pids[pflag])
			pflag++
			nb := e.Encode()
			err = r.conn.Send("lset", r.clientID, r.current, nb)
			r.current++
			r.readCache[e.MessageWithID.ID()] = nb
		}
		elems = append(elems, e)
	}
	err = r.conn.Flush()
	return
}

func (r *redis) ReadInflight(maxSize uint) (elems []*queue.Elem, err error) {
	r.cond.L.Lock()
	defer r.cond.L.Unlock()
	rs, err := redigo.Values(r.conn.Do("lrange", r.clientID, r.current, r.current+int(maxSize)-1))
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

func (r *redis) Remove(pid packets.PacketID) error {
	r.cond.L.Lock()
	defer r.cond.L.Unlock()
	if b, ok := r.readCache[pid]; ok {
		_, err := r.conn.Do("lrem", r.clientID, 1, b)
		if err != nil {
			return err
		}
		delete(r.readCache, pid)
		r.len--
		r.current--
	}
	return nil
}
