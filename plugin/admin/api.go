package admin

import (
	"context"
	"errors"
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

const (
	Name = "admin"
)

var log *zap.Logger

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

func (c *Config) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type cfg Config
	var v = &struct {
		Admin cfg `yaml:"admin"`
	}{
		Admin: cfg(DefaultConfig),
	}
	if err := unmarshal(v); err != nil {
		return err
	}
	emptyGRPC := GRPCConfig{}
	if v.Admin.GRPC == emptyGRPC {
		v.Admin.GRPC = DefaultConfig.GRPC
	}
	emptyHTTP := HTTPConfig{}
	if v.Admin.HTTP == emptyHTTP {
		v.Admin.HTTP = DefaultConfig.HTTP
	}
	empty := cfg(Config{})
	if v.Admin == empty {
		v.Admin = cfg(DefaultConfig)
	}
	*c = Config(v.Admin)
	return nil
}

// Config is the configuration of api plugin
type Config struct {
	HTTP HTTPConfig `yaml:"http"`
	GRPC GRPCConfig `yaml:"grpc"`
}

// HTTPConfig is the configuration for http endpoint.
type HTTPConfig struct {
	// Enable indicates whether to expose http endpoint.
	Enable bool `yaml:"enable"`
	// Addr is the address that the http server listen on.
	Addr string `yaml:"http_addr"`
}

// GRPCConfig is the configuration for grpc endpoint.
type GRPCConfig struct {
	// Addr is the address that the grpc server listen on.
	Addr string `yaml:"http_addr"`
}

func (c *Config) Validate() error {
	if c.HTTP.Enable {
		_, _, err := net.SplitHostPort(c.HTTP.Addr)
		if err != nil {
			return errors.New("invalid http_addr")
		}
	}
	_, _, err := net.SplitHostPort(c.GRPC.Addr)
	if err != nil {
		return errors.New("invalid grpc_addr")
	}
	return nil
}

// DefaultConfig is the default configuration.
var DefaultConfig = Config{
	HTTP: HTTPConfig{
		Enable: true,
		Addr:   ":8083",
	},
	GRPC: GRPCConfig{
		Addr: ":8084",
	},
}

// Admin providers grpc and http api that enables the external system to interact with the broker.
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

func (a *Admin) Load(service server.Server) (err error) {
	log = server.LoggerWithField(zap.String("plugin", "admin"))
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

func (a *Admin) HookWrapper() server.HookWrapper {
	return server.HookWrapper{
		OnSessionCreatedWrapper:    a.OnSessionCreatedWrapper,
		OnSessionResumedWrapper:    a.OnSessionResumeWrapper,
		OnClosedWrapper:            a.OnClosedWrapper,
		OnSessionTerminatedWrapper: a.OnSessionTerminatedWrapper,
		OnSubscribedWrapper:        a.OnSubscribedWrapper,
		OnUnsubscribedWrapper:      a.OnUnsubscribedWrapper,
	}
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
