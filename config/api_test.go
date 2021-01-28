package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAPI_Validate(t *testing.T) {
	a := assert.New(t)

	tt := []struct {
		cfg   API
		valid bool
	}{
		{
			cfg: API{
				GRPC: []*Endpoint{
					{
						Address: "udp://127.0.0.1",
					},
				},
				HTTP: []*Endpoint{
					{},
				},
			},
			valid: false,
		},
		{
			cfg: API{
				GRPC: []*Endpoint{
					{
						Address: "tcp://127.0.0.1:1234",
					},
				},
				HTTP: []*Endpoint{
					{
						Address: "udp://127.0.0.1",
					},
				},
			},
			valid: false,
		},
		{
			cfg: API{
				GRPC: []*Endpoint{
					{
						Address: "tcp://127.0.0.1:1234",
					},
				},
			},
			valid: true,
		},
		{
			cfg: API{
				GRPC: []*Endpoint{
					{
						Address: "tcp://127.0.0.1:1234",
					},
				},
				HTTP: []*Endpoint{
					{
						Address: "tcp://127.0.0.1:1235",
					},
				},
			},
			valid: false,
		},
		{
			cfg: API{
				GRPC: []*Endpoint{
					{
						Address: "unix:///var/run/gmqttd.sock",
					},
				},
				HTTP: []*Endpoint{
					{
						Address: "tcp://127.0.0.1:1235",
						Map:     "unix:///var/run/gmqttd.sock",
					},
				},
			},
			valid: true,
		},
	}
	for _, v := range tt {
		err := v.cfg.Validate()
		if v.valid {
			a.NoError(err)
		} else {
			a.Error(err)
		}
	}

}
