# Gmqtt
Gmqtt is a Golang MQTT broker that fully implements the MQTT protocol V3.1.1.
This repository also provides a MQTT protocol pack/unpack packet for implementing MQTT clients or testing

# Features
* Built-in hook methods so you can customized the behaviours of your project(Authentication, ACL, etc..)
* Support tls/ssl and websocket


# Installation
```go get github.com/DrmagicE/gmqtt```

# Get Started
Use the following command to start a simple broker that listens on port 1883
```
$ cd cmd
$ go run main.go
```
# Examples
There are some examples in `/examples`. 

# Documentation

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
This method is called after  `net.Listener.Accept` when using tcp or ssl.

```
//If returns is `false`, it will close the `net.Conn` directly
type OnAccept func(conn net.Conn) bool
```
This hook may be used to block some invalid connections.(blacklist, rate-limiting, etc..) 

### OnConnect()
This method is called after receiving MQTT CONNECT packet.
It returns the code of CONNACK packet.
```
//return the code of connack packet
type OnConnect func(client *Client) (code uint8)
```
This hook may be used to implement  Authentication process.For example:
```
...
server.OnConnect = func(client *server.Client) (code uint8) {
  username := client.ClientOption().Username
  password := client.ClientOption().Password
  if validateUser(username, password) { //Authentication info may save in DB,File System, memory, etc.
    return packets.CODE_ACCEPTED
  } else {
    return packets.CODE_BAD_USERNAME_OR_PSW
  }
}

```
### OnSubscribe()
This method is called after receiving MQTT SUBSCRIBE packet.
It returns the maximum QoS level that was granted to the subscription that was requested by the SUBSCRIBE packet.
```
//Allowed return codes:
//0x00 - Success - Maximum QoS 0
//0x01 - Success - Maximum QoS 1
//0x02 - Success - Maximum QoS 2
//0x80 - Failure
type OnSubscribe func(client *Client, topic packets.Topic) uint8
```
This hook may be used to implement  ACL(Access Control List) process.For example:
```
...
server.OnSubscribe = func(client *server.Client, topic packets.Topic) uint8 {
  if client.ClientOption().Username == "root" { //alow root user to subscribe whatever he wants
    return topic.Qos
  } else {
    if topic.Qos <= packets.QOS_1 {
      return topic.Qos
    }
    return packets.QOS_1   //for other users, the maximum QoS level is QoS1
  }
  
}
```

### OnPublish()
This method is called after receiving MQTT PUBLISH packet.
```
//Whether the publish packet will be delivered or not.
type OnPublish func(client *Client, publish *packets.Publish) bool
```
For example:
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
This method is called after Network Connection close.
```
//This is called after Network Connection close
type OnClose func(client *Client)
```

### OnStop()
This method is called after `server.Stop()`
```
type OnStop func()
```

## Server Stop Process
Call `server.Stop()` to stop the broker gracefully:
1. close all `net.Listener`
2. close all clients and wait until all `OnClose()` call are  complete
3. exit

# Test
## Unit Test
```
$ cd server
$ go test 
```
```
$ cd pkg/packets
$ go test
```
## Integration Test
Pass [paho.mqtt.testing](https://github.com/eclipse/paho.mqtt.testing).

# TODO
* More test(Unit/Integration)
* Benchmark test
* Message persistence
* Website monitor
* Cli mqtt client




