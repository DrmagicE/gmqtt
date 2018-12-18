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
$ go run connect_benchmark.go -c 10000 
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
        publishing interval (ms) (default 100)
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
        subscribing  interval (ms) (default 100)
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
$ go run connect_benchmark.go -c 10000
2018/12/02 18:25:25 starting benchmark testing:
2018/12/02 18:25:27 1905 clients connected,
2018/12/02 18:25:29 3763 clients connected,
2018/12/02 18:25:31 5694 clients connected,
2018/12/02 18:25:33 7458 clients connected,
2018/12/02 18:25:35 9363 clients connected,
2018/12/02 18:25:36 benchmark testing finished in 11 seconds
2018/12/02 18:25:36 10000 clients connected, QPS: 909
```

### Publish 测试

#### QOS1
测试连接10K客户端，每个客户端发布200个QOS1报文，每隔100ms连接一个客户端，每隔100ms发布一个报文
```
$ go run pub_benchmark.go -c 10000 -qos 1
2018/12/02 18:30:38 starting benchmark testing:
2018/12/02 18:30:40 1843 clients connected,133088 messages published,0 messages distributed
2018/12/02 18:30:42 3673 clients connected,264462 messages published,0 messages distributed
2018/12/02 18:30:44 5438 clients connected,393482 messages published,0 messages distributed
2018/12/02 18:30:46 7298 clients connected,523553 messages published,0 messages distributed
2018/12/02 18:30:48 9146 clients connected,653704 messages published,0 messages distributed
2018/12/02 18:30:50 10000 clients connected,789155 messages published,0 messages distributed
2018/12/02 18:30:52 10000 clients connected,933973 messages published,0 messages distributed
2018/12/02 18:30:54 10000 clients connected,1078592 messages published,0 messages distributed
2018/12/02 18:30:56 10000 clients connected,1225945 messages published,0 messages distributed
2018/12/02 18:30:58 10000 clients connected,1368634 messages published,0 messages distributed
2018/12/02 18:31:00 10000 clients connected,1516132 messages published,0 messages distributed
2018/12/02 18:31:02 10000 clients connected,1661843 messages published,0 messages distributed
2018/12/02 18:31:04 10000 clients connected,1810678 messages published,0 messages distributed
2018/12/02 18:31:06 10000 clients connected,1957730 messages published,0 messages distributed
2018/12/02 18:31:06 benchmark testing finished in 28 seconds
2018/12/02 18:31:06 10000 clients connected,2000000 messages published,0 messages distributed,QPS: 71785
```

#### QOS2
测试连接10K客户端，每个客户端发布200个QOS2报文，每隔100ms连接一个客户端，每隔100ms发布一个报文
```
$ go run pub_benchmark.go -c 10000 -qos 2
2018/12/02 18:32:49 starting benchmark testing:
2018/12/02 18:32:51 1862 clients connected,66760 messages published,0 messages distributed
2018/12/02 18:32:53 3703 clients connected,132009 messages published,0 messages distributed
2018/12/02 18:32:55 5511 clients connected,197031 messages published,0 messages distributed
2018/12/02 18:32:57 7361 clients connected,262194 messages published,0 messages distributed
2018/12/02 18:32:59 9142 clients connected,328997 messages published,0 messages distributed
2018/12/02 18:33:01 10000 clients connected,402515 messages published,0 messages distributed
2018/12/02 18:33:03 10000 clients connected,478835 messages published,0 messages distributed
2018/12/02 18:33:05 10000 clients connected,554997 messages published,0 messages distributed
2018/12/02 18:33:07 10000 clients connected,632324 messages published,0 messages distributed
2018/12/02 18:33:09 10000 clients connected,709051 messages published,0 messages distributed
2018/12/02 18:33:11 10000 clients connected,784347 messages published,0 messages distributed
2018/12/02 18:33:13 10000 clients connected,859663 messages published,0 messages distributed
2018/12/02 18:33:15 10000 clients connected,936790 messages published,0 messages distributed
2018/12/02 18:33:17 10000 clients connected,1012546 messages published,0 messages distributed
2018/12/02 18:33:19 10000 clients connected,1089786 messages published,0 messages distributed
2018/12/02 18:33:21 10000 clients connected,1167063 messages published,0 messages distributed
2018/12/02 18:33:23 10000 clients connected,1244063 messages published,0 messages distributed
2018/12/02 18:33:25 10000 clients connected,1317821 messages published,0 messages distributed
2018/12/02 18:33:27 10000 clients connected,1394318 messages published,0 messages distributed
2018/12/02 18:33:29 10000 clients connected,1470159 messages published,0 messages distributed
2018/12/02 18:33:31 10000 clients connected,1546872 messages published,0 messages distributed
2018/12/02 18:33:33 10000 clients connected,1623610 messages published,0 messages distributed
2018/12/02 18:33:35 10000 clients connected,1700876 messages published,0 messages distributed
2018/12/02 18:33:37 10000 clients connected,1777730 messages published,0 messages distributed
2018/12/02 18:33:39 10000 clients connected,1856084 messages published,0 messages distributed
2018/12/02 18:33:41 10000 clients connected,1933742 messages published,0 messages distributed
2018/12/02 18:33:43 benchmark testing finished in 54 seconds
2018/12/02 18:33:43 10000 clients connected,2000000 messages published,0 messages distributed,QPS: 37222

