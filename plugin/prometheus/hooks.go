package prometheus

import (
	"github.com/DrmagicE/gmqtt/server"
)

func (p *Prometheus) HookWrapper() server.HookWrapper {
	return server.HookWrapper{}
}
