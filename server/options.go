package server

import (
	"net"

	"github.com/DrmagicE/gmqtt/config"
	"github.com/DrmagicE/gmqtt/retained"
	"go.uber.org/zap"
)

type Options func(srv *server)

// WithConfig set the config of the server
func WithConfig(config config.Config) Options {
	return func(srv *server) {
		srv.config = config
	}
}

// WithTCPListener set  tcp listener(s) of the server. Default listen on  :1883.
func WithTCPListener(lns ...net.Listener) Options {
	return func(srv *server) {
		srv.tcpListener = append(srv.tcpListener, lns...)
	}
}

// WithWebsocketServer set  websocket server(s) of the server.
func WithWebsocketServer(ws ...*WsServer) Options {
	return func(srv *server) {
		srv.websocketServer = ws
	}
}

// WithPlugin set plugin(s) of the server.
func WithPlugin(plugin ...Plugin) Options {
	return func(srv *server) {
		srv.plugins = append(srv.plugins, plugin...)
	}
}

// WithHook set hooks of the server. Notice: WithPlugin() will overwrite hooks.
func WithHook(hooks Hooks) Options {
	return func(srv *server) {
		srv.hooks = hooks
	}
}

func WithLogger(logger *zap.Logger) Options {
	return func(srv *server) {
		zaplog = logger
	}
}

// WithRetainedStore set retained db of the server. Notice: WithRetainedStore(s) will overwrite retainedDB.
func WithRetainedStore(store retained.Store) Options {
	return func(srv *server) {
		srv.retainedDB = store
	}
}