```

### QOS1 + 1个订阅客户端
测试连接10K个消息发布客户端外加1个消息订阅客户端（10K个生产者,1个消费者），每个消息发布客户端发布200个QOS1报文，每隔100ms连接一个客户端，每隔100ms发布一个报文。
消息订阅客户端订阅QOS1的`#`主题
```
$ go run pub_benchmark.go -c 10000 -qos 1 -sub 1 -subqos 1
2018/12/02 18:38:27 starting benchmark testing:
2018/12/02 18:38:29 1825 clients connected,87983 messages published,87983 messages distributed
2018/12/02 18:38:31 3604 clients connected,175514 messages published,175511 messages distributed
2018/12/02 18:38:33 5356 clients connected,261674 messages published,261673 messages distributed
2018/12/02 18:38:35 7127 clients connected,348470 messages published,348467 messages distributed
2018/12/02 18:38:37 8845 clients connected,434100 messages published,434098 messages distributed
2018/12/02 18:38:39 10001 clients connected,524853 messages published,524822 messages distributed
2018/12/02 18:38:41 10001 clients connected,621653 messages published,621653 messages distributed
2018/12/02 18:38:43 10001 clients connected,717125 messages published,717124 messages distributed
2018/12/02 18:38:45 10001 clients connected,811075 messages published,811070 messages distributed
2018/12/02 18:38:47 10001 clients connected,907651 messages published,907649 messages distributed
2018/12/02 18:38:49 10001 clients connected,1000234 messages published,1000234 messages distributed
2018/12/02 18:38:51 10001 clients connected,1095560 messages published,1095676 messages distributed
2018/12/02 18:38:53 10001 clients connected,1191145 messages published,1190690 messages distributed
2018/12/02 18:38:55 10001 clients connected,1286594 messages published,1286593 messages distributed
2018/12/02 18:38:57 10001 clients connected,1382244 messages published,1382244 messages distributed
2018/12/02 18:38:59 10001 clients connected,1478770 messages published,1478769 messages distributed
2018/12/02 18:39:01 10001 clients connected,1574419 messages published,1574081 messages distributed
2018/12/02 18:39:03 10001 clients connected,1669853 messages published,1669850 messages distributed
2018/12/02 18:39:05 10001 clients connected,1767104 messages published,1767103 messages distributed
2018/12/02 18:39:07 10001 clients connected,1863912 messages published,1863908 messages distributed
2018/12/02 18:39:09 10001 clients connected,1961524 messages published,1961519 messages distributed
2018/12/02 18:39:10 benchmark testing finished in 43 seconds
2018/12/02 18:39:10 10001 clients connected,2000000 messages published,1999997 messages distributed,QPS: 93255


```

### Subscribe 测试
测试连接10K个客户端，每个客户端订阅200个主题报文，每隔100ms连接一个客户端，每隔100ms订阅一个主题。
```
$ go run sub_benchmark.go -c 10000
2018/12/02 18:42:13 starting benchmark testing:
2018/12/02 18:42:15 1881 clients connected,100977 topics subscribed,
2018/12/02 18:42:17 3667 clients connected,200863 topics subscribed,
2018/12/02 18:42:19 5487 clients connected,299493 topics subscribed,
2018/12/02 18:42:21 7264 clients connected,396722 topics subscribed,
2018/12/02 18:42:23 9025 clients connected,492015 topics subscribed,
2018/12/02 18:42:25 10000 clients connected,593226 topics subscribed,
2018/12/02 18:42:27 10000 clients connected,689092 topics subscribed,
2018/12/02 18:42:29 10000 clients connected,775540 topics subscribed,
2018/12/02 18:42:31 10000 clients connected,861801 topics subscribed,
2018/12/02 18:42:33 10000 clients connected,942405 topics subscribed,
2018/12/02 18:42:35 10000 clients connected,1014145 topics subscribed,
2018/12/02 18:42:37 10000 clients connected,1089165 topics subscribed,
2018/12/02 18:42:39 10000 clients connected,1160592 topics subscribed,
2018/12/02 18:42:41 10000 clients connected,1224802 topics subscribed,
2018/12/02 18:42:43 10000 clients connected,1290350 topics subscribed,
2018/12/02 18:42:45 10000 clients connected,1353208 topics subscribed,
2018/12/02 18:42:47 10000 clients connected,1410131 topics subscribed,
2018/12/02 18:42:49 10000 clients connected,1468550 topics subscribed,
2018/12/02 18:42:51 10000 clients connected,1525145 topics subscribed,
2018/12/02 18:42:53 10000 clients connected,1582015 topics subscribed,
2018/12/02 18:42:55 10000 clients connected,1634913 topics subscribed,
2018/12/02 18:42:57 10000 clients connected,1690029 topics subscribed,
2018/12/02 18:42:59 10000 clients connected,1744026 topics subscribed,
2018/12/02 18:43:01 10000 clients connected,1793471 topics subscribed,
2018/12/02 18:43:03 10000 clients connected,1845218 topics subscribed,
2018/12/02 18:43:05 10000 clients connected,1894890 topics subscribed,
2018/12/02 18:43:07 10000 clients connected,1939596 topics subscribed,
2018/12/02 18:43:09 10000 clients connected,1986118 topics subscribed,
2018/12/02 18:43:10 benchmark testing finished in 57 seconds
2018/12/02 18:43:10 10000 clients connected,2000000 topics subscribed,QPS: 35263

```
