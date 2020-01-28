# Prometheus
`Prometheus` implements the prometheus exporter for gmqtt. 


# Metrics

metric name | Type | Labels 
---|---|---
gmqtt_clients_connected_total | Counter | 
gmqtt_messages_dropped_total | Counter | qos:  qos of the dropped message
gmqtt_packets_received_bytes_total | Counter | type: type of the packet
gmqtt_packets_received_total | Counter |  type: type of the packet
gmqtt_packets_sent_bytes_total | Counter | type: type of the packet
gmqtt_packets_sent_total | Counter | type: type of the packet
gmqtt_sessions_active_current | Gauge | 
gmqtt_sessions_expired_total | Counter |
gmqtt_sessions_inactive_current | Gauge |
gmqtt_subscriptions_current | Gauge |
gmqtt_subscriptions_total | Counter |
messages_queued_current | Gauge |
messages_received_total | Counter | qos: qos of the message
messages_sent_total | Counter | qos: qos of the message