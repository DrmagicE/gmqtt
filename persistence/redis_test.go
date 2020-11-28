package persistence

import (
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	queue_test "github.com/DrmagicE/gmqtt/persistence/queue/test"
	sess_test "github.com/DrmagicE/gmqtt/persistence/session/test"
	sub_test "github.com/DrmagicE/gmqtt/persistence/subscription/test"
	unack_test "github.com/DrmagicE/gmqtt/persistence/unack/test"
	"github.com/DrmagicE/gmqtt/server"
)

type RedisSuite struct {
	suite.Suite
	factory *redisFactory
	p       server.Persistence
}

func (s *RedisSuite) SetupTest() {
	_, err := runContainer()
	if err != nil {
		s.Suite.T().Fatalf("fail to start redis container: %s", err)
	}
	time.Sleep(2 * time.Second) // wait for redis start

	factory := &redisFactory{}
	p, err := factory.New(server.Config{}, queue_test.TestHooks)
	if err != nil {
		s.Suite.T().Fatal(err.Error())
	}
	err = p.Open()
	if err != nil {
		s.Suite.T().Fatal("fail to open redis", err)
	}
	s.factory = factory
	s.p = p
}
func (s *RedisSuite) TearDownSuite() {
	stopContainer()
}

func (s *RedisSuite) TestQueue() {
	a := assert.New(s.T())
	qs, err := s.p.NewQueueStore(queue_test.TestServerConfig, queue_test.TestClientID)
	a.Nil(err)
	queue_test.TestQueue(s.T(), qs)
}
func (s *RedisSuite) TestSubscription() {
	a := assert.New(s.T())
	st, err := s.p.NewSubscriptionStore(server.Config{})
	a.Nil(err)
	sub_test.TestSuite(s.T(), st)
}

func (s *RedisSuite) TestSession() {
	a := assert.New(s.T())
	st, err := s.p.NewSessionStore(server.Config{})
	a.Nil(err)
	sess_test.TestSuite(s.T(), st)
}

func (s *RedisSuite) TestUnack() {
	a := assert.New(s.T())
	st, err := s.p.NewUnackStore(unack_test.TestServerConfig, unack_test.TestClientID)
	a.Nil(err)
	unack_test.TestSuite(s.T(), st)
}

func TestRedis(t *testing.T) {
	suite.Run(t, &RedisSuite{})
}

func runContainer() (string, error) {
	_ = exec.Command("/bin/sh", "-c", "docker rm -f gmqtt-testing").Run()
	cmd := exec.Command("/bin/sh", "-c", "docker run -d --name gmqtt-testing -p 6379:6379 redis")
	name, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(name), nil
}
func stopContainer() {
	_ = exec.Command("/bin/sh", "-c", "docker rm -f gmqtt-testing").Run()
}
