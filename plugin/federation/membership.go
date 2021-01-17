package federation

import (
	"net"
	"strconv"

	"github.com/google/uuid"
	"github.com/hashicorp/serf/serf"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var servePeerEventStream = func(p *peer) {
	p.serveEventStream()
}

func (f *Federation) startSerf() error {
	serfCfg := serf.DefaultConfig()
	serfCfg.NodeName = f.config.NodeName
	serfCfg.EventCh = f.serfEventCh
	host, port, _ := net.SplitHostPort(f.config.GossipAddr)
	if host != "" {
		serfCfg.MemberlistConfig.BindAddr = host
	}
	p, _ := strconv.Atoi(port)
	serfCfg.MemberlistConfig.BindPort = p
	//serfCfg.ReapInterval = 1 * time.Second
	//serfCfg.ReconnectTimeout = 10 * time.Second
	serfCfg.Tags = map[string]string{"fed_addr": f.config.FedAddr}
	serfCfg.Logger, _ = zap.NewStdLogAt(log, zapcore.InfoLevel)
	serfCfg.MemberlistConfig.Logger, _ = zap.NewStdLogAt(log, zapcore.InfoLevel)
	s, err := serf.Create(serfCfg)
	if err != nil {
		return err
	}
	s.Join(f.config.Join, true)
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
			f.mu.Lock()
			for _, v := range f.peers {
				v.stop()
			}
			f.mu.Unlock()
			return
		}
	}
}

func (f *Federation) nodeJoin(member serf.MemberEvent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, v := range member.Members {
		if v.Name == f.nodeName {
			continue
		}
		log.Info("member joined", zap.String("node_name", v.Name))
		if _, ok := f.members[v.Name]; !ok {
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
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, v := range member.Members {
		if v.Name == f.nodeName {
			continue
		}
		if p, ok := f.peers[v.Name]; ok {
			log.Error("node failed, close stream client", zap.String("node_name", v.Name))
			p.stop()
			delete(f.peers, v.Name)
			f.feSubStore.UnsubscribeAll(v.Name)
		}
	}
}
