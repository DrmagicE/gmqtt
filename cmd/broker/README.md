# Build-in MQTT Broker
__Notic: This is just an example to show a way of how to use this library. It is not recommend to use it in production environment.__

Use the following command to start a simple broker that listens on port 1883 for TCP and 8080 for websocket.
```
$ cd cmd/broker
$ go run main.go
```
## Configration
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
http_server: {addr: ":9090",user: { admin: "admin"}}
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

## REST API
Using HTTP Basic Authentication.Set your username and password in configration file 
### Get All Active Clients
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

### Get Client By ID
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


### Get All Sessions

Request:
```
GET /sessions?page=xxx&per-page=xxx
page: page, default to 1
per-page:pagesize, default to 20
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

### Get Session By ID

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

### Get All Subscriptions

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

### Get Subscriptions of the Client By ID

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



### Publish

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

### Subscribe

Request:
```
POST /subscribe
```
Post Form:
```
qos : qos level
topic : topic filter
clientID : client id
```

Response:
```
{
    "code": 0,
    "result": []
}
```

### UnSubscribe

Request:
```
POST /unsubscribe
```
Post Form:
```
topic : topic name
clientID : client id
```

Response:
```
{
    "code": 0,
    "result": []
}
```