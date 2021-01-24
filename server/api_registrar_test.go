package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io/ioutil"
	"sync"
	"testing"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"

	"github.com/DrmagicE/gmqtt/config"
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

func TestBuildTLSConfig(t *testing.T) {

	t.Run("verify_false", func(t *testing.T) {
		a := assert.New(t)
		cfg := &config.TLSOptions{
			CACert: "",
			Cert:   "./testdata/server-cert.pem",
			Key:    "./testdata/server-key.pem",
			Verify: false,
		}
		tlsCfg, err := buildTLSConfig(cfg)
		a.NoError(err)
		a.EqualValues(0, tlsCfg.ClientAuth)
		a.Len(tlsCfg.Certificates, 1)
	})

	t.Run("verify_true", func(t *testing.T) {
		a := assert.New(t)
		cfg := &config.TLSOptions{
			CACert: "",
			Cert:   "./testdata/server-cert.pem",
			Key:    "./testdata/server-key.pem",
			Verify: true,
		}
		tlsCfg, err := buildTLSConfig(cfg)
		a.NoError(err)
		a.EqualValues(tls.RequireAndVerifyClientCert, tlsCfg.ClientAuth)
		a.Len(tlsCfg.Certificates, 1)
	})

	t.Run("add_cacert", func(t *testing.T) {
		a := assert.New(t)
		cfg := &config.TLSOptions{
			CACert: "./testdata/ca.pem",
			Cert:   "./testdata/server-cert.pem",
			Key:    "./testdata/server-key.pem",
		}
		tlsCfg, err := buildTLSConfig(cfg)
		a.NoError(err)
		a.Len(tlsCfg.Certificates, 1)
		opts := x509.VerifyOptions{
			DNSName: "drmagic.local",
			Roots:   tlsCfg.ClientCAs,
		}
		certPEM, err := ioutil.ReadFile("./testdata/server-cert.pem")
		a.NoError(err)
		block, _ := pem.Decode(certPEM)
		cert, err := x509.ParseCertificate(block.Bytes)
		_, err = cert.Verify(opts)
		a.Nil(err)
	})

}
