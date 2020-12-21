# Prometheus
`Prometheus` implements the prometheus exporter for gmqtt.   
Default URL: 127.0.0.1:8082/metrics

# Metrics

metric name | Type | Labels 
---|---|---
gmqtt_clients_connected_total | Counter | 
gmqtt_messages_dropped_total | Counter | qos:  qos of the dropped message
gmqtt_packets_received_bytes_total | Counter | type: type of the packet
gmqtt_packets_received_total | Counter |  type: type of the packet
gmqtt_packets_sent_bytes_total | Counter | type: type of the packet
gmqtt_packets_sent_total | Counter | type: type of the packet
gmqtt_sessions_created_total | Counter | 
gmqtt_sessions_terminated_total | Counter | reason: the reason of termination. (expired|taken_over|normal)
gmqtt_sessions_active_current | Gauge | 
gmqtt_sessions_expired_total | Counter |
gmqtt_sessions_inactive_current | Gauge |
gmqtt_subscriptions_current | Gauge |
gmqtt_subscriptions_total | Counter |
gmqtt_messages_queued_current | Gauge |
gmqtt_messages_received_total | Counter | qos: qos of the message
gmqtt_messages_sent_total | Counter | qos: qos of the message