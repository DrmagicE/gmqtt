[中文文档](https://github.com/DrmagicE/gmqtt/blob/master/README_ZH.md)
# Gmqtt [![Mentioned in Awesome Go](https://awesome.re/mentioned-badge.svg)](https://github.com/avelino/awesome-go) [![Build Status](https://travis-ci.org/DrmagicE/gmqtt.svg?branch=master)](https://travis-ci.org/DrmagicE/gmqtt) [![codecov](https://codecov.io/gh/DrmagicE/gmqtt/branch/master/graph/badge.svg)](https://codecov.io/gh/DrmagicE/gmqtt) [![Go Report Card](https://goreportcard.com/badge/github.com/DrmagicE/gmqtt)](https://goreportcard.com/report/github.com/DrmagicE/gmqtt)

News: MQTT V5 is now supported. But due to those new features in v5, there area lots of breaking changes. 
If you have any migration problems, feel free to raise an issue.
Or you can use the latest v3 [broker](https://github.com/DrmagicE/gmqtt/tree/v0.1.4).

# Installation
```$ go get -u github.com/DrmagicE/gmqtt```

# Features
* Provide hook method to customized the broker behaviours(Authentication, ACL, etc..). See `server/hooks.go` for details
* Support tls/ssl and websocket
* Provide flexible plugable mechanism. See `server/plugin.go` and `/plugin` for details.
* Provide Go interface for extensions to interact with the server. For examples, the extensions or plugins can publish message or add/remove subscription through function call.
See `Server` interface in `server/server.go` and [admin](https://github.com/DrmagicE/Gmqtt/blob/master/plugin/admin/READEME.md) for details.
* Provide metrics (by using Prometheus). (plugin: [prometheus](https://github.com/DrmagicE/gmqtt/blob/master/plugin/prometheus/README.md))
* Provide GRPC and REST APIs to interact with server. (plugin:[admin](https://github.com/DrmagicE/gmqtt/blob/master/plugin/admin/README.md))
* Provide session persistence which means the broker can retrieve the session data after restart. 
Currently, only redis backend is supported.



# Limitations
* Cluster is not supported.


# Get Started

The following command will start gmqtt broker with default configuration.
The broker listens on 1883 for tcp server and 8883 for websocket server with `admin` and `prometheus` plugin loaded.

```bash
$ cd cmd/gmqttd
$ go run . start -c default_config.yml
```

## configuration
Gmqtt use `-c` flag to define configuration path. If not set, gmqtt reads `$HOME/gmqtt.yml` as default. If default path not exist, 
Gmqtt will start with [default configuration](https://github.com/DrmagicE/gmqtt/blob/master/cmd/gmqttd/default_config.yml).

## session persistence
Gmqtt uses memory to store session data by default and it is the recommended way because of the good performance.
But the session data will be lose after the broker restart. You can use redis as backend storage to prevent data 
loss from restart: 
```yaml
persistence:
  type: redis  
  redis:
    # redis server address
    addr: "127.0.0.1:6379"
    # the maximum number of idle connections in the redis connection pool
    max_idle: 1000
    # the maximum number of connections allocated by the redis connection pool at a given time.
    # If zero, there is no limit on the number of connections in the pool.
    max_active: 0
    # the connection idle timeout, connection will be closed after remaining idle for this duration. If the value is zero, then idle connections are not closed
    idle_timeout: 240s
    password: ""
    # the number of the redis database
    database: 0
```

## Docker
```
$ docker build -t gmqtt .
$ docker run -p 1883:1883 -p 8883:8883 -p 8082:8082 -p 8083:8083  -p 8084:8084  gmqtt
```

# Documentation
[godoc](https://www.godoc.org/github.com/DrmagicE/gmqtt)
## Hooks
Gmqtt implements the following hooks: 

| Name | hooking point | possible usages  |
|------|------------|------------|
| OnAccept  | When accepts a TCP connection.(Not supported in websocket)| Connection rate limit, IP allow/block list. |
| OnStop  | When the broker exists |    |
| OnSubscribe  | When received a subscribe packet | Subscribe access control, modifies subscriptions. |
| OnSubscribed  | When subscribe succeed   |     |
| OnUnsubscribe  |  When received a unsubscribe packet | Unsubscribe access controls, modifies the topics that is going to unsubscribe.|
| OnUnsubscribed  | When unsubscribe succeed     |        |
| OnMsgArrived  | When received a publish packet  |  Publish access control, modifies message before delivery.|
| OnBasicAuth  | When received a connect packet without AuthMethod property | Authentication      |
| OnEnhancedAuth  | When received a connect packet with AuthMethod property (Only for v5 clients) | Authentication      |
| OnReAuth  | When received a auth packet (Only for v5 clients)        | Authentication      |
| OnConnected  | When the client connected succeed|      | 
| OnSessionCreated  | When creates a new session       |         |
| OnSessionResumed  | When resumes from old session    |        |
| OnSessionTerminated  | When session terminated       |        |
| OnDelivered  | When a message is delivered to the client     |        |
| OnClosed  | When the client is closed  |        |
| OnMsgDropped  | When a message is dropped for some reasons|        |
See `/examples/hook` for details.


# Test
## Unit Test
```
$ go test -race ./...
```

## Integration Test
[paho.mqtt.testing](https://github.com/eclipse/paho.mqtt.testing).


# TODO
* Support bridge mode and cluster.

*Breaking changes may occur when adding this new features.*
