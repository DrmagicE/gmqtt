[中文文档](https://github.com/DrmagicE/gmqtt/blob/master/README_ZH.md)
# Gmqtt [![Build Status](https://travis-ci.org/DrmagicE/gmqtt.svg?branch=master)](https://travis-ci.org/DrmagicE/gmqtt) [![codecov](https://codecov.io/gh/DrmagicE/gmqtt/branch/master/graph/badge.svg)](https://codecov.io/gh/DrmagicE/gmqtt) [![Go Report Card](https://goreportcard.com/badge/github.com/DrmagicE/gmqtt)](https://goreportcard.com/report/github.com/DrmagicE/gmqtt)
Gmqtt provides:
*  MQTT broker that fully implements the MQTT protocol V3.1.1.
*  Golang MQTT broker package for secondary development.
*  MQTT protocol pack/unpack package for implementing MQTT clients or testing.
*  MQTT broker benchmark tool.

# Change Log 
## 2018.12.15
* Supported go modules.
* Restructured the package layout.
## 2018.12.2
* Optimized data structure of subscriptions that increased QPS & reduced response times.
* Updated benchmark results of optimization .
## 2018.11.25
* Added benchmark tool.
* Refacotoring & improved performance.
* Performance improvement on message distributing
* Added error information in `OnClose()`
## 2018.11.18
* Removed sessions/messages persistence which need a redesign.
* Added monitor/management API, added `cmd/broker/restapi` as an example.
* Added publish/subscribe/unsubscribe API, added `cmd/broker/restapi` as an example.
* Added session message queue.
* Refactoring & bug fixed.

# Features
* Built-in hook methods so you can customized the behaviours of your project(Authentication, ACL, etc..)
* Support tls/ssl and websocket
* Provide monitor/management API
* Provide publish/subscribe/unsubscribe API





# Installation
```$ go get -u github.com/DrmagicE/gmqtt```

# Get Started
## Build-in MQTT broker
[Build-in MQTT broker](https://github.com/DrmagicE/gmqtt/blob/master/cmd/broker/README.md)

## Using `gmqtt` Library for Secondary Development
The features of build-in MQTT broker are not rich enough.It is not implementing some features such as Authentication, ACL etc..
So It is recommend to use `gmqtt` package to customized your broker: 

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
    //Configures and registers callback before s.Run()
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
See `/examples` for more details.

# Documentation
[godoc](https://www.godoc.org/github.com/DrmagicE/gmqtt)
## Hooks
Gmqtt implements the following hooks:
* OnAccept  (Only for tcp/ssl, not for ws/wss)
* OnConnect 
* OnSubscribe
* OnPublish
* OnClose
* OnStop

See /examples/hook for more detail.

### OnAccept
```
// OnAccept will be called after a new connection established in TCP server. If returns false, the connection will be close directly.
type OnAccept func(conn net.Conn) bool
```
This hook may be used to block some invalid connections.(blacklist, rate-limiting, etc..) 

### OnConnect()
```
// OnConnect will be called when a valid connect packet is received.
// It returns the code of the connack packet.
type OnConnect func(client *Client) (code uint8)
```
This hook may be used to implement  Authentication process.For example:
```
...
server.RegisterOnConnect(func(client *server.Client) (code uint8) {
  username := client.ClientOptions().Username
  password := client.ClientOptions().Password
  if validateUser(username, password) { //Authentication info may save in DB,File System, memory, etc.
    return packets.CodeAccepted
  } else {
    return packets.CodeBadUsernameorPsw
  }
})

```
### OnSubscribe()
This method is called after receiving MQTT SUBSCRIBE packet.
It returns the maximum QoS level that was granted to the subscription that was requested by the SUBSCRIBE packet.
```
/*
OnSubscribe returns the maximum available QoS for the topic:
 0x00 - Success - Maximum QoS 0
 0x01 - Success - Maximum QoS 1
 0x02 - Success - Maximum QoS 2
 0x80 - Failure
*/
type OnSubscribe func(client *Client, topic packets.Topic) uint8
```
This hook may be used to implement  ACL(Access Control List) process.For example:
```
...
server.RegisterOnSubscribe(func(client *server.Client, topic packets.Topic) uint8 {
  if client.ClientOptions().Username == "root" { //alow root user to subscribe whatever he wants
    return topic.Qos
  } else {
    if topic.Qos <= packets.QOS_1 {
      return topic.Qos
    }
    return packets.QOS_1   //for other users, the maximum QoS level is QoS1
  }
})
```

### OnPublish()
This method will be called after receiving a MQTT PUBLISH packet.
```
// OnPublish returns whether the publish packet will be delivered or not.
// If returns false, the packet will not be delivered to any clients.
type OnPublish func(client *Client, publish *packets.Publish) bool
```
For example:
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
```
// OnClose will be called after the tcp connection of the client has been closed
type OnClose func(client *Client, err error)
```

### OnStop()
```
// OnStop will be called on server.Stop()
type OnStop func()
```

## Server Stop Process
Call `server.Stop()` to stop the broker gracefully:
1. Closing all open TCP listeners and shutting down all open websocket servers
2. Closing all idle connections
3. Waiting for all connections have been closed
4. Triggering OnStop()

# Test
## Unit Test
```
$ go test -race .
```
```
$ cd pkg/packets
$ go test -race .
```
## Integration Test
Pass [paho.mqtt.testing](https://github.com/eclipse/paho.mqtt.testing).

## Benchmark Test
[Documentation & Results](https://github.com/DrmagicE/gmqtt/blob/master/cmd/benchmark/README.md)

# TODO
* Improve documentation
* Performance comparison [EMQ/Mosquito]




