package redis

import (
	"fmt"
	"testing"
	"time"

	redigo "github.com/gomodule/redigo/redis"

	"github.com/DrmagicE/gmqtt"
)

func newPool(addr string) *redigo.Pool {
	return &redigo.Pool{
		MaxIdle:     3,
		IdleTimeout: 240 * time.Second,
		// Dial or DialContext must be set. When both are set, DialContext takes precedence over Dial.
		Dial: func() (redigo.Conn, error) { return redigo.Dial("tcp", addr) },
	}
}

func TestGetAll(t *testing.T) {
	s := New(newPool(":6379"))
	s.Iterate(func(session *gmqtt.Session) bool {
		fmt.Println(session)
		return true
	})
}
