# Federation
**WARNING: This is an experimental feature, and has never been used in production environment.**

Federation is a kind of clustering mechanism which provides high-availability and horizontal scaling.
In Federation mode, multiple gmqtt brokers can be grouped together and "act as one".
However, it is impossible to fulfill all requirements in MQTT specification in a distributed environment.
There are some limitations:  
1. Persistent session cannot be resumed from another node.  
2. Clients with same client id can connect to different nodes at the same time and will not be kicked out.

This is because session information only stores in local node and does not share between nodes.

## Quick Start
The following commands will start a two nodes federation, the configuration files can be found [here](./examples).  
Start node1 in Terminal1:
```bash
$ gmqttd start -c path/to/retry_join/node1_config.yml
```
Start node2 in Terminate2:
```bash
$ gmqttd start -c path/to/retry_join/node2_config2.yml
```
After node1 and node2 is started, they will join into one federation atomically. 

We can test the federation with `mosquitto_pub/sub`:  
Connect to node2 and subscribe topicA:
```bash
$ mosquitto_sub -t topicA -h 127.0.0.1 -p 1884
```
Connect to node1 and send a message to topicA:
```bash
$ mosquitto_pub -t topicA -m 123 -h 127.0.0.1 -p 1883
```
The `mosquitto_sub` will receive "123" and print it in the terminal.
```bash
$ mosquitto_sub -t topicA -h 127.0.0.1 -p 1884
123
```

## Join Nodes via REST API
Federation provides gRPC/REST API to join/leave and query members information, see [swagger](./swagger/federation.swagger.json) for details.
In addition to join nodes upon starting up, you can join a node into federation by using `Join` API.  

Start node3 with the configuration with empty `retry_join` which means that the node will not join any nodes upon starting up.
```bash
$ gmqttd start -c path/to/retry_join/join_node3_config.yml
```
We can send `Join` request to any nodes in the federation to get node3 joined, for example, sends `Join` request to node1:
```bash
$ curl -X POST -d '{"hosts":["127.0.0.1:8932"]}'  '127.0.0.1:8083/v1/federation/join' 
{}                                                                                                
```
And check the members in federation: 
```bash
curl http://127.0.0.1:8083/v1/federation/members   
{
    "members": [
        {
            "name": "node1",
            "addr": "192.168.0.105:8902",
            "tags": {
                "fed_addr": "192.168.0.105:8901"
            },
            "status": "STATUS_ALIVE"
        },
        {
            "name": "node2",
            "addr": "192.168.0.105:8912",
            "tags": {
                "fed_addr": "192.168.0.105:8911"
            },
            "status": "STATUS_ALIVE"
        },
        {
            "name": "node3",
            "addr": "192.168.0.105:8932",
            "tags": {
                "fed_addr": "192.168.0.105:8931"
            },
            "status": "STATUS_ALIVE"
        }
    ]
}%
```
You will see there are 3 nodes ara alive in the federation.

## Configuration
```go
// Config is the configuration for the federation plugin.
type Config struct {
	// NodeName is the unique identifier for the node in the federation. Defaults to hostname.
	NodeName string `yaml:"node_name"`
	// FedAddr is the gRPC server listening address for the federation internal communication.
	// Defaults to :8901.
	// If the port is missing, the default federation port (8901) will be used.
	FedAddr string `yaml:"fed_addr"`
	// AdvertiseFedAddr is used to change the federation gRPC server address that we advertise to other nodes in the cluster.
	// Defaults to "FedAddr" or the private IP address of the node if the IP in "FedAddr" is 0.0.0.0.
	// However, in some cases, there may be a routable address that cannot be bound.
	// If the port is missing, the default federation port (8901) will be used.
	AdvertiseFedAddr string `yaml:"advertise_fed_addr"`
	// GossipAddr is the address that the gossip will listen on, It is used for both UDP and TCP gossip. Defaults to :8902
	GossipAddr string `yaml:"gossip_addr"`
	// AdvertiseGossipAddr is used to change the gossip server address that we advertise to other nodes in the cluster.
	// Defaults to "GossipAddr" or the private IP address of the node if the IP in "GossipAddr" is 0.0.0.0.
	// If the port is missing, the default gossip port (8902) will be used.
	AdvertiseGossipAddr string `yaml:"advertise_gossip_addr"`
	// RetryJoin is the address of other nodes to join upon starting up.
	// If port is missing, the default gossip port (8902) will be used.
	RetryJoin []string `yaml:"retry_join"`
	// RetryInterval is the time to wait between join attempts. Defaults to 5s.
	RetryInterval time.Duration `yaml:"retry_interval"`
	// RetryTimeout is the timeout to wait before joining all nodes in RetryJoin successfully.
	// If timeout expires, the server will exit with error. Defaults to 1m.
	RetryTimeout time.Duration `yaml:"retry_timeout"`
	// SnapshotPath will be pass to "SnapshotPath" in serf configuration.
	// When Serf is started with a snapshot,
	// it will attempt to join all the previously known nodes until one
	// succeeds and will also avoid replaying old user events.
	SnapshotPath string `yaml:"snapshot_path"`
	// RejoinAfterLeave will be pass to "RejoinAfterLeave" in serf configuration.
	// It controls our interaction with the snapshot file.
	// When set to false (default), a leave causes a Serf to not rejoin
	// the cluster until an explicit join is received. If this is set to
	// true, we ignore the leave, and rejoin the cluster on start.
	RejoinAfterLeave bool `yaml:"rejoin_after_leave"`
}
```

