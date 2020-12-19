package auth

import (
	"context"

	"go.uber.org/zap"

	"github.com/DrmagicE/gmqtt/pkg/codes"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/DrmagicE/gmqtt/server"
)

func (a *Auth) HookWrapper() server.HookWrapper {
	return server.HookWrapper{
		OnBasicAuthWrapper: a.OnBasicAuthWrapper,
	}
}

func (a *Auth) OnBasicAuthWrapper(pre server.OnBasicAuth) server.OnBasicAuth {
	return func(ctx context.Context, client server.Client, req *server.ConnectRequest) (err error) {
		err = pre(ctx, client, req)
		if err != nil {
			return err
		}
		ok, err := a.validate(string(req.Connect.Username), string(req.Connect.Password))
		if err != nil {
			return err
		}
		if !ok {
			log.Debug("authentication failed", zap.String("username", string(req.Connect.Username)))
			switch client.Version() {
			case packets.Version311:
				return &codes.Error{
					Code: codes.V3NotAuthorized,
				}
			case packets.Version5:
				return &codes.Error{
					Code: codes.NotAuthorized,
				}
			}
		}
		return nil
	}
}
