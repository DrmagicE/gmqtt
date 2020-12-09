package admin

import (
	"context"
	"errors"
	"net"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/DrmagicE/gmqtt/config"
	"github.com/DrmagicE/gmqtt/server"
)

const (
	Name = "admin"
)

func init() {
	server.RegisterPlugin(Name, New)
	config.RegisterDefaultPluginConfig(Name, &DefaultConfig)
}

func New(config config.Config) (server.Plugable, error) {
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
	empty := cfg(Config{})
	if v.Admin == empty {
		v.Admin = cfg(DefaultConfig)
	}
	*c = Config(v.Admin)
	return nil
}

// Config is the configuration of api plugin
type Config struct {
	HTTPAddr string
	GRPCAddr string
}

func (c *Config) Validate() error {
	_, _, err := net.SplitHostPort(c.HTTPAddr)
	if err != nil {
		return errors.New("invalid http_addr")
	}
	_, _, err = net.SplitHostPort(c.GRPCAddr)
	if err != nil {
		return errors.New("invalid grpc_addr")
	}
	return nil
}

// DefaultConfig is the default configuration.
var DefaultConfig = Config{
	HTTPAddr: ":8083",
	GRPCAddr: ":8084",
}

type Admin struct {
	config      Config
	httpServer  *http.Server
	grpcServer  *grpc.Server
	statsReader server.StatsReader
	store       *store
}

func (a *Admin) Load(service server.Server) (err error) {
	s := grpc.NewServer()
	a.grpcServer = s
	RegisterClientServiceServer(s, &clientService{a: a})
	RegisterSubscriptionServiceServer(s, &subscriptionService{a: a})
	l, err := net.Listen("tcp", a.config.GRPCAddr)
	if err != nil {
		return err
	}
	go func() {
		err := s.Serve(l)
		if err != nil {
			panic(err)
		}
	}()
	mux := runtime.NewServeMux(runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{OrigName: true, EmitDefaults: true}))

	err = RegisterClientServiceHandlerFromEndpoint(
		context.Background(),
		mux,
		a.config.GRPCAddr,
		[]grpc.DialOption{grpc.WithInsecure()})

	err = RegisterSubscriptionServiceHandlerFromEndpoint(
		context.Background(),
		mux,
		a.config.GRPCAddr,
		[]grpc.DialOption{grpc.WithInsecure()})

	if err != nil {
		return err
	}
	httpServer := &http.Server{
		Handler: mux,
		Addr:    a.config.HTTPAddr,
	}
	a.httpServer = httpServer
	go func() {
		err := httpServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()
	a.statsReader = service.StatsManager()
	a.store = newStore(a.statsReader)
	a.store.subscriptionService = service.SubscriptionService()
	return nil
}

func (a *Admin) Unload() error {
	_ = a.httpServer.Shutdown(context.Background())
	a.grpcServer.Stop()
	return nil
}

func (a *Admin) HookWrapper() server.HookWrapper {
	return server.HookWrapper{
		OnSessionCreatedWrapper:    a.OnSessionCreatedWrapper,
		OnSessionResumedWrapper:    a.OnSessionResumeWrapper,
		OnCloseWrapper:             a.OnCloseWrapper,
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
