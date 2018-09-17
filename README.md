[English](https://github.com/DrmagicE/gmqtt/blob/master/README.EN.md).
# Gmqtt
本库的内容有：
* 基于Go语言实现的V3.1.1版本的MQTT服务器
* MQTT V3.1.1 版本的协议解析库

# 功能特性
* 内置了一些钩子方法，让使用者可以方便的定制需要的MQTT服务器（鉴权,ACL等功能）
* 支持tls/ssl以及ws/wss

# 安装
```$ go get github.com/DrmagicE/gmqtt```
# 开始
下列命令将监听1883端口，并开启一个MQTT服务器
```
$ cd cmd
$ go run main.go
```
# 例子
在`\examples`文件夹中有许多例子，里面介绍了一些本库的基本使用方法。 

# 文档说明

## 钩子
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
//If returns is `false`, it will close the `net.Conn` directly
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
server.OnConnect = func(client *server.Client) (code uint8) {
  username := client.ClientOption().Username
  password := client.ClientOption().Password
  if validateUser(username, password) { //鉴权信息可以保存在数据库，文件，内存等地方
    return packets.CODE_ACCEPTED
  } else {
    return packets.CODE_BAD_USERNAME_OR_PSW
  }
}

```
### OnSubscribe()
接收到SUBSCRIBE报文之后调用。
该方法返回允许当前订阅主题的最大QoS等级。
It returns the maximum QoS level that was granted to the subscription that was requested by the SUBSCRIBE packet.
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
server.OnSubscribe = func(client *server.Client, topic packets.Topic) uint8 {
  if client.ClientOption().Username == "root" { //root用户想订阅什么就订阅什么
    return topic.Qos
  } else {
    if topic.Qos <= packets.QOS_1 {
      return topic.Qos
    }
    return packets.QOS_1   //对于其他用户，最多只能订阅到QoS1等级
  }
  
}
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
server.OnPublish = func(client *server.Client, publish *packets.Publish)  bool {
  if client.ClientOption().Username == "subscribeonly" {
    client.Close()  //2.close the Network Connection
    return false
  }
  //Only qos1 & qos0 are acceptable(will be delivered)
	if publish.Qos == packets.QOS_2 {
    return false  //1.make a positive acknowledgement but not going to distribute the packet
  }
  return true
}
```
>If a Server implementation does not authorize a PUBLISH to be performed by a Client; it has no way of informing that Client. It MUST either 1.make a positive acknowledgement, according to the normal QoS rules, or 2.close the Network Connection [MQTT-3.3.5-2].

### OnClose()
当网络连接关闭之后调用
```
//This is called after Network Connection close
type OnClose func(client *Client)
```

### OnStop()
但mqtt服务停止的时候调用
```
type OnStop func()
```

## 服务停止流程
调用 `server.Stop()` 将服务优雅关闭:
1. 关闭所有的`net.Listener`
2. 关闭所有的client，一直等待，直到所有的client的`OnClose`方法调用完毕
3. 退出

# 测试
## 单元测试
```
$ cd server
$ go test 
```
```
$ cd pkg/packets
$ go test
```
## 集成测试
通过了 [paho.mqtt.testing](https://github.com/eclipse/paho.mqtt.testing).

# TODO
* 增加配置项
* 更多的测试（单元测试/集成测试）
* 性能测试
* 消息持久化
* 网页监控
* 控制台MQTT客户端
