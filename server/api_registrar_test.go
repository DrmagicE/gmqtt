package server

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

func TestAPIRegistrar_serveAPIServer(t *testing.T) {
	a := assert.New(t)
	var serveCount, shutdownCount int
	srv := &server{
		wg:       sync.WaitGroup{},
		exitChan: make(chan struct{}),
		apiRegistrar: &apiRegistrar{
			gRPCServers: []*gRPCServer{
				{
					server: &grpc.Server{},
					serve: func(errChan chan error) error {
						serveCount++
						return nil
					},
					shutdown: func() {
						shutdownCount++
					},
					endpoint: "tcp://127.0.0.1:1234",
				},
			},
			httpServers: []*httpServer{
				{
					gRPCEndpoint: "tcp://127.0.0.1:1234",
					serve: func(errChan chan error) error {
						serveCount++
						return nil
					},
					shutdown: func() {
						shutdownCount++
					},
					endpoint: "tcp://127.0.0.1:1234",
				},
			},
		},
	}
	srv.wg.Add(1)
	close(srv.exitChan)
	srv.serveAPIServer()
	done := make(chan struct{})
	go func() {
		srv.wg.Wait()
		close(done)
	}()
	select {
	case <-time.After(1 * time.Second):
		t.Fatal("serveAPIServer should exit immediately")
	case <-done:
	}
	a.Equal(2, serveCount)
	a.Equal(2, shutdownCount)

}

func TestAPIRegistrar_serveAPIServer_WithError(t *testing.T) {
	a := assert.New(t)
	var serveCount, shutdownCount int
	srv := &server{
		wg:       sync.WaitGroup{},
		exitChan: make(chan struct{}),
		apiRegistrar: &apiRegistrar{
			gRPCServers: []*gRPCServer{
				{
					server: &grpc.Server{},
					serve: func(errChan chan error) error {
						serveCount++
						return errors.New("some thing wrong")
					},
					shutdown: func() {
						shutdownCount++
					},
					endpoint: "tcp://127.0.0.1:1234",
				},
			},
			httpServers: []*httpServer{
				{
					gRPCEndpoint: "tcp://127.0.0.1:1234",
					serve: func(errChan chan error) error {
						serveCount++
						return nil
					},
					shutdown: func() {
						shutdownCount++
					},
					endpoint: "tcp://127.0.0.1:1234",
				},
			},
		},
	}
	srv.wg.Add(1)
	srv.serveAPIServer()
	done := make(chan struct{})
	go func() {
		srv.wg.Wait()
		close(done)
	}()
	select {
	case <-time.After(1 * time.Second):
		t.Fatal("serveAPIServer should exit immediately")
	case <-done:
	}
	a.Equal(1, serveCount)
	a.Equal(2, shutdownCount)
}

func TestAPIRegistrar_serveAPIServer_WithErrorChan(t *testing.T) {
	a := assert.New(t)
	var serveCount, shutdownCount int
	srv := &server{
		wg:       sync.WaitGroup{},
		exitChan: make(chan struct{}),
		apiRegistrar: &apiRegistrar{
			gRPCServers: []*gRPCServer{
				{
					server: &grpc.Server{},
					serve: func(errChan chan error) error {
						errChan <- errors.New("something wrong")
						return nil
					},
					shutdown: func() {
						shutdownCount++
					},
					endpoint: "tcp://127.0.0.1:1234",
				},
			},
			httpServers: []*httpServer{
				{
					gRPCEndpoint: "tcp://127.0.0.1:1234",
					serve: func(errChan chan error) error {
						serveCount++
						return nil
					},
					shutdown: func() {
						shutdownCount++
					},
					endpoint: "tcp://127.0.0.1:1234",
				},
			},
		},
	}
	srv.wg.Add(1)
	srv.serveAPIServer()
	done := make(chan struct{})
	go func() {
		srv.wg.Wait()
		close(done)
	}()
	select {
	case <-time.After(1 * time.Second):
		t.Fatal("serveAPIServer should exit immediately")
	case <-done:
	}
	a.Equal(1, serveCount)
	a.Equal(2, shutdownCount)
}

func TestApiRegistrar_RegisterHTTPHandler(t *testing.T) {
	a := assert.New(t)
	// test unix socket
	reg := &apiRegistrar{
		httpServers: []*httpServer{
			{
				gRPCEndpoint: "unix:///var/run/gmqttd.sock",
				endpoint:     "",
				mux:          &runtime.ServeMux{},
				serve: func(errChan chan error) error {
					return nil
				},
				shutdown: func() {
					return
				},
			},
		},
	}
	a.NoError(reg.RegisterHTTPHandler(func(ctx context.Context, mux *runtime.ServeMux, endpoint string, opts []grpc.DialOption) (err error) {
		a.Equal(reg.httpServers[0].gRPCEndpoint, endpoint)
		return nil
	}))

	// test tcp socket
	reg = &apiRegistrar{
		httpServers: []*httpServer{
			{
				gRPCEndpoint: "tcp://127.0.0.1:1234",
				endpoint:     "",
				mux:          &runtime.ServeMux{},
				serve: func(errChan chan error) error {
					return nil
				},
				shutdown: func() {
					return
				},
			},
		},
	}
	a.NoError(reg.RegisterHTTPHandler(func(ctx context.Context, mux *runtime.ServeMux, endpoint string, opts []grpc.DialOption) (err error) {
		a.Equal("127.0.0.1:1234", endpoint)
		return nil
	}))
}
