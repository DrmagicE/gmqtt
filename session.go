package gmqtt

import (
	"time"
)

type Session struct {
	// ClientID
	ClientID string
	// Will is the will message of the client, can be nil if there is no will message.
	Will              *Message
	WillDelayInterval uint32
	ConnectedAt       time.Time
	ExpiryInterval    uint32
}

func (s *Session) IsExpired(now time.Time) bool {
	return s.ConnectedAt.Add(time.Duration(s.ExpiryInterval) * time.Second).Before(now)
}
