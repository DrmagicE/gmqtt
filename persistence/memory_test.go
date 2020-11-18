package persistence

import (
	"testing"

	"github.com/DrmagicE/gmqtt/persistence/test_suite"
)

func TestQueue(t *testing.T) {
	test_suite.TestQueue(t, &memoryFactory{})
}
