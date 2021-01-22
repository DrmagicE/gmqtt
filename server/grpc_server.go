package server

import (
	"net"
	"strings"

	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"go.uber.org/zap"
)

func listenGRPCAddr(addr string) (net.Listener, error) {
	addrParts := strings.SplitN(addr, "://", 2)
	if len(addrParts) == 1 && addrParts[0] != "" {
		addrParts = []string{"tcp", addrParts[0]}
	}
	return net.Listen(addrParts[0], addrParts[1])
}

func (srv *server) serveGRPC() {
	defer srv.wg.Done()
	l, err := listenGRPCAddr(srv.config.GRPC.Endpoint)
	if err != nil {
		return
	}
	grpc_prometheus.Register(srv.grpcServer)
	errChan := make(chan error)
	go func() {
		errChan <- srv.grpcServer.Serve(l)
	}()
	defer srv.grpcServer.Stop()
	zaplog.Info("gRPC server started", zap.String("endpoint", srv.config.GRPC.Endpoint))
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
