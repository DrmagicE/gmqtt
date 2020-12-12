# admin

Admin plugin use [grpc-gateway](https://github.com/grpc-ecosystem/grpc-gateway) to provide both REST HTTP and GRPC APIs for integration with external systems.

# API Doc
 
See [swagger](https://github.com/DrmagicE/gmqtt/blob/master/plugin/admin/swagger)

# Examples

## List Clients
```bash
$ curl 127.0.0.1:8083/v1/clients
```
Response:
```json
{
      "clients": [
          {
              "client_id": "ab",
              "username": "",
              "keep_alive": 60,
              "version": 4,
              "remote_addr": "127.0.0.1:51637",
              "local_addr": "127.0.0.1:1883",
              "connected_at": "2020-12-12T12:26:36Z",
              "disconnected_at": null,
              "session_expiry": 7200,
              "max_inflight": 100,
              "inflight_len": 0,
              "max_queue": 100,
              "queue_len": 0,
              "subscriptions_current": 0,
              "subscriptions_total": 0,
              "packets_received_bytes": "54",
              "packets_received_nums": "3",
              "packets_send_bytes": "8",
              "packets_send_nums": "2",
              "message_dropped": "0"
          }
      ],
      "total_count": 1
  }
```

## Filter Subscriptions
```bash
$ curl 127.0.0.1:8083/v1/filter_subscriptions?filter_type=1,2,3&match_type=1&topic_name=/a
```
This curl is able to filter the subscription that the topic name is equal to "/a".

Response:
```json
{
    "subscriptions": [
        {
            "topic_name": "/a",
            "id": 0,
            "qos": 1,
            "no_local": false,
            "retain_as_published": false,
            "retain_handling": 0,
            "client_id": "ab"
        }
    ]
}
```

## Publish Message 
```bash
$ curl -X POST 127.0.0.1:8083/v1/publish -d '{"topic_name":"a","payload":"test","qos":1}'
```
This curl will publish the message to the broker.The broker will check if there are matched topics and
send the message to the subscribers, just like received a message from a MQTT client.