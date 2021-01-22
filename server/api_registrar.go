package server

import (
	"context"
	"net"
	"net/http"
	"strings"

	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	gcodes "google.golang.org/grpc/codes"

	"github.com/DrmagicE/gmqtt/config"
)

// APIRegistrar is the registrar for all gRPC servers and HTTP servers.
// It provides the ability for plugins to register gRPC and HTTP handler.
type APIRegistrar interface {
	// RegisterHTTPHandler registers the handler to all http servers.
	RegisterHTTPHandler(fn HTTPHandler) error
	// RegisterService registers a service and its implementation to all gRPC servers.
	RegisterService(desc *grpc.ServiceDesc, impl interface{})
}

type apiRegistrar struct {
	gRPCServers []*gRPCServer
	httpServers []*httpServer
}

func (a *apiRegistrar) RegisterService(desc *grpc.ServiceDesc, impl interface{}) {
	for _, v := range a.gRPCServers {
		v.server.RegisterService(desc, impl)
	}
}

func (a *apiRegistrar) RegisterHTTPHandler(fn HTTPHandler) error {
	var err error
	for _, v := range a.httpServers {
		schema, addr := splitEndpoint(v.gRPCEndpoint)
		if schema == "unix" {
			err = fn(context.Background(), v.mux, v.gRPCEndpoint, []grpc.DialOption{grpc.WithInsecure()})
			if err != nil {
				return err
			}
			continue
		}
		err = fn(context.Background(), v.mux, addr, []grpc.DialOption{grpc.WithInsecure()})
		if err != nil {
			return err
		}
	}
	return nil
}

type gRPCServer struct {
	server   *grpc.Server
	serve    func(errChan chan error) error
	shutdown func()
	endpoint string
}

type httpServer struct {
	gRPCEndpoint string
	endpoint     string
	mux          *runtime.ServeMux
	serve        func(errChan chan error) error
	shutdown     func()
}

// HTTPHandler is the http handler defined by gRPC-gateway.
type HTTPHandler = func(ctx context.Context, mux *runtime.ServeMux, endpoint string, opts []grpc.DialOption) (err error)

func splitEndpoint(endpoint string) (schema string, addr string) {
	epParts := strings.SplitN(endpoint, "://", 2)
	if len(epParts) == 1 && epParts[0] != "" {
		epParts = []string{"tcp", epParts[0]}
	}
	return epParts[0], epParts[1]
}

func buildGRPCServer(endpoint *config.Endpoint) *gRPCServer {
	server := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			grpc_zap.UnaryServerInterceptor(zaplog, grpc_zap.WithLevels(func(code gcodes.Code) zapcore.Level {
				if code == gcodes.OK {
					return zapcore.DebugLevel
				}
				return grpc_zap.DefaultClientCodeToLevel(code)
			})),
			grpc_prometheus.UnaryServerInterceptor),
	)
	grpc_prometheus.Register(server)
	shutdown := func() {
		server.Stop()
	}
	serve := func(errChan chan error) error {
		schema, addr := splitEndpoint(endpoint.Address)
		l, err := net.Listen(schema, addr)
		if err != nil {
			return err
		}
		go func() {
			select {
			case errChan <- server.Serve(l):
			default:
			}
		}()
		return nil
	}

	return &gRPCServer{
		server:   server,
		serve:    serve,
		shutdown: shutdown,
		endpoint: endpoint.Address,
	}
}

func buildHTTPServer(endpoint *config.Endpoint) *httpServer {
	mux := runtime.NewServeMux(runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{OrigName: true, EmitDefaults: true}))
	server := &http.Server{
		Handler: mux,
	}
	shutdown := func() {
		server.Shutdown(context.Background())
	}
	serve := func(errChan chan error) error {
		schema, addr := splitEndpoint(endpoint.Address)
		l, err := net.Listen(schema, addr)
		if err != nil {
			return err
		}
		go func() {
			select {
			case errChan <- server.Serve(l):
			default:
			}
		}()

		return nil
	}

	return &httpServer{
		gRPCEndpoint: endpoint.Map,
		mux:          mux,
		serve:        serve,
		shutdown:     shutdown,
		endpoint:     endpoint.Address,
	}
}

func (srv *server) serveAPIServer() {
	defer func() {
		srv.wg.Done()
		srv.Stop(context.Background())
	}()
	var err error
	errChan := make(chan error, 1)
	defer func() {
		if err != nil {
			zaplog.Error("serveAPIServer error", zap.Error(err))
		}
		for _, v := range srv.apiRegistrar.gRPCServers {
			v.shutdown()
		}
		for _, v := range srv.apiRegistrar.httpServers {
			v.shutdown()
		}
	}()
	for _, v := range srv.apiRegistrar.gRPCServers {
		err = v.serve(errChan)
		if err != nil {
			return
		}
		zaplog.Info("gRPC server started", zap.String("bind_address", v.endpoint))
	}

	for _, v := range srv.apiRegistrar.httpServers {
		err = v.serve(errChan)
		if err != nil {
			return
		}
		zaplog.Info("HTTP server started", zap.String("bind_address", v.endpoint), zap.String("gRPC_endpoint", v.gRPCEndpoint))
	}

	for {
		select {
		case <-srv.exitChan:
			return
		case err := <-errChan:
			if err != nil {
				zaplog.Error("gRPC server stop error", zap.Error(err))
			}
			return
		}

	}
}
