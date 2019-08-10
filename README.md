[中文文档](https://github.com/DrmagicE/gmqtt/blob/master/README_ZH.md)
# Gmqtt [![Mentioned in Awesome Go](https://awesome.re/mentioned-badge.svg)](https://github.com/avelino/awesome-go) [![Build Status](https://travis-ci.org/DrmagicE/gmqtt.svg?branch=master)](https://travis-ci.org/DrmagicE/gmqtt) [![codecov](https://codecov.io/gh/DrmagicE/gmqtt/branch/master/graph/badge.svg)](https://codecov.io/gh/DrmagicE/gmqtt) [![Go Report Card](https://goreportcard.com/badge/github.com/DrmagicE/gmqtt)](https://goreportcard.com/report/github.com/DrmagicE/gmqtt)
Inspired by [EMQ](https://github.com/emqx/emqx)

Gmqtt provides:
*  MQTT broker that fully implements the MQTT protocol V3.1.1.
*  Golang MQTT broker package for secondary development.
*  MQTT protocol pack/unpack package for implementing MQTT clients or testing.
*  MQTT broker benchmark tool.

# Installation
```$ go get -u github.com/DrmagicE/gmqtt```

# Features
* Built-in hook methods so you can customized the behaviours of your project(Authentication, ACL, etc..)
* Support tls/ssl and websocket
* Provide monitor/management API
## New Features
* More Hooks function ara available. See `hooks.go` for more details
* Enable user to develop plugins . See `plugin.go` and `/plugin/management` for more details
* Use tire tree to store subscriptions

# Limitation
* The retained messages are not persisted when the server exit.
* Cluster is not supported.


# Get Started
## Build-in MQTT broker
[Build-in MQTT broker](https://github.com/DrmagicE/gmqtt/blob/master/cmd/broker/README.md)

## Using `gmqtt` Library for Secondary Development
The features of build-in MQTT broker are not rich enough.It is not implementing some features such as Authentication, ACL etc..
So It is recommend to use `gmqtt` package to customized your broker: 

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
* OnUnsubscribed
* OnMsgArrived
* OnAcked
* OnMsgDropped
* OnDeliver
* OnClose
* OnStop

See /examples/hook for more detail.

### OnConnect()
```
// OnConnect will be called when a valid connect packet is received.
// It returns the code of the connack packet.
type OnConnect func(cs ChainStore, client Client) (code uint8)
```
This hook may be used to implement  Authentication process.For example:
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
type OnSubscribe func(cs ChainStore, client Client, topic packets.Topic) (qos uint8)
```
This hook may be used to implement  ACL(Access Control List) process.For example:
```
...
s.RegisterOnSubscribe(func(cs gmqtt.ChainStore, client gmqtt.Client, topic packets.Topic) (qos uint8) {
    if client.OptionsReader().Username() == "root" {
        return topic.Qos
    }
    if client.OptionsReader().Username() == "qos0" {
        if topic.Qos <= packets.QOS_0 {
            return topic.Qos
        }
        return packets.QOS_0
    }
    if client.OptionsReader().Username() == "qos1" {
        if topic.Qos <= packets.QOS_1 {
            return topic.Qos
        }
        return packets.QOS_1
    }
    if client.OptionsReader().Username() == "publishonly" {
        return packets.SUBSCRIBE_FAILURE
    }
    return topic.Qos
})
```
### OnUnsubscribed()
```
// OnUnsubscribed will be called after the topic has been unsubscribed
type OnUnsubscribed func(cs ChainStore, client Client, topicName string)
```


### OnMsgArrived()
This method will be called after receiving a MQTT PUBLISH packet.
```
// OnMsgArrived returns whether the publish packet will be delivered or not.
// If returns false, the packet will not be delivered to any clients.
type OnMsgArrived func(cs ChainStore, client Client, msg Message) (valid bool)
```
For example:
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
```
// OnClose will be called after the tcp connection of the client has been closed
type OnClose func(cs ChainStore, client Client, err error)
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





