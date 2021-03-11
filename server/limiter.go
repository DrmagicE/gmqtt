package server

import (
	"sync"

	"github.com/DrmagicE/gmqtt/pkg/bitmap"
	"github.com/DrmagicE/gmqtt/pkg/packets"
)

func newPacketIDLimiter(limit uint16) *packetIDLimiter {
	return &packetIDLimiter{
		cond:      sync.NewCond(&sync.Mutex{}),
		used:      0,
		limit:     limit,
		exit:      false,
		freePid:   1,
		lockedPid: bitmap.New(packets.MaxPacketID),
	}
}

// packetIDLimiter limit the generation of packet id to keep the number of inflight messages
// always less or equal than receive maximum setting of the client.
type packetIDLimiter struct {
	cond      *sync.Cond
	used      uint16
	limit     uint16
	exit      bool
	lockedPid *bitmap.Bitmap   // packet id in-use
	freePid   packets.PacketID // next available id
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
		for p.lockedPid.IsUsed(p.freePid) {
			if p.freePid == packets.MaxPacketID {
				p.freePid = packets.MinPacketID
			} else {
				p.freePid++
			}
		}
		id = append(id, p.freePid)
		p.used++
		p.lockedPid.Used(p.freePid)
		if p.freePid == packets.MaxPacketID {
			p.freePid = packets.MinPacketID
		} else {
			p.freePid++
		}
	}
	return id
}

// release marks the given id list as unused
func (p *packetIDLimiter) release(id packets.PacketID) {
	p.cond.L.Lock()
	p.releaseLocked(id)
	p.cond.L.Unlock()
	p.cond.Signal()

}
func (p *packetIDLimiter) releaseLocked(id packets.PacketID) {
	if p.lockedPid.IsUsed(id) {
		p.lockedPid.Unused(id)
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
	p.lockedPid.Used(id)
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
