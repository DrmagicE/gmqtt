package admin

import (
	"context"
	"net"
	"net/http"

	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/DrmagicE/gmqtt/config"
	"github.com/DrmagicE/gmqtt/server"
)

var _ server.Plugin = (*Admin)(nil)

const Name = "admin"

func init() {
	server.RegisterPlugin(Name, New)
	config.RegisterDefaultPluginConfig(Name, &DefaultConfig)
}

func New(config config.Config) (server.Plugin, error) {
	cfg := config.Plugins[Name].(*Config)
	return &Admin{
		config: *cfg,
	}, nil
}

var log *zap.Logger

// GRPCGatewayRegister provides the ability to share the gRPC and HTTP server to other plugins.
type GRPCGatewayRegister interface {
	GRPCRegister
	HTTPRegister
}

// GRPCRegister is the interface that enable the implement to expose gRPC endpoint.
type GRPCRegister interface {
	// RegisterGRPC registers the gRPC handler into gRPC server which created by admin plugin.
	RegisterGRPC(s grpc.ServiceRegistrar)
}

// HTTPRegister is the interface that enable the implement to expose HTTP endpoint.
type HTTPRegister interface {
	// RegisterHTTP registers the http handler into http server which created by admin plugin.
	RegisterHTTP(ctx context.Context, mux *runtime.ServeMux, endpoint string, opts []grpc.DialOption) (err error)
}

// Admin providers gRPC and HTTP API that enables the external system to interact with the broker.
type Admin struct {
	config        Config
	httpServer    *http.Server
	grpcServer    *grpc.Server
	statsReader   server.StatsReader
	publisher     server.Publisher
	clientService server.ClientService
	store         *store
}

func (a *Admin) registerHTTP(mux *runtime.ServeMux) (err error) {
	err = RegisterClientServiceHandlerFromEndpoint(
		context.Background(),
		mux,
		a.config.GRPC.Addr,
		[]grpc.DialOption{grpc.WithInsecure()},
	)
	if err != nil {
		return err
	}

	err = RegisterSubscriptionServiceHandlerFromEndpoint(
		context.Background(),
		mux,
		a.config.GRPC.Addr,
		[]grpc.DialOption{grpc.WithInsecure()},
	)
	if err != nil {
		return err
	}
	err = RegisterPublishServiceHandlerFromEndpoint(
		context.Background(),
		mux,
		a.config.GRPC.Addr,
		[]grpc.DialOption{grpc.WithInsecure()},
	)

	if err != nil {
		return err
	}
	httpServer := &http.Server{
		Handler: mux,
		Addr:    a.config.HTTP.Addr,
	}
	a.httpServer = httpServer
	go func() {
		err := httpServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()
	return nil
}

func (a *Admin) Load(service server.Server) error {
	log = server.LoggerWithField(zap.String("plugin", Name))
	s := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			grpc_zap.UnaryServerInterceptor(log),
			grpc_prometheus.UnaryServerInterceptor),
	)
	a.grpcServer = s

	RegisterClientServiceServer(s, &clientService{a: a})
	RegisterSubscriptionServiceServer(s, &subscriptionService{a: a})
	RegisterPublishServiceServer(s, &publisher{a: a})
	mux := runtime.NewServeMux(runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{OrigName: true, EmitDefaults: true}))
	if a.config.HTTP.Enable {
		err := a.registerHTTP(mux)
		if err != nil {
			return err
		}
	}

	for _, v := range service.Plugins() {
		if v, ok := v.(GRPCRegister); ok {
			v.RegisterGRPC(s)
		}

		if v, ok := v.(HTTPRegister); a.config.HTTP.Enable && ok {
			err := v.RegisterHTTP(context.Background(),
				mux,
				a.config.GRPC.Addr,
				[]grpc.DialOption{grpc.WithInsecure()})
			if err != nil {
				return err
			}
		}

	}
	l, err := net.Listen("tcp", a.config.GRPC.Addr)
	if err != nil {
		return err
	}
	grpc_prometheus.Register(s)
	a.statsReader = service.StatsManager()
	a.store = newStore(a.statsReader)
	a.store.subscriptionService = service.SubscriptionService()
	a.publisher = service.Publisher()
	a.clientService = service.ClientService()
	go func() {
		err := s.Serve(l)
		if err != nil {
			panic(err)
		}
	}()
	return err
}

func (a *Admin) Unload() error {
	if a.httpServer != nil {
		_ = a.httpServer.Shutdown(context.Background())
	}
	a.grpcServer.Stop()
	return nil
}

func (a *Admin) Name() string {
	return Name
}
