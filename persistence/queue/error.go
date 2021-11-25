package queue

import (
	"errors"
)

var (
	ErrClosed                   = errors.New("queue has been closed")
	ErrDropExceedsMaxPacketSize = errors.New("maximum packet size exceeded")
	ErrDropQueueFull            = errors.New("the message queue is full")
	ErrDropExpired              = errors.New("the message is expired")
	ErrDropExpiredInflight      = errors.New("the inflight message is expired")
)

// InternalError wraps the error of the backend storage.
type InternalError struct {
	// Err is the error return by the backend storage.
	Err error
}

func (i *InternalError) Error() string {
	return i.Err.Error()
}
