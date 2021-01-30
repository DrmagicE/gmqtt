package federation

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestConfig_Validate(t *testing.T) {

	var tt = []struct {
		name     string
		cfg      *Config
		expected *Config
		valid    bool
	}{
		{
			name: "invalid1",
			cfg: &Config{
				NodeName:         "name1",
				FedAddr:          "",
				AdvertiseFedAddr: "127.0.0.1:1234",
				GossipAddr:       "127.0.0.1:1235",
				RetryJoin:        nil,
				RetryInterval:    0,
				RetryTimeout:     0,
				SnapshotPath:     "",
				RejoinAfterLeave: false,
			},
			valid: false,
		},
		{
			name: "invalid2",
			cfg: &Config{
				NodeName:         "name2",
				FedAddr:          "127.0.0.1:1233",
				AdvertiseFedAddr: "127.0.0.1:1234",
				GossipAddr:       "127.0.0.1:1235",
				RetryJoin:        nil,
				RetryInterval:    0,
				RetryTimeout:     0,
				SnapshotPath:     "",
				RejoinAfterLeave: false,
			},
			valid: false,
		},
		{
			name: "invalid3",
			cfg: &Config{
				NodeName:         "name2",
				FedAddr:          "127.0.0.1:",
				AdvertiseFedAddr: "127.0.0.1:1234",
				GossipAddr:       "127.0.0.1:1235",
				RetryJoin:        nil,
				RetryInterval:    1,
				RetryTimeout:     2,
				SnapshotPath:     "",
				RejoinAfterLeave: false,
			},
			valid: false,
		},
		{
			name: "invalid4",
			cfg: &Config{
				NodeName:         "name2",
				FedAddr:          "127.0.0.1:1234:",
				AdvertiseFedAddr: "127.0.0.1:1234",
				GossipAddr:       "127.0.0.1:1235",
				RetryJoin:        nil,
				RetryInterval:    1,
				RetryTimeout:     2,
				SnapshotPath:     "",
				RejoinAfterLeave: false,
			},
			valid: false,
		},
		{
			name: "addDefaultPortIPv4",
			cfg: &Config{
				NodeName:         "name2",
				FedAddr:          "127.0.0.1",
				AdvertiseFedAddr: "127.0.0.1",
				GossipAddr:       "127.0.0.1",
				RetryJoin:        []string{"127.0.0.1", "127.0.0.2"},
				RetryInterval:    1,
				RetryTimeout:     2,
				SnapshotPath:     "",
				RejoinAfterLeave: false,
			},
			expected: &Config{
				NodeName:         "name2",
				FedAddr:          "127.0.0.1" + DefaultFedAddr,
				AdvertiseFedAddr: "127.0.0.1" + DefaultFedAddr,
				GossipAddr:       "127.0.0.1" + DefaultGossipAddr,
				RetryJoin:        []string{"127.0.0.1" + DefaultGossipAddr, "127.0.0.2" + DefaultGossipAddr},
				RetryInterval:    1,
				RetryTimeout:     2,
				SnapshotPath:     "",
				RejoinAfterLeave: false,
			},
			valid: true,
		},
		{
			name: "addDefaultPortIPv6",
			cfg: &Config{
				NodeName:         "name2",
				FedAddr:          "[::1]",
				AdvertiseFedAddr: "[::1]:1234",
				GossipAddr:       "127.0.0.1",
				RetryJoin:        []string{"127.0.0.1", "127.0.0.2"},
				RetryInterval:    1,
				RetryTimeout:     2,
				SnapshotPath:     "",
				RejoinAfterLeave: false,
			},
			expected: &Config{
				NodeName:         "name2",
				FedAddr:          "[::1]" + DefaultFedAddr,
				AdvertiseFedAddr: "[::1]:1234",
				GossipAddr:       "127.0.0.1" + DefaultGossipAddr,
				RetryJoin:        []string{"127.0.0.1" + DefaultGossipAddr, "127.0.0.2" + DefaultGossipAddr},
				RetryInterval:    1,
				RetryTimeout:     2,
				SnapshotPath:     "",
				RejoinAfterLeave: false,
			},
			valid: true,
		},
	}
	for _, v := range tt {
		t.Run(v.name, func(t *testing.T) {
			a := assert.New(t)
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			err := v.cfg.Validate()
			if v.valid {
				a.NoError(err)
				a.Equal(v.expected, v.cfg)
				return
			}
			a.Error(err)
		})
	}

}
