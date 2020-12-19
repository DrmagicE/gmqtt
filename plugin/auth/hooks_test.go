package auth

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/DrmagicE/gmqtt/config"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/DrmagicE/gmqtt/server"
)

func TestAuth_OnBasicAuthWrapper(t *testing.T) {
	a := assert.New(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	path := "./testdata/gmqtt_password.yml"
	cfg := DefaultConfig
	cfg.PasswordFile = path
	cfg.Hash = Plain
	auth, err := New(config.Config{
		Plugins: map[string]config.Configuration{
			"auth": &cfg,
		},
	})
	mockClient := server.NewMockClient(ctrl)
	mockClient.EXPECT().Version().Return(packets.Version311).AnyTimes()
	a.Nil(err)
	a.Nil(auth.Load(nil))
	au := auth.(*Auth)
	var preCalled bool
	fn := au.OnBasicAuthWrapper(func(ctx context.Context, client server.Client, req *server.ConnectRequest) (err error) {
		preCalled = true
		return nil
	})
	// pass
	a.Nil(fn(context.Background(), mockClient, &server.ConnectRequest{
		Connect: &packets.Connect{
			Username: []byte("u1"),
			Password: []byte("p1"),
		},
	}))
	a.True(preCalled)

	// fail
	a.NotNil(fn(context.Background(), mockClient, &server.ConnectRequest{
		Connect: &packets.Connect{
			Username: []byte("u1"),
			Password: []byte("p11"),
		},
	}))

	a.Nil(au.Unload())
}
