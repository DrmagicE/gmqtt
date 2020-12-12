package unack

import (
	"github.com/DrmagicE/gmqtt/pkg/packets"
)

// Store represents a unack store for one client.
// Unack store is used to persist the unacknowledged qos2 messages.
type Store interface {
	// Init will be called when the client connect.
	// If cleanStart set to true, the implementation should remove any associated data in backend store.
	// If it set to false, the implementation should retrieve the associated data from backend store.
	Init(cleanStart bool) error
	// Set sets the given id into store.
	// The return boolean indicates whether the id exist.
	Set(id packets.PacketID) (bool, error)
	// Remove removes the given id from store.
	Remove(id packets.PacketID) error
}
