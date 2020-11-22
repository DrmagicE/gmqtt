package persistence

import (
	"log"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DrmagicE/gmqtt/persistence/queue"
	queue_test "github.com/DrmagicE/gmqtt/persistence/queue/test"
	"github.com/DrmagicE/gmqtt/persistence/subscription"
	sub_test "github.com/DrmagicE/gmqtt/persistence/subscription/test"
)

func TestRedis(t *testing.T) {
	a := assert.New(t)
	name, err := runContainer()
	if err != nil {
		t.Fatalf("fail to start redis container: %s", err)
		return
	}
	defer func() {
		msg, err := stopContainer(name)
		if err != nil {
			log.Printf("fail to stop container: %s,%s", msg, err)
		}
	}()
	time.Sleep(5 * time.Second) // wait for redis start
	r := &redisFactory{}
	s, err := r.New(queue_test.TestServerConfig, queue_test.TestHooks)
	a.Nil(err)
	err = s.Open()
	a.Nil(err)
	qs, err := s.NewQueueStore(queue_test.TestServerConfig, queue_test.TestClient)
	a.Nil(err)

	testRedisQueue(t, qs)

	subStore, err := s.NewSubscriptionStore(queue_test.TestServerConfig)
	a.Nil(err)

	testRedisSubscriptionStore(t, subStore)

}
func runContainer() (string, error) {
	cmd := exec.Command("/bin/sh", "-c", "docker run -d -p 6379:6379 redis")
	name, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(name), nil
}
func stopContainer(name string) (errMsg string, err error) {
	cmd := exec.Command("/bin/sh", "-c", "docker stop "+name)
	b, err := cmd.Output()
	if err != nil {
		return string(b), err
	}
	cmd = exec.Command("/bin/sh", "-c", "docker rm "+name)
	b, err = cmd.Output()
	if err != nil {
		return string(b), err
	}
	return
}

func testRedisQueue(t *testing.T, store queue.Store) {
	queue_test.TestQueue(t, store)
}

func testRedisSubscriptionStore(t *testing.T, store subscription.Store) {
	sub_test.TestSuite(t, store)
}
