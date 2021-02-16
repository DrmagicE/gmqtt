mockgen -source=config/config.go -destination=./config/config_mock.go -package=config -self_package=github.com/DrmagicE/gmqtt/config
mockgen -source=persistence/queue/elem.go -destination=./persistence/queue/elem_mock.go -package=queue -self_package=github.com/DrmagicE/gmqtt/queue
mockgen -source=persistence/queue/queue.go -destination=./persistence/queue/queue_mock.go -package=queue -self_package=github.com/DrmagicE/gmqtt/queue
mockgen -source=persistence/session/session.go -destination=./persistence/session/session_mock.go -package=session -self_package=github.com/DrmagicE/gmqtt/session
mockgen -source=persistence/subscription/subscription.go -destination=./persistence/subscription/subscription_mock.go -package=subscription -self_package=github.com/DrmagicE/gmqtt/subscription
mockgen -source=persistence/unack/unack.go -destination=./persistence/unack/unack_mock.go -package=unack -self_package=github.com/DrmagicE/gmqtt/unack
mockgen -source=pkg/packets/packets.go -destination=./pkg/packets/packets_mock.go -package=packets -self_package=github.com/DrmagicE/gmqtt/packets
mockgen -source=plugin/auth/account_grpc.pb.go -destination=./plugin/auth/account_grpc.pb_mock.go -package=auth -self_package=github.com/DrmagicE/gmqtt/auth
mockgen -source=plugin/federation/federation.pb.go -destination=./plugin/federation/federation.pb_mock.go -package=federation -self_package=github.com/DrmagicE/gmqtt/federation
mockgen -source=plugin/federation/peer.go -destination=./plugin/federation/peer_mock.go -package=federation -self_package=github.com/DrmagicE/gmqtt/federation
mockgen -source=plugin/federation/membership.go -destination=./plugin/federation/membership_mock.go -package=federation -self_package=github.com/DrmagicE/gmqtt/federation
mockgen -source=retained/interface.go -destination=./retained/interface_mock.go -package=retained -self_package=github.com/DrmagicE/gmqtt/retained
mockgen -source=server/client.go -destination=./server/client_mock.go -package=server -self_package=github.com/DrmagicE/gmqtt/server
mockgen -source=server/persistence.go -destination=./server/persistence_mock.go -package=server -self_package=github.com/DrmagicE/gmqtt/server
mockgen -source=server/plugin.go -destination=./server/plugin_mock.go -package=server -self_package=github.com/DrmagicE/gmqtt/server
mockgen -source=server/server.go -destination=./server/server_mock.go -package=server -self_package=github.com/DrmagicE/gmqtt/server
mockgen -source=server/service.go -destination=./server/service_mock.go -package=server -self_package=github.com/DrmagicE/gmqtt/server
mockgen -source=server/stats.go -destination=./server/stats_mock.go -package=server -self_package=github.com/DrmagicE/gmqtt/server
mockgen -source=server/topic_alias.go -destination=./server/topic_alias_mock.go -package=server -self_package=github.com/DrmagicE/gmqtt/server

# reflection mode.
# gRPC streaming mock issue: https://github.com/golang/mock/pull/163
mockgen -package=federation -destination=/usr/local/gopath/src/github.com/DrmagicE/gmqtt/plugin/federation/federation_grpc.pb_mock.go  github.com/DrmagicE/gmqtt/plugin/federation  FederationClient,Federation_EventStreamClient
