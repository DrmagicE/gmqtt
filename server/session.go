package server

import (
	"sync"

	"go.uber.org/zap"

	"github.com/DrmagicE/gmqtt/pkg/packets"
)

// packetIDLimiter limit the generation of packet id to keep the number of inflight messages always less or equal than receive maximum setting of the client.
type packetIDLimiter struct {
	cond   *sync.Cond
	used   uint16 // 当前用了多少ID
	limit  uint16 // 最多同时可以用多少个个ID
	exit   bool
	client *client

	lockedPid map[packets.PacketID]bool // packet id in-use
	freePid   packets.PacketID          //下一个可以使用的freeID
}

func (client *client) newPacketIDLimiter(limit uint16) {
	client.pl = &packetIDLimiter{
		cond:      sync.NewCond(&sync.Mutex{}),
		used:      0,
		limit:     limit,
		exit:      false,
		client:    client,
		freePid:   1,
		lockedPid: make(map[packets.PacketID]bool),
	}
}
func (p *packetIDLimiter) close() {
	p.cond.L.Lock()
	p.exit = true
	p.cond.L.Unlock()
	p.cond.Signal()
}

// pollPacketIDs returns at most max number of unused packetID and marks them as used for a client.
// If there is no available id, the call will be blocked until at least one packet id is available or the limiter has been closed.
// return 0 means the limiter is closed.
// the return number = min(max, i.used).
func (p *packetIDLimiter) pollPacketIDs(max uint16) (id []packets.PacketID) {
	p.cond.L.Lock()
	defer p.cond.L.Unlock()
	for p.used >= p.limit && !p.exit {
		p.cond.Wait()
	}
	if p.exit {
		return nil
	}
	n := max
	if remain := p.limit - p.used; remain < max {
		n = remain
	}
	for j := uint16(0); j < n; j++ {
		for p.lockedPid[p.freePid] {
			p.freePid++
			if p.freePid > packets.MaxPacketID {
				p.freePid = packets.MinPacketID
			}
		}
		id = append(id, p.freePid)
		p.used++
		p.lockedPid[p.freePid] = true
		p.freePid++
		if p.freePid > packets.MaxPacketID {
			p.freePid = packets.MinPacketID
		}
	}
	return id
}

// release marks the given id list as unused
func (p *packetIDLimiter) release(id packets.PacketID) {
	p.cond.L.Lock()
	p.releaseLocked(id)
	zaplog.Debug("release client quota",
		zap.String("client_id", p.client.opts.ClientID),
		zap.Uint16("quota", p.used),
		zap.String("remote_addr", p.client.rwc.RemoteAddr().String()),
	)
	p.cond.L.Unlock()
	p.cond.Signal()

}
func (p *packetIDLimiter) releaseLocked(id packets.PacketID) {
	if p.lockedPid[id] {
		p.lockedPid[id] = false
		p.used--
	}
}

func (p *packetIDLimiter) batchRelease(id []packets.PacketID) {
	p.cond.L.Lock()
	for _, v := range id {
		p.releaseLocked(v)
	}
	p.cond.L.Unlock()
	p.cond.Signal()

}

// markInUsed marks the given id as used.
func (p *packetIDLimiter) markUsedLocked(id packets.PacketID) {
	p.used++
	p.lockedPid[id] = true
}

func (p *packetIDLimiter) lock() {
	p.cond.L.Lock()
}
func (p *packetIDLimiter) unlock() {
	p.cond.L.Unlock()
}
func (p *packetIDLimiter) unlockAndSignal() {
	p.cond.L.Unlock()
	p.cond.Signal()
}
