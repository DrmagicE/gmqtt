# Management
`Management` provide restful api for users to  query the current server state and do other operations. 

## API list
### Get All Active Clients
Request:
```
GET /clients?page=xxx&page_size=xxx
page: default to 1
page_size: default to 20
```
Response:
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

### Get Client By ID
Request:
```
GET /client/:id
```
Response:
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


### Get All Sessions

Request:
```
GET /sessions?page=xxx&page_size=xxx
page: default to 1
page_size: default to 20
```
response：
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

### Get Session By ID

Request:
```
GET /session/:id
```
Response:
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

### Get Subscriptions of the Client By ID

Request:
```
GET /subscriptions/:clientid?page=xxx&page_size=xxx
page: default to 1
page_size: default to 20
```
Response:
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
retain: retain flag, retain != ""  means this is a retained message
```

Response:
```
{
    "code": 0,
    "message": "",
    "data": {}
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
    "message": "",
    "data": {}
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
    "message": "",
    "data": {}
}
```

### Close Client

Request
```
DELETE /client/:id
```

Response
```
{
    "code": 0,
    "message": "",
    "data": {}
}
```