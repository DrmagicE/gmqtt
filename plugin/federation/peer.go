package federation

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/hashicorp/serf/serf"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/persistence/subscription"
)

type peerState byte

const (
	peerStateStopped peerState = iota + 1
	peerStateStreaming
)

// peer represents a remote node which act as the event stream server.
type peer struct {
	fed       *Federation
	localName string
	member    serf.Member
	exit      chan struct{}
	// local session id
	sessionID string
	queue     queue
	// stateMu guards the following fields
	stateMu sync.Mutex
	state   peerState
	// client-side stream
	stream *stream
}

type stream struct {
	queue   queue
	conn    *grpc.ClientConn
	client  Federation_EventStreamClient
	close   chan struct{}
	errOnce sync.Once
	err     error
	wg      sync.WaitGroup
}

// interface for testing
type queue interface {
	clear()
	close()
	open()
	setReadPosition(id uint64)
	add(event *Event)
	fetchEvents() []*Event
	ack(id uint64)
}

// eventQueue store the events that are ready to send.
// TODO add max buffer size
type eventQueue struct {
	cond     *sync.Cond
	nextID   uint64
	l        *list.List
	nextRead *list.Element
	closed   bool
}

func newEventQueue() *eventQueue {
	return &eventQueue{
		cond:   sync.NewCond(&sync.Mutex{}),
		nextID: 0,
		l:      list.New(),
		closed: false,
	}
}

func (e *eventQueue) clear() {
	e.cond.L.Lock()
	defer e.cond.L.Unlock()
	e.nextID = 0
	e.l = list.New()
	e.nextRead = nil
	e.closed = false
}

func (e *eventQueue) close() {
	e.cond.L.Lock()
	defer e.cond.L.Unlock()
	e.closed = true
	e.cond.Signal()
}

func (e *eventQueue) open() {
	e.cond.L.Lock()
	defer e.cond.L.Unlock()
	e.closed = false
	e.cond.Signal()
}

func (e *eventQueue) setReadPosition(id uint64) {
	e.cond.L.Lock()
	defer e.cond.L.Unlock()
	for elem := e.l.Front(); elem != nil; elem = elem.Next() {
		ev := elem.Value.(*Event)
		if ev.Id == id {
			e.nextRead = elem
			return
		}
	}
}

func (e *eventQueue) add(event *Event) {
	e.cond.L.Lock()
	defer func() {
		e.cond.L.Unlock()
		e.cond.Signal()
	}()
	event.Id = e.nextID
	e.nextID++
	elem := e.l.PushBack(event)
	if e.nextRead == nil {
		e.nextRead = elem
	}
}

func (e *eventQueue) fetchEvents() []*Event {
	e.cond.L.Lock()
	defer e.cond.L.Unlock()

	for (e.l.Len() == 0 || e.nextRead == nil) && !e.closed {
		e.cond.Wait()
	}
	if e.closed {
		return nil
	}
	ev := make([]*Event, 0)
	var elem *list.Element
	elem = e.nextRead
	for i := 0; i < 100; i++ {
		ev = append(ev, elem.Value.(*Event))
		elem = elem.Next()
		if elem == nil {
			break
		}
	}
	e.nextRead = elem
	return ev
}

func (e *eventQueue) ack(id uint64) {
	e.cond.L.Lock()
	defer func() {
		e.cond.L.Unlock()
		e.cond.Signal()
	}()
	var next *list.Element
	for elem := e.l.Front(); elem != nil; elem = next {
		next = elem.Next()
		req := elem.Value.(*Event)
		if req.Id <= id {
			e.l.Remove(elem)
		}
		if req.Id == id {
			return
		}
	}
}

func (p *peer) stop() {
	select {
	case <-p.exit:
	default:
		close(p.exit)
	}
	p.stateMu.Lock()
	state := p.state
	if state == peerStateStreaming {
		_ = p.stream.conn.Close()
	}
	p.state = peerStateStopped
	p.stateMu.Unlock()
	if state == peerStateStreaming {
		p.stream.wg.Wait()
	}
}

func (p *peer) serveEventStream() {
	timer := time.NewTimer(0)
	var reconnectCount int
	for {
		select {
		case <-p.exit:
			return
		case <-timer.C:
			err := p.serveStream(reconnectCount, timer)
			select {
			case <-p.exit:
				return
			default:
			}
			if err != nil {
				log.Error("stream broken, reconnecting", zap.Error(err),
					zap.Int("reconnect_count", reconnectCount))
				reconnectCount++
				continue
			}
			return
		}
	}
}

