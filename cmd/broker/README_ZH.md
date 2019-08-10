# 内置的MQTT服务器
__注意: 请将此服务器看作是一个例子，不建议在生产环境下直接使用__


下列命令将启动一个监听`1883`端口[tcp]和`8080`端口[websocket]的MQTT服务。
该服务同时使用`management`插件，监听`9090`端口提供http api服务，具体可参考`management` [README](https://github.com/DrmagicE/gmqtt/blob/master/plugin/management/README_ZH.md)

```
$ cd cmd/broker
$ go run main.go 
```

