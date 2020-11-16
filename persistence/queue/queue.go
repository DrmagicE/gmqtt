package queue

import (
	"errors"
	"time"

	"github.com/DrmagicE/gmqtt/pkg/packets"
)

var (
	ErrClosed = errors.New("queue has been closed")
)

// Store represents a queue store for one client.
type Store interface {
	// Close will be called when the client disconnect.
	// This method must unblock the Read method.
	Close() error
	// Init will be called when the client connect.
	// If cleanStart set to true, the implementation should remove any associated data in backend store.
	// If it set to false, the implementation should retrieve the associated data from backend store.
	Init(cleanStart bool) error
	Clean() error
	// Add inserts a elem to the queue.
	// When the len of queue is reaching the maximum setting, the implementation should drop non-inflight messages according the following priorities:
	// 1. the current elem if there is no more non-inflight messages.
	// 2. expired message
	// 3. qos0 message
	// 4. the front message
	// see queue.mem for more details.
	Add(elem *Elem) error
	// Replace replaces the PUBLISH with the PUBREL with the same packet id.
	Replace(elem *Elem) (replaced bool, err error)

	// Read reads a batch of new message (non-inflight) from the store.
	// The caller must call ReadInflight first to read all inflight message before calling this method.
	// The size of the batch will be less than or equal to the size of the given packet id list。
	// The message read by the method should always has a 0 value packet id.
	// Calling this method will be block until there are any new message can be read or the store has been closed.
	// If the store has been closed, returns nil, ErrClosed.
	Read(pids []packets.PacketID) ([]*Elem, error)

	// ReadInflight reads at most maxSize inflight messages.
	// The caller must call this method to read all inflight messages before calling Read method.
	// Returning 0 length elems means all inflight messages have been read.
	ReadInflight(maxSize uint) (elems []*Elem, err error)

	// Remove
	Remove(pid packets.PacketID) error
}

//type StoreFactory interface {
//	NewStore(config server.Config, client server.Client) (Store, error)
//	Clean()
//	Close() error
//}

// ElemExpiry return whether the elem is expired
func ElemExpiry(now time.Time, elem *Elem) bool {
	t := time.Time{}
	if elem.Expiry != t {
		return now.After(elem.Expiry)
	}
	return false
}