func (p *peer) initStream(client FederationClient, conn *grpc.ClientConn) (s *stream, err error) {
	p.stateMu.Lock()
	defer func() {
		if err == nil {
			p.state = peerStateStreaming
		}
		p.stateMu.Unlock()
	}()
	if p.state == peerStateStopped {
		return nil, errors.New("peer has been stopped")
	}
	helloMD := metadata.Pairs("node_name", p.localName)
	helloCtx := metadata.NewOutgoingContext(context.Background(), helloMD)
	sh, err := client.Hello(helloCtx, &ClientHello{
		SessionId: p.sessionID,
	})
	if err != nil {
		return nil, fmt.Errorf("handshake error: %s", err.Error())
	}
	log.Info("handshake succeed", zap.String("remote_node", p.member.Name), zap.Bool("clean_start", sh.CleanStart))
	if sh.CleanStart {
		p.queue.clear()
		// sync full state
		p.fed.localSubStore.Lock()
		for k := range p.fed.localSubStore.topics {
			shareName, topicFilter := subscription.SplitTopic(k)
			p.queue.add(&Event{
				Event: &Event_Subscribe{Subscribe: &Subscribe{
					ShareName:   shareName,
					TopicFilter: topicFilter,
				}},
			})
		}
		p.fed.localSubStore.Unlock()

		p.fed.retainedStore.Iterate(func(message *gmqtt.Message) bool {
			// TODO add timestamp to retained message and use Last Write Wins (LWW) to resolve write conflicts.
			p.queue.add(&Event{
				Event: &Event_Message{
					Message: messageToEvent(message.Copy()),
				},
			})
			return true
		})
	}
	p.queue.setReadPosition(sh.NextEventId)
	md := metadata.Pairs("node_name", p.localName)
	ctx := metadata.NewOutgoingContext(context.Background(), md)
	c, err := client.EventStream(ctx)
	if err != nil {
		return nil, err
	}
	p.queue.open()
	s = &stream{
		queue:  p.queue,
		conn:   conn,
		client: c,
		close:  make(chan struct{}),
	}
	p.stream = s
	return s, nil
}

func (p *peer) serveStream(reconnectCount int, backoff *time.Timer) (err error) {
	defer func() {
		if err != nil {
			du := time.Duration(0)
			if reconnectCount != 0 {
				du = time.Duration(reconnectCount) * 500 * time.Millisecond
			}
			if max := 2 * time.Second; du > max {
				du = max
			}
			backoff.Reset(du)
		}
	}()
	addr := p.member.Tags["fed_addr"]
	conn, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		return err
	}
	client := NewFederationClient(conn)
	s, err := p.initStream(client, conn)
	if err != nil {
		return err
	}
	return s.serve()
}

func (s *stream) serve() error {
	s.wg.Add(2)
	go s.readLoop()
	go s.sendEvents()
	s.wg.Wait()
	return s.err
}

func (s *stream) setError(err error) {
	s.errOnce.Do(func() {
		s.queue.close()
		s.conn.Close()
		close(s.close)
		if err != nil && err != io.EOF {
			log.Error("stream error", zap.Error(err))
			s.err = err
		}
	})
}

func (s *stream) readLoop() {
	var err error
	var resp *Ack
	defer func() {
		if re := recover(); re != nil {
			err = errors.New(fmt.Sprint(re))
		}
		s.setError(err)
		s.wg.Done()
	}()
	for {
		select {
		case <-s.close:
			return
		default:
			resp, err = s.client.Recv()
			if err != nil {
				return
			}
			s.queue.ack(resp.EventId)
			if ce := log.Check(zapcore.DebugLevel, "event acked"); ce != nil {
				ce.Write(zap.Uint64("id", resp.EventId))
			}
		}
	}
}

func (s *stream) sendEvents() {
	var err error
	defer func() {
		if re := recover(); re != nil {
			err = errors.New(fmt.Sprint(re))
		}
		s.setError(err)
		s.wg.Done()
	}()
	for {
		events := s.queue.fetchEvents()
		// stream has been closed
		if events == nil {
			return
		}
		for _, v := range events {
			err := s.client.Send(v)
			if err != nil {
				return
			}
			if ce := log.Check(zapcore.DebugLevel, "event sent"); ce != nil {
				ce.Write(zap.String("event", v.String()))
			}
		}
	}
}
