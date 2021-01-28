package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
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
	"google.golang.org/grpc/credentials"

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

// RegisterService implements APIRegistrar interface
func (a *apiRegistrar) RegisterService(desc *grpc.ServiceDesc, impl interface{}) {
	for _, v := range a.gRPCServers {
		v.server.RegisterService(desc, impl)
	}
}

// RegisterHTTPHandler implements APIRegistrar interface
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
	tlsCfg       *tls.Config
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

func buildTLSConfig(cfg *config.TLSOptions) (*tls.Config, error) {
	c, err := tls.LoadX509KeyPair(cfg.Cert, cfg.Key)
	if err != nil {
		return nil, err
	}
	certPool := x509.NewCertPool()
	if cfg.CACert != "" {
		b, err := ioutil.ReadFile(cfg.CACert)
		if err != nil {
			return nil, err
		}
		certPool.AppendCertsFromPEM(b)
	}
	var cliAuthType tls.ClientAuthType
	if cfg.Verify {
		cliAuthType = tls.RequireAndVerifyClientCert
	}
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{c},
		ClientCAs:    certPool,
		ClientAuth:   cliAuthType,
	}
	return tlsCfg, nil
}

func buildGRPCServer(endpoint *config.Endpoint) (*gRPCServer, error) {
	var cred credentials.TransportCredentials
	if cfg := endpoint.TLS; cfg != nil {
		tlsCfg, err := buildTLSConfig(cfg)
		if err != nil {
			return nil, err
		}
		cred = credentials.NewTLS(tlsCfg)
	}
	server := grpc.NewServer(
		grpc.Creds(cred),
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
	}, nil
}

func buildHTTPServer(endpoint *config.Endpoint) (*httpServer, error) {
	var tlsCfg *tls.Config
	var err error
	if cfg := endpoint.TLS; cfg != nil {
		tlsCfg, err = buildTLSConfig(cfg)
		if err != nil {
			return nil, err
		}
	}
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
		if tlsCfg != nil {
			l = tls.NewListener(l, tlsCfg)
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
	}, nil
}

func (srv *server) exit() {
	select {
	case <-srv.exitChan:
	default:
		close(srv.exitChan)
	}
}

func (srv *server) serveAPIServer() {
	var err error
	defer func() {
		srv.wg.Done()
		if err != nil {
			zaplog.Error("serveAPIServer error", zap.Error(err))
			srv.setError(err)
		}
	}()
	errChan := make(chan error, 1)
	defer func() {
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
		case err = <-errChan:
			return
		}

	}
}
