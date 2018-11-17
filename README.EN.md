# Gmqtt [![Build Status](https://travis-ci.org/DrmagicE/gmqtt.svg?branch=master)](https://travis-ci.org/DrmagicE/gmqtt)
Gmqtt provides:
*  MQTT broker that fully implements the MQTT protocol V3.1.1.
*  Golang MQTT broker package for secondary development.
*  MQTT protocol pack/unpack package for implementing MQTT clients or testing.

# Change Log 2018.11.18
* Removed sessions/messages persistence which need a redesign
* Added monitor/management API, added `cmd/broker/restapi` as an example
* Added publish/subscribe/unsubscribe API, added `cmd/broker/restapi` as an example
* Added session message queue
* Refactoring & bug fixed

# Features
* Built-in hook methods so you can customized the behaviours of your project(Authentication, ACL, etc..)
* Support tls/ssl and websocket
* ~~Support sessions/messages persistence~~
* Provide monitor/management API
* Provide publish/subscribe/unsubscribe API


# Installation
```go get github.com/DrmagicE/gmqtt/cmd/broker```

# Get Started
## Build-in MQTT broker
Use the following command to start a simple broker that listens on port 1883 for TCP and 8080 for websocket.
```
$ cd cmd
$ go run main.go
```



### Build-in MQTT Broker Configration
See `cmd/broker/config.yaml`:
```
# Delivery retry interval .Defaults to 20 seconds
delivery_retry_interval: 20
# The maximum number of QoS 1 or 2 messages that can be in the process of being transmitted simultaneously. Defaults to 20
max_inflight_messages: 20
# Set to true to queue messages with QoS 0 when a persistent client is disconnected.Defaults to true.
queue_qos0_messages: true
# The maximum number of messages to hold in the queue (per client) above those messages that are currently in flight. 
Defaults to 20. Set to 0 for no maximum (not recommended).
max_msgqueue_messages: 20

# pprof
# pprof.cpu:The file to store CPU profile, if specified.
# pprof.mem:The file to store memory profile, if specified.
profile: {cpu: "cpuprofile", mem: "memprofile"}
# Set to true to enable logging. Defaults to false 
logging: false
# http_server REST http server
# http_server.addr Addr of http server
# http_server.user User info of http basic auth, username => password
http_server: {addr: ":8080",user: { admin: "admin"}}
# listener
# listener.$.protocol:Set the protocol to accept for this listener. Can be mqtt, the default, or websockets.
# listener.$.addr:Bind address, it wil pass to net.Listen(network, address string) address parameter.
# listener.$.certfile:The cert file path,if using tls/ssl.
# listenr.$.keyfile:The key file path,if using tls/ssl.
listener:
- {protocol: mqtt, addr: ':1883', certfile: , keyfile:  }
- {protocol: websocket, addr: ':8080', certfile: ,keyfile: }
```

`cmd/broker/config.yaml` is the default config file.
Use the following command to specify a config file:

`$ go run main.go -config <config-file-path>`

### REST API
Using HTTP Basic Authentication,configure your username and password with configration file 
#### Get All Active Clients
Request:
```
GET /clients?page=xxx&per-page=xxx
page: page, default to 1
per-page:pagesize, default to 20
```
Response:
```
{
    "list": [
        {
            "client_id": "1",
            "username": "publishonly",
            "remote_addr": "127.0.0.1:56359",
            "clean_session": true,
            "keep_alive": 60,
            "connected_at": "2018-11-18T02:10:36.6958382+08:00"
        }
    ],
    "page": 1,
    "page_size": 20,
    "current_count": 1,
    "total_count": 1,
    "total_page": 1
}

```

#### Get Client With Id
Request:
```
GET /client/:id
```
Response:
```
{
    "client_id": "1",
    "username": "publishonly",
    "remote_addr": "127.0.0.1:56359",
    "clean_session": true,
    "keep_alive": 60,
    "connected_at": "2018-11-18T02:10:36.6958382+08:00"
}
```


#### Get All Sessions

