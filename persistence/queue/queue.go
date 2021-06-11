package queue

import (
	"time"

	"github.com/DrmagicE/gmqtt/pkg/packets"
)

// InitOptions is used to pass some required client information to the queue.Init()
type InitOptions struct {
	// CleanStart is the cleanStart field in the connect packet.
	CleanStart bool
	// Version is the client MQTT protocol version.
	Version packets.Version
	// ReadBytesLimit indicates the maximum publish size that is allow to read.
	ReadBytesLimit uint32
	Notifier       Notifier
}

// Store represents a queue store for one client.
type Store interface {
	// Close will be called when the client disconnect.
	// This method must unblock the Read method.
	Close() error
	// Init will be called when the client connect.
	// If opts.CleanStart set to true, the implementation should remove any associated data in backend store.
	// If it sets to false, the implementation should be able to retrieve the associated data from backend store.
	// The opts.version indicates the protocol version of the connected client, it is mainly used to calculate the publish packet size.
	Init(opts *InitOptions) error
	Clean() error
	// Add inserts a elem to the queue.
	// When the len of queue is reaching the maximum setting, the implementation should drop messages according the following priorities:
	// 1. Drop the expired inflight message.
	// 2. Drop the current elem if there is no more non-inflight messages.
	// 3. Drop expired non-inflight message.
	// 4. Drop qos0 message.
	// 5. Drop the front message.
	// See queue.mem for more details.
	Add(elem *Elem) error
	// Replace replaces the PUBLISH with the PUBREL with the same packet id.
	Replace(elem *Elem) (replaced bool, err error)

	// Read reads a batch of new message (non-inflight) from the store. The qos0 messages will be removed after read.
	// The size of the batch will be less than or equal to the size of the given packet id list.
	// The implementation must remove and do not return any :
	// 1. expired messages
	// 2. publish message which exceeds the InitOptions.ReadBytesLimit
	// while reading.
	// The caller must call ReadInflight first to read all inflight message before calling this method.
	// Calling this method will be blocked until there are any new messages can be read or the store has been closed.
	// If the store has been closed, returns nil, ErrClosed.
	Read(pids []packets.PacketID) ([]*Elem, error)

	// ReadInflight reads at most maxSize inflight messages.
	// The caller must call this method to read all inflight messages before calling Read method.
	// Returning 0 length elems means all inflight messages have been read.
	ReadInflight(maxSize uint) (elems []*Elem, err error)

	// Remove removes the elem for a given id.
	Remove(pid packets.PacketID) error
}

type Notifier interface {
	// NotifyDropped will be called when the element in the queue is dropped.
	// The err indicates the reason of why it is dropped.
	// The MessageWithID field in elem param can be queue.Pubrel or queue.Publish.
	NotifyDropped(elem *Elem, err error)
	NotifyInflightAdded(delta int)
	NotifyMsgQueueAdded(delta int)
}

// ElemExpiry return whether the elem is expired
func ElemExpiry(now time.Time, elem *Elem) bool {
	if !elem.Expiry.IsZero() {
		return now.After(elem.Expiry)
	}
	return false
}
