package session

import (
	"github.com/DrmagicE/gmqtt"
)

// IterateFn is the callback function used by Iterate()
// Return false means to stop the iteration.
type IterateFn func(session *gmqtt.Session) bool

type Store interface {
	Set(session *gmqtt.Session) error
	Remove(clientID string) error
	Get(clientID string) (*gmqtt.Session, error)
	Iterate(fn IterateFn) error
	SetSessionExpiry(clientID string, expiry uint32) error
}
