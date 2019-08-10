# Gmqtt [![Mentioned in Awesome Go](https://awesome.re/mentioned-badge.svg)](https://github.com/avelino/awesome-go) [![Build Status](https://travis-ci.org/DrmagicE/gmqtt.svg?branch=master)](https://travis-ci.org/DrmagicE/gmqtt) [![codecov](https://codecov.io/gh/DrmagicE/gmqtt/branch/master/graph/badge.svg)](https://codecov.io/gh/DrmagicE/gmqtt) [![Go Report Card](https://goreportcard.com/badge/github.com/DrmagicE/gmqtt)](https://goreportcard.com/report/github.com/DrmagicE/gmqtt)
本项目受[EMQ](https://github.com/emqx/emqx)启发，参考了其中的许多设计。
# 本库的内容有：
* 基于Go语言实现的V3.1.1版本的MQTT服务器
* 提供MQTT服务器开发库，使用该库可以二次开发出功能更丰富的MQTT服务器应用
* MQTT V3.1.1 版本的协议解析库
* MQTT压力测试工具 [README.md](https://github.com/DrmagicE/gmqtt/blob/master/cmd/benchmark/README_ZH.md)

# 安装
```$ go get -u github.com/DrmagicE/gmqtt```

# 功能特性
* 内置了许多实用的钩子方法，使用者可以方便的定制需要的MQTT服务器（鉴权,ACL等功能）
* 支持tls/ssl以及ws/wss
* 提供服务状态监控/管理api

## 新特性
* 增加了许多钩子方法，可参考`hooks.go`。
* 增加插件开发的能力。具体内容可参考`plugin.go` 和 `/plugin/management`
* 使用字典树来存储订阅消息

# 缺陷
* 保留消息还未实现持久化存储。
* 不支持集群部署。



# 开始

## 使用内置的MQTT服务器
[内置MQTT服务器](https://github.com/DrmagicE/gmqtt/blob/master/cmd/broker/README_ZH.md)

## 使用MQTT服务器开发库
当前内置的MQTT服务器功能比较弱，鉴权，ACL等功能均没有实现，建议采用MQTT服务器库进行二次开发：
```
func main() {
	s := gmqtt.DefaultServer()
	ln, err := net.Listen("tcp", ":1883")
	if err != nil {
		log.Fatalln(err.Error())
		return
	}
	crt, err := tls.LoadX509KeyPair("../testcerts/server.crt", "../testcerts/server.key")
	if err != nil {
		log.Fatalln(err.Error())
		return
	}
	tlsConfig := &tls.Config{}
	tlsConfig.Certificates = []tls.Certificate{crt}
	tlsln, err := tls.Listen("tcp", ":8883", tlsConfig)

	if err != nil {
		log.Fatalln(err.Error())
		return
	}
	s.AddTCPListenner(ln)
	s.AddTCPListenner(tlsln)

	s.RegisterOnSubscribe(func(cs gmqtt.ChainStore, client gmqtt.Client, topic packets.Topic) (qos uint8) {
		if topic.Name == "test/nosubscribe" {
			return packets.SUBSCRIBE_FAILURE
		}
		return topic.Qos
	})

	//server.SetLogger(logger.NewLogger(os.Stderr, "", log.LstdFlags))
	s.Run()
	fmt.Println("started...")
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	<-signalCh
	s.Stop(context.Background())
	fmt.Println("stopped")
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
* OnUnsubscribed
* OnMsgArrived
* OnAcked
* OnMsgDropped
* OnDeliver
* OnClose
* OnStop

在 `/examples/hook` 中有钩子的使用方法介绍。

### OnConnect()
接收到登录报文之后会调用该方法。
该方法返回CONNACK报文当中的code值。
```
//return the code of connack packet
type OnConnect func(cs ChainStore, client Client) (code uint8)
```
该方法可以用作鉴权实现，比如：
```
...
s.RegisterOnConnect(func(cs gmqtt.ChainStore, client gmqtt.Client) (code uint8) {
    username := client.OptionsReader().Username()
    password := client.OptionsReader().Password()
    if validateUser(username, password) {
        return packets.CodeAccepted
    }
    return packets.CodeBadUsernameorPsw
})
```
### OnSubscribe()
接收到SUBSCRIBE报文之后调用。
该方法返回允许当前订阅主题的最大QoS等级。
```
//允许的一些返回值:
//0x00 - 成功 - 最大 QoS 0
//0x01 - 成功 - 最大 QoS 1
//0x02 - 成功 - 最大 QoS 2
//0x80 - 订阅失败
type OnSubscribe func(cs ChainStore, client Client, topic packets.Topic) (qos uint8)
```
该方法可以用作实现ACL访问控制，比如：
```
...
s.RegisterOnSubscribe(func(cs gmqtt.ChainStore, client gmqtt.Client, topic packets.Topic) (qos uint8) {
    if client.OptionsReader().Username() == "root" { // root用户想订阅什么就订阅什么
        return topic.Qos
    }
    if client.OptionsReader().Username() == "qos0" { //最大只能订阅qos0的用户
        if topic.Qos <= packets.QOS_0 {
            return topic.Qos
        }
        return packets.QOS_0
    }
    if client.OptionsReader().Username() == "qos1" { //最大只能订阅qos1的用户
        if topic.Qos <= packets.QOS_1 {
            return topic.Qos
        }
        return packets.QOS_1
    }
    if client.OptionsReader().Username() == "publishonly" { // 不允许订阅的用户
        return packets.SUBSCRIBE_FAILURE
    }
    return topic.Qos
})
```
### OnUnsubscribed()
取消订阅之后调用。
```
// OnUnsubscribed will be called after the topic has been unsubscribed
type OnUnsubscribed func(cs ChainStore, client Client, topicName string)
```

### OnMsgArrived()
接收到PUBLISH报文之后调用。
```
//返回该报文是否会被继续分发下去
type OnMsgArrived func(cs ChainStore, client Client, msg Message) (valid bool)
```
比如：
```
...
s.RegisterOnMsgArrived(func(cs gmqtt.ChainStore, client gmqtt.Client, msg gmqtt.Message) (valid bool) {
    if client.OptionsReader().Username() == "subscribeonly" {
        client.Close()  //2.close the Network Connection
        return false
    }
    //Only qos1 & qos0 are acceptable(will be delivered)
    if msg.Qos() == packets.QOS_2 {
        return false  //1.make a positive acknowledgement but not going to distribute the packet
    }
    return true
})
```
>If a Server implementation does not authorize a PUBLISH to be performed by a Client; it has no way of informing that Client. It MUST either 1.make a positive acknowledgement, according to the normal QoS rules, or 2.close the Network Connection [MQTT-3.3.5-2].

### OnClose()
当网络连接关闭之后调用
```
//This is called after Network Connection close
type OnClose func(cs ChainStore, client Client, err error)
```


## 服务停止流程
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

## 压力测试
[文档与测试结果](https://github.com/DrmagicE/gmqtt/blob/master/cmd/benchmark/README_ZH.md)
