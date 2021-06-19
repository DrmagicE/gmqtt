// +build !windows

package persistence

import (
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/DrmagicE/gmqtt/config"
	queue_test "github.com/DrmagicE/gmqtt/persistence/queue/test"
	sess_test "github.com/DrmagicE/gmqtt/persistence/session/test"
	"github.com/DrmagicE/gmqtt/persistence/subscription"
	sub_test "github.com/DrmagicE/gmqtt/persistence/subscription/test"
	unack_test "github.com/DrmagicE/gmqtt/persistence/unack/test"
	"github.com/DrmagicE/gmqtt/server"
)

var redisConfig config.RedisPersistence

func init() {
	maxIdle := uint(100)
	maxActive := uint(100)
	redisConfig = config.RedisPersistence{
		Addr:        ":6379",
		Password:    "",
		Database:    0,
		MaxIdle:     &maxIdle,
		MaxActive:   &maxActive,
		IdleTimeout: 100 * time.Second,
	}
}

type RedisSuite struct {
	suite.Suite
	p server.Persistence
}

func (s *RedisSuite) SetupTest() {
	_, err := runContainer()
	if err != nil {
		s.Suite.T().Fatalf("fail to start redis container: %s", err)
	}
	time.Sleep(2 * time.Second) // wait for redis start

	p, err := NewRedis(config.Config{
		Persistence: config.Persistence{
			Type:  config.PersistenceTypeRedis,
			Redis: redisConfig,
		},
	})
	if err != nil {
		s.Suite.T().Fatal(err.Error())
	}
	err = p.Open()
	if err != nil {
		s.Suite.T().Fatal("fail to open redis", err)
	}
	s.p = p
}

func (s *RedisSuite) TearDownSuite() {
	stopContainer()
}

func (s *RedisSuite) TestQueue() {
	a := assert.New(s.T())
	cfg := queue_test.TestServerConfig
	cfg.Persistence.Redis = redisConfig
	qs, err := s.p.NewQueueStore(cfg, queue_test.TestNotifier, queue_test.TestClientID)
	a.Nil(err)
	queue_test.TestQueue(s.T(), qs)
}

func (s *RedisSuite) TestSubscription() {
	newFn := func() subscription.Store {
		st, err := s.p.NewSubscriptionStore(config.Config{})
		if err != nil {
			panic(err)
		}
		return st
	}
	sub_test.TestSuite(s.T(), newFn)
}

func (s *RedisSuite) TestSession() {
	a := assert.New(s.T())
	st, err := s.p.NewSessionStore(config.Config{})
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
