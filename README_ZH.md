# Gmqtt [![Mentioned in Awesome Go](https://awesome.re/mentioned-badge.svg)](https://github.com/avelino/awesome-go) [![Build Status](https://travis-ci.org/DrmagicE/Gmqtt.svg?branch=master)](https://travis-ci.org/DrmagicE/Gmqtt) [![codecov](https://codecov.io/gh/DrmagicE/Gmqtt/branch/master/graph/badge.svg)](https://codecov.io/gh/DrmagicE/Gmqtt) [![Go Report Card](https://goreportcard.com/badge/github.com/DrmagicE/Gmqtt)](https://goreportcard.com/report/github.com/DrmagicE/Gmqtt)

News: 现已支持V5版本，但由于V5的功能特性，Gmqtt做了很多不兼容的改动，对此有疑问欢迎提issue交流，或者依然使用最新的[V3版本](https://github.com/DrmagicE/gmqtt/tree/v0.1.4).

Gmqtt是用Go语言实现的一个具备灵活灵活扩展能力，高性能的MQTT broker，其完整实现了MQTT V3.1.1和V5协议。

# 安装
```$ go get -u github.com/DrmagicE/gmqtt/```

# 功能特性
* 内置了许多实用的钩子方法，使用者可以方便的定制需要的MQTT服务器（鉴权,ACL等功能）
* 支持tls/ssl以及ws/wss
* 提供扩展编程接口，可以通过函数调用直接往broker发消息，添加删除订阅等。详见`server.go`的`Server`接口定义，以及 [admin](https://github.com/DrmagicE/Gmqtt/blob/master/plugin/admin/READEME.md)插件。
* 丰富的钩子方法和扩展编程接口赋予了Gmqtt强大的插件定制化能力。详见`server/plugin.go` 和 `/plugin`。
* 提供监控指标，支持prometheus。 (plugin: [prometheus](https://github.com/DrmagicE/Gmqtt/blob/master/plugin/prometheus/READEME.md))
* GRPC和REST API 支持. (plugin:[admin](https://github.com/DrmagicE/Gmqtt/blob/master/plugin/admin/READEME.md))
* 支持session持久化，broker重启消息不丢失，目前支持redis持久化。

# 缺陷
* 不支持集群。

# 开始

下列命令会使用默认配置启动Gmqtt服务，该服务使用1883端口[tcp]和8883端口[websocket]端口提供MQTT broker服务，并加载admin和prometheus插件。
```bash
$ cd cmd/gmqttd
$ go run . start -c default_config.yml
```

## 配置
Gmqtt通过`-c`来指定配置文件路径，如果没有指定，Gmqtt默认读取`$HOME/gmqtt.yml`为配置文件，如果文件不存在，Gmqtt将以[默认配置](https://github.com/DrmagicE/Gmqtt/blob/master/cmd/Gmqttd/default_config.yml)。

## 使用持久化存储
Gmqtt默认使用内存存储，这也是Gmqtt推荐的存储方式，内存存储具备绝佳的性能优势，但缺点是session信息会在broker重启后丢失。
如果你希望重启后session不丢失，可以配置redis持久化存储：
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

# 文档说明
[godoc](https://www.godoc.org/github.com/DrmagicE/gmqtt)
## 钩子方法
Gmqtt实现了下列钩子方法。

| hook | 说明 | 用途示例 |
|------|------------|------------|
| OnAccept  | TCP连接建立时调用|  TCP连接限速，黑白名单等.      |
| OnStop  | Broker退出时调用 |    |
| OnSubscribe  | 收到订阅请求时调用| 校验订阅是否合法    |
| OnSubscribed  | 订阅成功后调用   |   统计订阅报文数量   |
| OnUnsubscribe  | 取消订阅时调用       | 校验是否允许取消订阅       |
| OnUnsubscribed  | 取消订阅成功后调用   |   统计订阅报文数     |
| OnMsgArrived  | 收到消息发布报文时调用       |  校验发布权限，改写发布消息       |
| OnBasicAuth  | 收到连接请求报文时调用       | 客户端连接鉴权       |
| OnEnhancedAuth  | 收到带有AuthMetho的连接请求报文时调用（V5特性）| 客户端连接鉴权      |
| OnReAuth  | 收到Auth报文时调用（V5特性）        | 客户端连接鉴权      |
| OnConnected  | 客户端连接成功后调用|    统计在线客户端数量    | 
| OnSessionCreated  | 客户端创建新session后调用       |  统计session数量       |
| OnSessionResumed  | 客户端从旧session恢复后调用       | 统计session数量       |
| OnSessionTerminated  | session删除后调用       | 统计session数量       |
| OnDelivered  | 消息从broker投递到客户端后调用       |        |
| OnClosed  | 客户端断开连接后调用       |   统计在线客户端数量      |
| OnMsgDropped  | 消息被丢弃时调用 |        |

在 `/examples/hook` 中有常用钩子的使用方法介绍。

## 怎么写插件
[How to write plugins](https://github.com/DrmagicE/gmqtt/blob/master/plugin/README.md)

# 测试
## 单元测试
```
$ go test -race ./...  
```

## 集成测试
[paho.mqtt.testing](https://github.com/eclipse/paho.mqtt.testing).


# TODO
* 桥接模式，集群模式

*暂时不保证向后兼容，在添加上述新功能时可能会有breaking changes。*
