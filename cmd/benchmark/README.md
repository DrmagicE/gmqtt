English [TODO]
# 压力测试工具

## 功能特性
提供如下测试工具：
* connect压力测试工具
* publish压力测试工具
* subscribe压力测试工具
## 安装
`$ go get -d github.com/DrmagicE/gmqtt/cmd/benchmark`

## 开始
开启要进行压力测试的MQTT broker
```
$ cd example/benchmark
$ go run main.go
```
### Connect 压力测试
```
$ cd cmd/benchmark
$ go run pub_benchmark.go -help
Usage:
  -C    clean session
  -c int
        number of clients (default 1000)
  -ci int
        connection interval (ms) (default 100)
  -h string
        host (default "localhost")
  -p string
        port (default ":1883")
  -pwd string
        password
  -t int
        timeout (second)
  -u string
        username

```
例如:
```
$ cd cmd/benchmark
$ go run connect_benchmark -c 10000 
```

### Publish 压力测试
```
$ go run pub_benchmark.go -help
Usage:
  -C    clean session
  -c int
        number of clients (default 1000)
  -ci int
        connection interval (ms) (default 100)
  -h string
        host (default "localhost")
  -i int
        publish interval (ms) (default 100)
  -n int
        number of messages to publish per client (default 200)
  -p string
        port (default ":1883")
  -pwd string
        password
  -qos int
        qos (default 1)
  -s int
        payload size (bytes) (default 256)
  -sub int
        number of clients which subscribe topic #
  -subqos int
        qos of subscriptions
  -t int
        timeout (second)
  -topic string
        topic name (default "topic_name")
  -u string
        username
```
例如:
```
$ cd cmd/benchmark
$ go run pub_benchmark -c 10000 
```
### Subscribe 压力测试
```
$ go run sub_benchmark.go -help
Usage:
  -C    clean session (default true)
  -c int
        number of clients (default 1000)
  -ci int
        connection interval (ms) (default 100)
  -h string
        host (default "localhost")
  -i int
        subscription interval (ms) (default 100)
  -n int
        number of subscriptios per client (default 200)
  -p string
        port (default ":1883")
  -pwd string
        password
  -qos int
        qos (default 1)
  -t int
        timeout (second)
  -topic string
        topic name prefix (default "topic_name")
  -u string
        username
```
例如:
```
$ cd cmd/benchmark
$ go run sub_benchmark -c 10000 
```

## Gmqtt 的测试结果
以下结果是针对`example/benchmark/main.go`情况的测试数据
### 测试环境
System:Win10, RAM:16GB,CPU:3.20GHz, 12核

### Connect 测试
测试连接10K客户端，每隔100ms连接一个客户端
```
$ go run connect_benchmark -c 10000
2018/11/25 23:13:24 starting benchmark testing:
2018/11/25 23:13:26 1726 clients connected,
2018/11/25 23:13:28 3428 clients connected,
2018/11/25 23:13:30 5125 clients connected,
2018/11/25 23:13:32 6822 clients connected,
2018/11/25 23:13:34 8618 clients connected,
2018/11/25 23:13:35 benchmark testing finished in 11 seconds
2018/11/25 23:13:35 10000 clients connected, QPS: 909

```

### Publish 测试

