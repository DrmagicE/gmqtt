package federation

import (
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/serf/serf"
	"go.uber.org/zap"
)

// iSerf is the interface for *serf.Serf.
// It is used for test.
type iSerf interface {
	Join(existing []string, ignoreOld bool) (int, error)
	RemoveFailedNode(node string) error
	Leave() error
	Members() []serf.Member
	Shutdown() error
}

var servePeerEventStream = func(p *peer) {
	p.serveEventStream()
}

func (f *Federation) startSerf(t *time.Timer) error {
	defer func() {
		t.Reset(f.config.RetryInterval)
	}()
	if _, err := f.serf.Join(f.config.RetryJoin, true); err != nil {
		return err
	}
	go f.eventHandler()
	return nil
}

func (f *Federation) eventHandler() {
	for {
		select {
		case evt := <-f.serfEventCh:
			switch evt.EventType() {
			case serf.EventMemberJoin:
				f.nodeJoin(evt.(serf.MemberEvent))
			case serf.EventMemberLeave, serf.EventMemberFailed, serf.EventMemberReap:
				f.nodeFail(evt.(serf.MemberEvent))
			case serf.EventUser:
			case serf.EventMemberUpdate:
				// TODO
			case serf.EventQuery: // Ignore
			default:
			}
		case <-f.exit:
			f.memberMu.Lock()
			for _, v := range f.peers {
				v.stop()
			}
			f.memberMu.Unlock()
			return
		}
	}
}

func (f *Federation) nodeJoin(member serf.MemberEvent) {
	f.memberMu.Lock()
	defer f.memberMu.Unlock()
	for _, v := range member.Members {
		if v.Name == f.nodeName {
			continue
		}
		log.Info("member joined", zap.String("node_name", v.Name))
		if _, ok := f.peers[v.Name]; !ok {
			p := &peer{
				fed:       f,
				member:    v,
				exit:      make(chan struct{}),
				sessionID: uuid.New().String(),
				queue:     newEventQueue(),
				localName: f.nodeName,
			}
			f.peers[v.Name] = p
			go servePeerEventStream(p)
		}
	}
}

func (f *Federation) nodeFail(member serf.MemberEvent) {
	f.memberMu.Lock()
	defer f.memberMu.Unlock()
	for _, v := range member.Members {
		if v.Name == f.nodeName {
			continue
		}
		if p, ok := f.peers[v.Name]; ok {
			log.Error("node failed, close stream client", zap.String("node_name", v.Name))
			p.stop()
			delete(f.peers, v.Name)
			_ = f.fedSubStore.UnsubscribeAll(v.Name)
			f.sessionMgr.del(v.Name)
		}
	}
}
