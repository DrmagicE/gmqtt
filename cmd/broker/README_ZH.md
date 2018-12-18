# 内置的MQTT服务器
__注意: 请将此服务器看作是一个例子，不建议在生产环境下直接使用__

下列命令将监听`1883`端口[tcp]和`8080`端口[websocket]，开启MQTT服务器。
```
$ cd cmd/broker
$ go run main.go 
```
## 配置文件
使用yaml的配置文件格式，例子见：`cmd/broker/config.yaml`
```
# 超时重传间隔秒数，默认20秒
delivery_retry_interval: 20
# 最大飞行窗口，默认20条
max_inflight_messages: 20
# 是否为离线的保持会话客户端转发QoS0消息，默认转发
queue_qos0_messages: true
# 缓存队列最大容量，默认20条
max_msgqueue_messages: 20
# pprof 监控文件，默认不开启pprof
# pprof.cpu CPU监控文件
# pprof.mem 内存监控文件
profile: {cpu: "cpuprofile", mem: "memprofile"}
# 是否打印日志，调试时使用，默认false不打印
logging: false
# http_server服务监听地址
# http_server.addr http服务监听地址
# http_server.user http basic auth的用户名密码,key是用户名，value是密码
http_server: {addr: ":9090",user: { admin: "admin"}}

# listener
# listener.$.protocol 支持mqtt或者websocket
# listener.$.addr 监听的端口,  用作填充net.Listen(network, address string) 中的address参数
# listener.$.certfile 如果使用tls/ssl，填写cert文件路径
# listenr.$.keyfile 如果使用tls/ssl，填写key文件路径
listener:
- {protocol: mqtt, addr: ':1883', certfile: , keyfile:  }
- {protocol: websocket, addr: ':8080', certfile: ,keyfile: }



```
默认使用`cmd/broker/config.yaml`配置文件，使用下列命令可以设置配置文件路径
```
$ go run main.go -config <config-file-path>
```

## REST服务
通过HTTP Basic Auth进行鉴权，用户名密码配置见配置文件
### 获取所有在线客户端
请求格式：
```
GET /clients?page=xxx&per-page=xxx
page:请求页数，不传默认第一页
per-page:每一页的条数，不传默认20条
```
响应格式：
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

### 获取指定客户端id的客户端
请求格式：
```
GET /client/:id
```
响应格式：
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


### 获取所有会话（session）

请求格式：
```
GET /sessions?page=xxx&per-page=xxx
page:请求页数，不传默认第一页
per-page:每一页的条数，不传默认20条
```
响应格式：
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

### 获取指定客户端id的会话

请求格式：
```
GET /session/:id
```
响应格式：
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

### 获取所有订阅主题信息

请求格式：
```
GET /subscriptions
```
响应格式：
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

### 获取指定会话的订阅主题信息

请求格式：
```
GET /subscriptions/:id
```
响应格式：
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



### 发布主题

请求格式：
```
POST /publish
```
POST请求参数：
```
qos : qos等级
topic : 发布的主题名称
payload : 主题payload
```

响应格式：
```
{
    "code": 0,
    "result": []
}
```

### 订阅主题

请求格式：
```
POST /subscribe
```
POST请求参数：
```
qos : qos等级
topic : 订阅的主题名称
clientID : 订阅的客户端id
```

响应格式：
```
{
    "code": 0,
    "result": []
}
```

### 取消订阅

请求格式：
```
POST /unsubscribe
```
POST请求参数：
```
topic : 需要取消订阅的主题名称
clientID : 需要取消订阅的客户端id
```

响应格式：
```
{
    "code": 0,
    "result": []
}
```