#### QOS1
测试连接10K客户端，每个客户端发布200个QOS1报文，每隔100ms连接一个客户端，每隔100ms发布一个报文
```
$ go run pub_benchmark.go -c 10000 -qos 1
2018/11/25 23:20:11 starting benchmark testing:
2018/11/25 23:20:13 527 clients connected,99325 messages published,0 messages distributed
2018/11/25 23:20:15 1058 clients connected,206922 messages published,0 messages distributed
2018/11/25 23:20:17 1549 clients connected,304566 messages published,0 messages distributed
2018/11/25 23:20:19 2037 clients connected,402535 messages published,0 messages distributed
2018/11/25 23:20:21 2526 clients connected,498198 messages published,0 messages distributed
2018/11/25 23:20:23 3082 clients connected,612538 messages published,0 messages distributed
2018/11/25 23:20:25 3713 clients connected,735173 messages published,0 messages distributed
2018/11/25 23:20:27 4335 clients connected,859045 messages published,0 messages distributed
2018/11/25 23:20:29 4994 clients connected,991270 messages published,0 messages distributed
2018/11/25 23:20:31 5610 clients connected,1115178 messages published,0 messages distributed
2018/11/25 23:20:33 6200 clients connected,1234436 messages published,0 messages distributed
2018/11/25 23:20:35 6752 clients connected,1347392 messages published,0 messages distributed
2018/11/25 23:20:37 7349 clients connected,1464449 messages published,0 messages distributed
2018/11/25 23:20:39 7941 clients connected,1581735 messages published,0 messages distributed
2018/11/25 23:20:41 8537 clients connected,1701229 messages published,0 messages distributed
2018/11/25 23:20:43 9084 clients connected,1808976 messages published,0 messages distributed
2018/11/25 23:20:45 9637 clients connected,1921901 messages published,0 messages distributed
2018/11/25 23:20:47 benchmark testing finished in 36 seconds
2018/11/25 23:20:47 10000 clients connected,2000000 messages published,0 messages distributed,QPS: 55833

```

#### QOS2
测试连接10K客户端，每个客户端发布200个QOS2报文，每隔100ms连接一个客户端，每隔100ms发布一个报文
```
$ go run pub_benchmark.go -c 10000 -qos 2
2018/11/25 23:22:10 starting benchmark testing:
2018/11/25 23:22:12 398 clients connected,74826 messages published,0 messages distributed
2018/11/25 23:22:14 767 clients connected,150447 messages published,0 messages distributed
2018/11/25 23:22:16 1146 clients connected,219581 messages published,0 messages distributed
2018/11/25 23:22:18 1553 clients connected,301553 messages published,0 messages distributed
2018/11/25 23:22:20 1928 clients connected,381894 messages published,0 messages distributed
2018/11/25 23:22:22 2383 clients connected,469265 messages published,0 messages distributed
2018/11/25 23:22:24 2822 clients connected,557121 messages published,0 messages distributed
2018/11/25 23:22:26 3307 clients connected,651712 messages published,0 messages distributed
2018/11/25 23:22:28 3768 clients connected,745827 messages published,0 messages distributed
2018/11/25 23:22:30 4223 clients connected,840623 messages published,0 messages distributed
2018/11/25 23:22:32 4716 clients connected,933273 messages published,0 messages distributed
2018/11/25 23:22:34 5214 clients connected,1032921 messages published,0 messages distributed
2018/11/25 23:22:36 5642 clients connected,1124033 messages published,0 messages distributed
2018/11/25 23:22:38 6103 clients connected,1213450 messages published,0 messages distributed
2018/11/25 23:22:40 6557 clients connected,1307392 messages published,0 messages distributed
2018/11/25 23:22:42 7059 clients connected,1401106 messages published,0 messages distributed
2018/11/25 23:22:44 7499 clients connected,1492729 messages published,0 messages distributed
2018/11/25 23:22:46 7985 clients connected,1584577 messages published,0 messages distributed
2018/11/25 23:22:48 8415 clients connected,1669812 messages published,0 messages distributed
2018/11/25 23:22:50 8832 clients connected,1761055 messages published,0 messages distributed
2018/11/25 23:22:52 9263 clients connected,1846837 messages published,0 messages distributed
2018/11/25 23:22:54 9745 clients connected,1943825 messages published,0 messages distributed
2018/11/25 23:22:56 benchmark testing finished in 46 seconds
2018/11/25 23:22:56 10000 clients connected,2000000 messages published,0 messages distributed,QPS: 43695

```

