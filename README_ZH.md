# Gmqtt [![Mentioned in Awesome Go](https://awesome.re/mentioned-badge.svg)](https://github.com/avelino/awesome-go) [![Build Status](https://travis-ci.org/DrmagicE/gmqtt.svg?branch=master)](https://travis-ci.org/DrmagicE/gmqtt) [![codecov](https://codecov.io/gh/DrmagicE/gmqtt/branch/master/graph/badge.svg)](https://codecov.io/gh/DrmagicE/gmqtt) [![Go Report Card](https://goreportcard.com/badge/github.com/DrmagicE/gmqtt)](https://goreportcard.com/report/github.com/DrmagicE/gmqtt)

# 本库的内容有：
* 基于Go语言实现的V3.1.1版本的MQTT服务器
* 提供MQTT服务器开发库，使用该库可以二次开发出功能更丰富的MQTT服务器应用
* MQTT V3.1.1 版本的协议解析库

# 安装
```$ go get -u github.com/DrmagicE/gmqtt```

# 功能特性
* 内置了许多实用的钩子方法，使用者可以方便的定制需要的MQTT服务器（鉴权,ACL等功能）
* 支持tls/ssl以及ws/wss
* 定制化插件能力。具体内容可参考`plugin.go` 和 `/plugin/`
* 暴露服务接口，向外部提供与server交互的能力，详见`server.go`的`Server`接口定义和`example_test.go`。
* 提供监控指标，目前支持prometheus。 (plugin: [prometheus](https://github.com/DrmagicE/gmqtt/blob/master/plugin/prometheus/READEME.md))
* restful API支持. (plugin:[management](https://github.com/DrmagicE/gmqtt/blob/master/plugin/management/READEME.md))


# 缺陷
* 保留消息还未实现持久化存储。
* 不支持集群部署。


# 开始

## 使用内置的MQTT服务器
下列命令将启动一个监听`1883`端口[tcp]和`8080`端口[websocket]的MQTT服务。
该broker加载了如下插件:
 * [management](https://github.com/DrmagicE/gmqtt/blob/master/plugin/management/README.md): 监听`8081`端口, 提供restful api服务
 * [prometheus](https://github.com/DrmagicE/gmqtt/blob/master/plugin/prometheus/README.md): 监听`8082`端口, 作为prometheus exporter供prometheus server采集，接口地址为: `/metrics`

```
$ cd cmd/broker
$ go run main.go 
```
## Docker
```
$ docker build -t gmqtt .
$ docker run -p 1883:1883 -p  8081:8081 -p 8082:8082 gmqtt
```
## 从外部代码引入
当前内置的MQTT服务器功能比较弱，鉴权，ACL等功能均没有实现。
但可以通过自定义插件来实现相关功能：
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
更多的使用例子可以参考`\examples`，里面介绍了常用钩子函数的使用方法。


# 文档说明
[godoc](https://www.godoc.org/github.com/DrmagicE/gmqtt)
## 钩子方法
Gmqtt实现了下列钩子方法
* OnAccept  (仅支持在tcp/ssl下,websocket不支持)
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

在 `/examples/hook` 中有钩子的使用方法介绍。

## 停止server
调用 `server.Stop()` 将服务优雅关闭:
1. 关闭所有的在监听的listener和websocket server
2. 关闭所有的client连接
3. 等待所有的client关闭完成
4. 触发OnStop()

# 测试
## 单元测试
```
$ go test -race .
```
```
$ cd pkg/packets
$ go test -race .
```
## 集成测试
通过了 [paho.mqtt.testing](https://github.com/eclipse/paho.mqtt.testing).



# TODO
* 支持MQTT V3和V5
* 桥接模式，集群模式（看情况）

*暂时不保证向后兼容，在添加上述新功能时可能会有breaking changes。*