## Implementation Details

### Inner-node Communication
Nodes in the same federation communicate with each other through a couple of gRPC streaming apis:
```proto
message Event {
    uint64 id = 1;
    oneof Event {
        Subscribe Subscribe = 2;
        Message message = 3;
        Unsubscribe unsubscribe = 4;
    }
}
service Federation {
    rpc Hello(ClientHello) returns (ServerHello){}
    rpc EventStream (stream Event) returns (stream Ack){}
}
```
In general, a node is both Client and Server which implements the `Federation` gRPC service. 
* As Client, the node will send subscribe, unsubscribe and message published events to other nodes if necessary.  
Each event has a EventID, which is incremental and unique in a session. 
* As Server, when receives a event from Client, the node returns an acknowledgement after the event has been handled successfully.

### Session State
The event is designed to be idempotent and will be delivered at least once, just like the QoS 1 message in MQTT protocol.
In order to implement QoS 1 protocol flows, the Client and Server need to associate state with a SessionID, 
this is referred to as the Session State. The Server also stores the federation tree and retained messages as part of the Session State.

The Session State in the Client consists of:
 * Events which have been sent to the Server, but have not been acknowledged.
 * Events pending transmission to the Server.

The Session State in the Server consists of:
 * The existence of a Session, even if the rest of the Session State is empty.
 * The EventID of the next event that the Server is willing to accept.
 * Events which have been received from the Client, but have not sent acknowledged yet.
 
The Session State stores in memory only. When the Client starts, it generates a random UUID as SessionID.
When the Client detects a new node is joined or reconnects to the Server, it sends the `Hello` request which contains the SessionID to perform a handshake.
During the handshake, the Server will check whether the session for the SessionID exists.  

* If the session not exists, the Server sends response with `clean_start=true`. 
* If the session exists, the Server sends response with `clean_start=false` and sets the next EventID that it is willing to accept to `next_event_id`.  

After handshake succeed, the Client will start `EventStream`: 
* If the Client receives `clean_start=true`, it sends all local subscriptions and retained messages to the Server in order to sync the full state.
* If the Client receives `clean_start=false`, it sends events of which the EventID is greater than or equal to `next_event_id`.

### Subscription Tree
Each node in the federation will have two subscription trees, the local tree and the federation tree.
The local tree stores subscriptions for local clients which is managed by gmqtt core and the federation tree stores the subscriptions for remote nodes which is managed by the federation plugin.
The federation tree takes node name as subscriber identifier for subscriptions. 
* When receives a sub/unsub packet from a local client, the node will update it's local tree first and then broadcasts the event to other nodes. 
* When receives sub/unsub event from a remote node, the node will only update it's federation tree.  

With the federation tree, the node can determine which node the incoming message should be routed to. 
For example, Node1 and Node2 are in the same federation. Client1 connects to Node1 and subscribes to topic a/b, the subscription trees of these two nodes are as follows:  

Node1 local tree:  

| subscriber | topic | 
|------------|-------|
| client1 | a/b |

Node1 federation tree:    
empty.

Node2 local tree:  
empty.

Node2 federation tree:  
    
| subscriber | topic | 
|------------|-------|
| node1 | a/b |

### Message Distribution Process
When an MQTT client publishes a message, the node where it is located queries the federation tree 
and forwards the message to the relevant node according to the message topic, 
and then the relevant node retrieves the local subscription tree and sends the message to the relevant subscriber.

### Membership Management
Federation uses [Serf](https://github.com/hashicorp/serf) to manage membership.  