### QOS1 + 1个订阅客户端
测试连接10K个消息发布客户端外加1个消息订阅客户端（10K个生产者,1个消费者），每个消息发布客户端发布200个QOS1报文，每隔100ms连接一个客户端，每隔100ms发布一个报文。
消息订阅客户端订阅QOS1的`#`主题
```
$ go run pub_benchmark.go -c 10000 -qos 1 -sub 1 -subqos 1
2018/11/25 23:30:07 starting benchmark testing:
2018/11/25 23:30:09 1815 clients connected,113018 messages published,7453 messages distributed
2018/11/25 23:30:11 3566 clients connected,225708 messages published,8411 messages distributed
2018/11/25 23:30:13 4832 clients connected,339316 messages published,8979 messages distributed
2018/11/25 23:30:15 5754 clients connected,452733 messages published,9446 messages distributed
2018/11/25 23:30:17 6555 clients connected,567245 messages published,9851 messages distributed
2018/11/25 23:30:19 7241 clients connected,688740 messages published,10239 messages distributed
2018/11/25 23:30:21 7904 clients connected,813378 messages published,10611 messages distributed
2018/11/25 23:30:23 8497 clients connected,943972 messages published,10991 messages distributed
2018/11/25 23:30:25 8998 clients connected,1077739 messages published,11351 messages distributed
2018/11/25 23:30:27 9493 clients connected,1208318 messages published,11711 messages distributed
2018/11/25 23:30:29 9951 clients connected,1340560 messages published,12071 messages distributed
2018/11/25 23:30:31 10001 clients connected,1472450 messages published,12464 messages distributed
2018/11/25 23:30:33 10001 clients connected,1606225 messages published,12924 messages distributed
2018/11/25 23:30:35 10001 clients connected,1738827 messages published,13471 messages distributed
2018/11/25 23:30:37 10001 clients connected,1873431 messages published,14251 messages distributed
2018/11/25 23:30:39 benchmark testing finished in 32 seconds
2018/11/25 23:30:39 10001 clients connected,2000000 messages published,16486 messages distributed,QPS: 63327

```

### Subscribe 测试
测试连接10K个客户端，每个客户端订阅200个主题报文，每隔100ms连接一个客户端，每隔100ms订阅一个主题。
```
$ go run sub_benchmark.go -c 10000
2018/11/25 23:36:32 starting benchmark testing:
2018/11/25 23:36:34 472 clients connected,72939 topics subscribed,
2018/11/25 23:36:36 831 clients connected,143091 topics subscribed,
2018/11/25 23:36:38 1180 clients connected,213147 topics subscribed,
2018/11/25 23:36:40 1526 clients connected,283129 topics subscribed,
2018/11/25 23:36:42 1875 clients connected,352731 topics subscribed,
2018/11/25 23:36:44 2248 clients connected,426733 topics subscribed,
2018/11/25 23:36:46 10000 clients connected,516495 topics subscribed,
2018/11/25 23:36:48 10000 clients connected,650959 topics subscribed,
2018/11/25 23:36:50 10000 clients connected,779153 topics subscribed,
2018/11/25 23:36:52 10000 clients connected,904905 topics subscribed,
2018/11/25 23:36:54 10000 clients connected,999121 topics subscribed,
2018/11/25 23:36:56 10000 clients connected,1094615 topics subscribed,
2018/11/25 23:36:58 10000 clients connected,1179352 topics subscribed,
2018/11/25 23:37:00 10000 clients connected,1253898 topics subscribed,
2018/11/25 23:37:02 10000 clients connected,1315336 topics subscribed,
2018/11/25 23:37:04 10000 clients connected,1376505 topics subscribed,
2018/11/25 23:37:06 10000 clients connected,1437960 topics subscribed,
2018/11/25 23:37:08 10000 clients connected,1502334 topics subscribed,
2018/11/25 23:37:10 10000 clients connected,1564538 topics subscribed,
2018/11/25 23:37:12 10000 clients connected,1619792 topics subscribed,
2018/11/25 23:37:14 10000 clients connected,1678025 topics subscribed,
2018/11/25 23:37:16 10000 clients connected,1733498 topics subscribed,
2018/11/25 23:37:18 10000 clients connected,1783257 topics subscribed,
2018/11/25 23:37:20 10000 clients connected,1835458 topics subscribed,
2018/11/25 23:37:22 10000 clients connected,1885567 topics subscribed,
2018/11/25 23:37:24 10000 clients connected,1931617 topics subscribed,
2018/11/25 23:37:26 10000 clients connected,1979365 topics subscribed,
2018/11/25 23:37:27 benchmark testing finished in 55 seconds
2018/11/25 23:37:27 10000 clients connected,2000000 topics subscribed,QPS: 36545

```
