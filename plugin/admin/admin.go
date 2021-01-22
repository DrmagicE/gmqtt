package admin

import (
	"context"
	"net/http"

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

// Admin providers gRPC and HTTP API that enables the external system to interact with the broker.
type Admin struct {
	config        Config
	httpServer    *http.Server
	statsReader   server.StatsReader
	publisher     server.Publisher
	clientService server.ClientService
	store         *store
}

var ABC = func(ctx context.Context, mux *runtime.ServeMux, endpoint string, opts []grpc.DialOption) (err error) {
}

// TODO
func (a *Admin) registerHTTP(mux *runtime.ServeMux) (err error) {
	err = RegisterClientServiceHandlerFromEndpoint(
		context.Background(),
		mux,
		"unix:./gmqttd.sock",
		[]grpc.DialOption{grpc.WithInsecure()},
	)
	if err != nil {
		return err
	}

	err = RegisterSubscriptionServiceHandlerFromEndpoint(
		context.Background(),
		mux,
		"unix:./gmqttd.sock",
		[]grpc.DialOption{grpc.WithInsecure()},
	)
	if err != nil {
		return err
	}
	err = RegisterPublishServiceHandlerFromEndpoint(
		context.Background(),
		mux,
		"unix:./gmqttd.sock",
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
	gRPCReg := service.GRPCRegistrar()

	log = server.LoggerWithField(zap.String("plugin", Name))

	RegisterClientServiceServer(gRPCReg, &clientService{a: a})
	RegisterSubscriptionServiceServer(gRPCReg, &subscriptionService{a: a})
	RegisterPublishServiceServer(gRPCReg, &publisher{a: a})
	mux := runtime.NewServeMux(runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{OrigName: true, EmitDefaults: true}))
	if a.config.HTTP.Enable {
		err := a.registerHTTP(mux)
		if err != nil {
			return err
		}
	}

	//for _, v := range service.Plugins() {
	//	if v, ok := v.(GRPCRegister); ok {
	//		v.RegisterGRPC(s)
	//	}
	//
	//	if v, ok := v.(HTTPRegister); a.config.HTTP.Enable && ok {
	//		err := v.RegisterHTTP(context.Background(),
	//			mux,
	//			a.config.GRPC.Addr,
	//			[]grpc.DialOption{grpc.WithInsecure()})
	//		if err != nil {
	//			return err
	//		}
	//	}
	//
	//}
	a.statsReader = service.StatsManager()
	a.store = newStore(a.statsReader)
	a.store.subscriptionService = service.SubscriptionService()
	a.publisher = service.Publisher()
	a.clientService = service.ClientService()
	return nil
}

func (a *Admin) Unload() error {
	if a.httpServer != nil {
		_ = a.httpServer.Shutdown(context.Background())
	}
	return nil
}

func (a *Admin) Name() string {
	return Name
}
