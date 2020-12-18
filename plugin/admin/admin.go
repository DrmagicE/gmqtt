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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/DrmagicE/gmqtt/config"
	"github.com/DrmagicE/gmqtt/server"
)

var _ server.Plugin = (*Admin)(nil)

const Name = "admin"

func init() {
	server.RegisterPlugin(Name, New)
	config.RegisterDefaultPluginConfig(Name, &DefaultConfig)
}

func New(ctx context.Context, config config.Config) (server.Plugin, error) {
	cfg := config.Plugins[Name].(*Config)
	return &Admin{
		config: *cfg,
	}, nil
}

var log *zap.Logger

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

func (a *Admin) registerHTTP() (err error) {
	mux := runtime.NewServeMux(runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{OrigName: true, EmitDefaults: true}))

	err = RegisterClientServiceHandlerFromEndpoint(
		context.Background(),
		mux,
		a.config.GRPC.Addr,
		[]grpc.DialOption{grpc.WithInsecure()})

	err = RegisterSubscriptionServiceHandlerFromEndpoint(
		context.Background(),
		mux,
		a.config.GRPC.Addr,
		[]grpc.DialOption{grpc.WithInsecure()})

	err = RegisterPublishServiceHandlerFromEndpoint(
		context.Background(),
		mux,
		a.config.GRPC.Addr,
		[]grpc.DialOption{grpc.WithInsecure()})

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
	if a.config.HTTP.Enable {
		err = a.registerHTTP()
	}
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

func getPage(reqPage, reqPageSize uint32) (page, pageSize uint) {
	page = 1
	pageSize = 20
	if reqPage != 0 {
		page = uint(reqPage)
	}
	if reqPageSize != 0 {
		pageSize = uint(reqPageSize)
	}
	return
}

func InvalidArgument(name string, msg string) error {
	errString := "invalid " + name
	if msg != "" {
		errString = errString + ":" + msg
	}
	return status.Error(codes.InvalidArgument, errString)
}
