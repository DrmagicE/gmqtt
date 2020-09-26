package topicalias

import (
	"github.com/DrmagicE/gmqtt/server"
	"github.com/DrmagicE/gmqtt/topicalias/fifo"
)

type defaultFactory struct {
}

func (d *defaultFactory) New() server.TopicAliasManager {
	return fifo.New()
}

func init() {
	server.DefaultTopicAliasMgrFactory = &defaultFactory{}
}
