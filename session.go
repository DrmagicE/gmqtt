package gmqtt

import (
	"time"
)

// Session represents a MQTT session.
type Session struct {
	// ClientID represents the client id.
	ClientID string
	// Will is the will message of the client, can be nil if there is no will message.
	Will *Message
	// WillDelayInterval represents the Will Delay Interval in seconds
	WillDelayInterval uint32
	// ConnectedAt is the session create time.
	ConnectedAt time.Time
	// ExpiryInterval represents the Session Expiry Interval in seconds
	ExpiryInterval uint32
}

// IsExpired return whether the session is expired
func (s *Session) IsExpired(now time.Time) bool {
	return s.ConnectedAt.Add(time.Duration(s.ExpiryInterval) * time.Second).Before(now)
}
