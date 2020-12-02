package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DrmagicE/gmqtt/pkg/packets"
)

func Test_packetIDLimiter(t *testing.T) {
	a := assert.New(t)
	p := newPacketIDLimiter(10)
	ids := p.pollPacketIDs(20)
	a.Len(ids, 10)
	a.Equal([]packets.PacketID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, ids)

	p.batchRelease([]packets.PacketID{7, 8, 9})

	ids = p.pollPacketIDs(4)
	a.Len(ids, 3)
	a.Equal([]packets.PacketID{11, 12, 13}, ids)

	c := make(chan struct{})
	go func() {
		p.pollPacketIDs(1)
		c <- struct{}{}
	}()
	select {
	case <-c:
		t.Fatal("pollPacketIDs should be blocked")
	case <-time.After(1 * time.Second):
	}
	p.close()
	a.Nil(p.pollPacketIDs(10))
}

func Test_packetIDLimiterMax(t *testing.T) {
	a := assert.New(t)
	p := newPacketIDLimiter(65535)
	ids := p.pollPacketIDs(65535)
	a.Len(ids, 65535)
	p.batchRelease([]packets.PacketID{1, 2, 3, 65535})
	a.Equal([]packets.PacketID{1, 2, 3}, p.pollPacketIDs(3))
	a.Equal([]packets.PacketID{65535}, p.pollPacketIDs(3))

}
