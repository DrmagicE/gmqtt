# Build-in MQTT Broker
__Notic: This is just an example to show a way of how to use this library. It is not recommend to use it in production environment.__

Use the following command to start a simple broker that listens on port 1883 for TCP and 8080 for websocket.
The broker also use `management` plugin which listens on port 9090 to provide restful api service.See [management](https://github.com/DrmagicE/gmqtt/blob/master/plugin/management/READEME.md) for more details.

```
$ cd cmd/broker
$ go run main.go

