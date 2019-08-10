# Management
`Management`插件会启动一个http server提供http api服务，提供查询和管理接口。

## API列表
### 获取所有在线客户端
请求格式：
```
GET /clients?page=xxx&page_size=xxx
page:请求页数，不传默认第一页
page_size:每一页的条数，不传默认20条
```
响应格式：
```
{
    "code": 0,
    "message": "",
    "data": {
        "pager": {
            "page": 1,
            "page_size": 10,
            "count": 1
        },
        "result": [
            {
                "client_id": "id",
                "username": "root",
                "password": "rootpwd",
                "keep_alive": 65,
                "clean_session": false,
                "will_flag": false,
                "will_retain": false,
                "will_qos": 0,
                "will_topic": "",
                "will_payload": "",
                "remote_addr": "127.0.0.1:49252",
                "local_addr": "127.0.0.1:1883",
                "connected_at": "2019-08-10T23:29:56+08:00",
                "disconnected_at": "1970-01-01T08:00:00+08:00"
            }
        ]
    }
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
    "code": 0,
    "message": "",
    "data": {
        "client_id": "id",
        "username": "root",
        "password": "rootpwd",
        "keep_alive": 65,
        "clean_session": false,
        "will_flag": false,
        "will_retain": false,
        "will_qos": 0,
        "will_topic": "",
        "will_payload": "",
        "remote_addr": "127.0.0.1:49252",
        "local_addr": "127.0.0.1:1883",
        "connected_at": "2019-08-10T23:29:56+08:00",
        "disconnected_at": "1970-01-01T08:00:00+08:00"
    }
}
```


### 获取所有会话（session）

请求格式：
```
GET /sessions?page=xxx&page_size=xxx
page:请求页数，不传默认第一页
page_size:每一页的条数，不传默认20条
```
响应格式：
```
{
    "code": 0,
    "message": "",
    "data": {
        "pager": {
            "page": 1,
            "page_size": 20,
            "count": 1
        },
        "result": [
            {
                "client_id": "id",
                "status": "online",
                "clean_session": false,
                "subscriptions": 0,
                "max_inflight": 32,
                "inflight_len": 0,
                "max_msg_queue": 1000,
                "msg_queue_len": 0,
                "max_await_rel": 100,
                "await_rel_len": 0,
                "msg_dropped_total": 0,
                "msg_delivered_total": 0,
                "connected_at": "2019-08-10T23:29:56+08:00",
                "disconnected_at": "1970-01-01T08:00:00+08:00"
            }
        ]
    }
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
    "code": 0,
    "message": "",
    "data": {
        "client_id": "id",
        "status": "online",
        "clean_session": false,
        "subscriptions": 0,
        "max_inflight": 32,
        "inflight_len": 0,
        "max_msg_queue": 1000,
        "msg_queue_len": 0,
        "max_await_rel": 100,
        "await_rel_len": 0,
        "msg_dropped_total": 0,
        "msg_delivered_total": 0,
        "connected_at": "2019-08-10T23:29:56+08:00",
        "disconnected_at": "1970-01-01T08:00:00+08:00"
    }
}
```

### 获取指定会话的订阅主题信息
请求格式：
```
GET /subscriptions/:clientid?page=xxx&page_size=xxx
page:请求页数，不传默认第一页
page_size:每一页的条数，不传默认20条
```
响应格式：
```
{
    "code": 0,
    "message": "",
    "data": [
        {
            "client_id": "id",
            "qos": 1,
            "name": "test1",
            "at": "2019-08-10T23:36:53.7859575+08:00"
        },
        {
            "client_id": "id",
            "qos": 0,
            "name": "test2",
            "at": "2019-08-10T23:36:53.7859575+08:00"
        }
    ]
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
retain: 是否保留消息，非空字符串表保留消息
```

响应格式：
```
{
    "code": 0,
    "message": "",
    "data": {}
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
    "message": "",
    "data": {}
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
    "message": "",
    "data": {}
}
```

### 关闭指定客户端连接

请求格式：
```
DELETE /client/:id
```

响应格式：
```
{
    "code": 0,
    "message": "",
    "data": {}
}
```