[中文文档](https://github.com/DrmagicE/gmqtt/blob/master/README_ZH.md)
# Gmqtt [![Mentioned in Awesome Go](https://awesome.re/mentioned-badge.svg)](https://github.com/avelino/awesome-go) [![Build Status](https://travis-ci.org/DrmagicE/gmqtt.svg?branch=master)](https://travis-ci.org/DrmagicE/gmqtt) [![codecov](https://codecov.io/gh/DrmagicE/gmqtt/branch/master/graph/badge.svg)](https://codecov.io/gh/DrmagicE/gmqtt) [![Go Report Card](https://goreportcard.com/badge/github.com/DrmagicE/gmqtt)](https://goreportcard.com/report/github.com/DrmagicE/gmqtt)

MQTT V5 is under development.([branch v5](https://github.com/DrmagicE/gmqtt/tree/v5))

Gmqtt provides:
*  MQTT broker that fully implements the MQTT protocol V3.1.1.
*  Golang MQTT broker package for secondary development.
*  MQTT protocol pack/unpack package for implementing MQTT clients or testing.

# Installation
```$ go get -u github.com/DrmagicE/gmqtt```

# Features
* Provide hook method to customized the broker behaviours(Authentication, ACL, etc..). See `hooks.go` for more details
* Support tls/ssl and websocket
* Enable user to write plugins. See `plugin.go` and `/plugin` for more details.
* Provide abilities for extensions to interact with the server. See `Server` interface in `server.go`  and `example_test.go` for more details.
* Provide metrics (by using Prometheus). (plugin: [prometheus](https://github.com/DrmagicE/gmqtt/blob/master/plugin/prometheus/README.md))
* Provide restful API to interact with server. (plugin:[management](https://github.com/DrmagicE/gmqtt/blob/master/plugin/management/README.md))

# Limitations
* The retained messages are not persisted when the server exit.
* Cluster is not supported.


# Get Started
## Build-in MQTT broker
```
$ cd cmd/broker
$ go run main.go
```
The broker will listen on port 1883 for TCP and 8080 for websocket.
The broker loads the following plugins:
 * [management](https://github.com/DrmagicE/gmqtt/blob/master/plugin/management/README.md): Listens on port `8081`, provides restful api service
 * [prometheus](https://github.com/DrmagicE/gmqtt/blob/master/plugin/prometheus/README.md): Listens on port `8082`, serve as a prometheus exporter with `/metrics` path.


## Docker
```
$ docker build -t gmqtt .
$ docker run -p 1883:1883 -p  8081:8081 -p 8082:8082 gmqtt
```
## Build with external source code
The features of build-in MQTT broker are not rich enough.It is not implementing some features such as Authentication, ACL etc..
But It is easy to write your own plugins to extend the broker.
```
func main() {
	// listener
	ln, err := net.Listen("tcp", ":1883")
	if err != nil {
		log.Fatalln(err.Error())
		return
	}
	// websocket server
	ws := &gmqtt.WsServer{
		Server: &http.Server{Addr: ":8080"},
		Path:   "/ws",
	}
	if err != nil {
		panic(err)
	}

	l, _ := zap.NewProduction()
	// l, _ := zap.NewDevelopment()
	s := gmqtt.NewServer(
		gmqtt.WithTCPListener(ln),
		gmqtt.WithWebsocketServer(ws),
		// Add your plugins
		gmqtt.WithPlugin(management.New(":8081", nil)),
		gmqtt.WithPlugin(prometheus.New(&http.Server{
			Addr: ":8082",
		}, "/metrics")),
		gmqtt.WithLogger(l),
	)

	s.Run()
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	<-signalCh
	s.Stop(context.Background())
}
```
See `/examples` for more details.

# Documentation
[godoc](https://www.godoc.org/github.com/DrmagicE/gmqtt)
## Hooks
Gmqtt implements the following hooks:
* OnAccept  (Only for tcp/ssl, not for ws/wss)
* OnConnect 
* OnConnected
* OnSessionCreated
* OnSessionResumed
* OnSessionTerminated
* OnSubscribe
* OnSubscribed
* OnUnsubscribe
* OnUnsubscribed
* OnMsgArrived
* OnAcked
* OnMsgDropped
* OnDeliver
* OnClose
* OnStop

See `/examples/hook` for more detail.



## Stop the Server
Call `server.Stop()` to stop the broker gracefully:
1. Close all open TCP listeners and shutting down all open websocket servers
2. Close all idle connections
3. Wait for all connections have been closed
4. Trigger OnStop().

# Test
## Unit Test
```
$ go test -race . && go test -race pkg/packets
```
```
$ cd pkg/packets
$ go test -race .
```
## Integration Test
Pass [paho.mqtt.testing](https://github.com/eclipse/paho.mqtt.testing).


# TODO
* Support MQTT V3 and V5.
* Support bridge mode and maybe cluster.

*Breaking changes may occur when adding this new features.*