Request:
```
GET /sessions?page=xxx&per-page=xxx
page:请求页数，不传默认第一页
per-page:每一页的条数，不传默认20条
```
Response:
```
{
    "list": [
        {
            "client_id": "1",
            "status": "online",
            "remote_addr": "127.0.0.1:56359",
            "clean_session": true,
            "subscriptions": 0,
            "max_inflight": 20,
            "inflight_len": 0,
            "max_msg_queue": 20,
            "msg_queue_len": 0,
            "msg_queue_dropped": 0,
            "connected_at": "2018-11-18T02:10:36.6958382+08:00",
            "offline_at": "0001-01-01T00:00:00Z"
        }
    ],
    "page": 1,
    "page_size": 20,
    "current_count": 1,
    "total_count": 1,
    "total_page": 1
}
```

#### Get Session With Id

Request:
```
GET /session/:id
```
Response:
```
{
    "client_id": "1",
    "status": "online",
    "remote_addr": "127.0.0.1:56359",
    "clean_session": true,
    "subscriptions": 0,
    "max_inflight": 20,
    "inflight_len": 0,
    "max_msg_queue": 20,
    "msg_queue_len": 0,
    "msg_queue_dropped": 0,
    "connected_at": "2018-11-18T02:10:36.6958382+08:00",
    "offline_at": "0001-01-01T00:00:00Z"
}
```

#### Get All Subscriptions

Request:
```
GET /subscriptions
```
Response:

```
{
    "list": [
        {
            "client_id": "1",
            "qos": 0,
            "name": "test8",
            "at": "2018-11-18T02:14:46.4582717+08:00"
        },
        {
            "client_id": "2",
            "qos": 2,
            "name": "123",
            "at": "2018-11-18T02:14:46.4582717+08:00"
        }
    ],
    "page": 1,
    "page_size": 20,
    "current_count": 2,
    "total_count": 2,
    "total_page": 1
}
```

#### Get Subscriptions Of A Client With Id

Request:
```
GET /subscriptions/:id
```
Response:
```
{
    "list": [
        {
            "client_id": "1",
            "qos": 0,
            "name": "test8",
            "at": "2018-11-18T02:14:46.4582717+08:00"
        },
        {
            "client_id": "1",
            "qos": 2,
            "name": "123",
            "at": "2018-11-18T02:14:46.4582717+08:00"
        }
    ],
    "page": 1,
    "page_size": 20,
    "current_count": 2,
    "total_count": 2,
    "total_page": 1
}
```



#### Publish

Request:
```
POST /publish
```
Post Form:
```
qos : qos level
topic : topic name
payload : payload
```

Response:
```
{
    "code": 0,
    "result": []
}
```

#### 订阅主题

Request:
```
POST /subscribe
```
Post Form:
```
qos : qos level
topic : topic filter
clientId : client id
```

Response:
```
{
    "code": 0,
    "result": []
}
```

#### 取消订阅

Request:
```
POST /unsubscribe
```
Post Form:
```
topic : topic name
clientId : client id
```

Response:
```
{
    "code": 0,
    "result": []
}
```

## Using `gmqtt/server` Package for Secondary Development
The features of build-in MQTT broker is not rich enough.It is not implementing some features such as Authentication, ACL etc..
So It is recommend to use `gmqtt/server` package to customized your broker: 

```
func main() {
	s := server.NewServer()
	ln, err := net.Listen("tcp",":1883")
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
	tlsln, err := tls.Listen("tcp",":8883",tlsConfig)
	if err != nil {
		log.Fatalln(err.Error())
		return
	}
	s.AddTCPListenner(ln)
	s.AddTCPListenner(tlsln)
	//Setting hook methods & configration before s.Run()
	s.OnConnect = .... Authentication
	s.OnSubscribe = ....ACL
	s.SetQueueQos0Messages(false)
	....
	
	
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
  username := client.ClientOptions().Username
  password := client.ClientOptions().Password
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
  if client.ClientOptions().Username == "root" { //alow root user to subscribe whatever he wants
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
  if client.ClientOptions().Username == "subscribeonly" {
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
* Benchmark test
* Vendoring
* Website monitor




