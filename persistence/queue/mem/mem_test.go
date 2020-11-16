package mem

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DrmagicE/gmqtt/persistence/queue"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/DrmagicE/gmqtt/server"
)

// Test case pid按顺序消费
func TestMem_Read(t *testing.T) {
	a := assert.New(t)
	m, _ := New(server.Config{}, nil)
	rs, err := m.ReadInflight(1)
	a.Nil(err)
	a.Len(rs, 0)

	a.Nil(err)
	m.Add(&queue.Elem{
		At:     time.Time{},
		Expiry: time.Time{},
		MessageWithID: &queue.Pubrel{
			PacketID: 1,
		},
	})
	fmt.Println(m.Read([]packets.PacketID{1, 2}))
	m.Add(&queue.Elem{
		At:     time.Time{},
		Expiry: time.Time{},
		MessageWithID: &queue.Pubrel{
			PacketID: 2,
		},
	})
	fmt.Println(m.Read([]packets.PacketID{1, 2}))
}
