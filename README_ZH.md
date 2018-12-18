# Gmqtt [![Build Status](https://travis-ci.org/DrmagicE/gmqtt.svg?branch=master)](https://travis-ci.org/DrmagicE/gmqtt) [![codecov](https://codecov.io/gh/DrmagicE/gmqtt/branch/master/graph/badge.svg)](https://codecov.io/gh/DrmagicE/gmqtt) [![Go Report Card](https://goreportcard.com/badge/github.com/DrmagicE/gmqtt)](https://goreportcard.com/report/github.com/DrmagicE/gmqtt)

# 更新日志
## 2018.12.15
* 增加go modules支持
* 调整包结构
## 2018.12.2
* 优化订阅存储结构，大幅提高并发能力和减少响应时间
* 更新优化后的压力测试结果
## 2018.11.25
* 增加压力测试工具
* 优化部分代码结构
* 改变订阅主题的存储方式，优化转发性能
* 修改OnClose钩子方法，增加连接关闭原因
## 2018.11.18
* 暂时删除了session持久化功能，需要重新设计
* 新增运行状态监控/管理功能，在`cmd/broker`中通过restapi呈现
* 新增服务端触发的发布/订阅功能，在`cmd/broker`中通过restapi呈现
* 为session增加了缓存队列
* 重构部分代码，bug修复

# 本库的内容有：
* 基于Go语言实现的V3.1.1版本的MQTT服务器
* 提供MQTT服务器开发库，使用该库可以二次开发出功能更丰富的MQTT服务器应用
* MQTT V3.1.1 版本的协议解析库
* MQTT压力测试工具 [README.md](https://github.com/DrmagicE/gmqtt/blob/master/cmd/benchmark/README_ZH.md)

# 功能特性
* 内置了许多实用的钩子方法，使用者可以方便的定制需要的MQTT服务器（鉴权,ACL等功能）
* 支持tls/ssl以及ws/wss
* 提供服务状态监控/管理api
* 提供发布/订阅/取消订阅api

# 安装
```$ go get -u github.com/DrmagicE/gmqtt```
# 开始

## 使用内置的MQTT服务器
[内置MQTT服务器](https://github.com/DrmagicE/gmqtt/blob/master/cmd/broker/README_ZH.md)

## 使用MQTT服务器开发库
当前内置的MQTT服务器功能比较弱，鉴权，ACL等功能均没有实现，建议采用MQTT服务器库进行二次开发：
```
func main() {

    s := gmqtt.NewServer()
   
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
    //在Run()之前可以设置配置参数，以及注册钩子方法
    s.SetMaxInflightMessages(20)
    s.SetMaxQueueMessages(99999)
    s.RegisterOnSubscribe(func(client *gmqtt.Client, topic packets.Topic) uint8 {
        if topic.Name == "test/nosubscribe" {
            return packets.SUBSCRIBE_FAILURE
        }
        return topic.Qos
    })
    s.Run()
    fmt.Println("started...")
    signalCh := make(chan os.Signal, 1)
    signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
    <-signalCh
    s.Stop(context.Background())
    fmt.Println("stopped")
}

```
更多的使用例子可以参考`\examples`，里面介绍了全部钩子的使用方法。


# 文档说明
[godoc](https://www.godoc.org/github.com/DrmagicE/gmqtt)
## 钩子方法
Gmqtt实现了下列钩子方法
* OnAccept  (仅支持在tcp/ssl下,websocket不支持)
* OnConnect 
* OnSubscribe
* OnPublish
* OnClose
* OnStop

在 `/examples/hook` 中有钩子的使用方法介绍。

### OnAccept
当使用tcp或者ssl方式连接的时候，该钩子方法会在`net.Listener.Accept`之后调用，
如果返回false，则会直接关闭tcp连接。
```
type OnAccept func(conn net.Conn) bool
```
该钩子方法可以拒绝一些非法链接，可以用做自定义黑名单，连接速率限制等功能。

### OnConnect()
接收到登录报文之后会调用该方法。
该方法返回CONNACK报文当中的code值。
```
//return the code of connack packet
type OnConnect func(client *Client) (code uint8)
```
该方法可以用作鉴权实现，比如：
```
...
server.RegisterOnConnect(func(client *server.Client) (code uint8) {
  username := client.ClientOptions().Username
  password := client.ClientOptions().Password
  if validateUser(username, password) { //鉴权信息可能保存在数据库，文件，内存等地方
    return packets.CodeAccepted
  } else {
    return packets.CodeBadUsernameorPsw
  }
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
type OnSubscribe func(client *Client, topic packets.Topic) uint8
```
该方法可以用作实现ACL访问控制，比如：
```
...
server.RegisterOnSubscribe(func(client *server.Client, topic packets.Topic) uint8 {
  if client.ClientOptions().Username == "root" { //root用户想订阅什么就订阅什么
    return topic.Qos
  } else {
    if topic.Qos <= packets.QOS_1 {
      return topic.Qos
    }
    return packets.QOS_1   //对于其他用户，最多只能订阅到QoS1等级
  }
})
```

### OnPublish()
接收到PUBLISH报文之后调用。
```
//返回该报文是否会被继续分发下去
type OnPublish func(client *Client, publish *packets.Publish) bool
```
比如：
```
...
server.RegisterOnPublish(func(client *server.Client, publish *packets.Publish)  bool {
  if client.ClientOptions().Username == "subscribeonly" {
    client.Close()  //2.close the Network Connection
    return false
  }
  //Only qos1 & qos0 are acceptable(will be delivered)
	if publish.Qos == packets.QOS_2 {
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
type OnClose func(client *Client, err error)
```

### OnStop()
当mqtt服务停止的时候调用
```
type OnStop func()
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

# TODO 
* 完善文档
* 性能对比[EMQ/Mosquito]
