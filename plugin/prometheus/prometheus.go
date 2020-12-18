package prometheus

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/DrmagicE/gmqtt/config"
	"github.com/DrmagicE/gmqtt/persistence/subscription"
	"github.com/DrmagicE/gmqtt/server"
)

func init() {
	server.RegisterPlugin(Name, New)
	config.RegisterDefaultPluginConfig(Name, &DefaultConfig)
}

func New(ctx context.Context, config config.Config) (server.Plugin, error) {
	cfg := config.Plugins[Name].(*Config)
	httpServer := &http.Server{
		Addr: cfg.ListenAddress,
	}
	return &Prometheus{
		httpServer: httpServer,
		path:       cfg.Path,
	}, nil
}

const (
	Name         = "prometheus"
	metricPrefix = "gmqtt_"
)

var log *zap.Logger

func (c *Config) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type cfg Config
	var v = &struct {
		Prometheus cfg `yaml:"prometheus"`
	}{
		Prometheus: cfg(DefaultConfig),
	}
	if err := unmarshal(v); err != nil {
		return err
	}
	empty := cfg(Config{})
	if v.Prometheus == empty {
		v.Prometheus = cfg(DefaultConfig)
	}
	*c = Config(v.Prometheus)
	return nil
}

// Config is the configuration of prometheus exporter plugin.
type Config struct {
	// ListenAddress is the address that the exporter will listen on.
	ListenAddress string `yaml:"listen_address"`
	// Path is the exporter url path.
	Path string `yaml:"path"`
}

func (c *Config) Validate() error {
	_, _, err := net.SplitHostPort(c.ListenAddress)
	if err != nil {
		return errors.New("invalid listner_address")
	}
	return nil
}

// DefaultConfig is the default configuration.
var DefaultConfig = Config{
	ListenAddress: ":8082",
	Path:          "/metrics",
}

// Prometheus served as a prometheus exporter that exposes gmqtt metrics.
type Prometheus struct {
	statsManager server.StatsReader
	httpServer   *http.Server
	path         string
}

func (p *Prometheus) Load(service server.Server) error {
	log = server.LoggerWithField(zap.String("plugin", Name))
	p.statsManager = service.StatsManager()
	r := prometheus.DefaultRegisterer
	r.MustRegister(p)
	mu := http.NewServeMux()
	mu.Handle(p.path, promhttp.Handler())
	p.httpServer.Handler = mu
	go func() {
		err := p.httpServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			panic(err.Error())
		}
	}()
	return nil
}
func (p *Prometheus) Unload() error {
	return p.httpServer.Shutdown(context.Background())
}
func (p *Prometheus) HookWrapper() server.HookWrapper {
	return server.HookWrapper{}
}
func (p *Prometheus) Name() string {
	return Name
}

func (p *Prometheus) Describe(desc chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(p, desc)
}

func (p *Prometheus) Collect(m chan<- prometheus.Metric) {
	log.Debug("metrics collected")
	st := p.statsManager.GetGlobalStats()
	collectPacketsStats(&st.PacketStats, m)
	collectClientStats(&st.ConnectionStats, m)
	collectSubscriptionStats(&st.SubscriptionStats, m)
	collectMessageStats(&st.MessageStats, m)
}

func collectPacketsStats(ps *server.PacketStats, m chan<- prometheus.Metric) {
	bytesReceivedMetricName := metricPrefix + "packets_received_bytes_total"
	ReceivedCounterMetricName := metricPrefix + "packets_received_total"
	bytesSentMetricName := metricPrefix + "packets_sent_bytes_total"
	sentCounterMetricName := metricPrefix + "packets_sent_total"

	collectPacketsStatsBytes(bytesReceivedMetricName, &ps.BytesReceived, m)
	collectPacketsStatsBytes(bytesSentMetricName, &ps.BytesSent, m)

	collectPacketsStatsCounter(ReceivedCounterMetricName, &ps.ReceivedTotal, m)
	collectPacketsStatsCounter(sentCounterMetricName, &ps.SentTotal, m)
}
func collectPacketsStatsBytes(metricName string, pb *server.PacketBytes, m chan<- prometheus.Metric) {
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", []string{"type"}, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&pb.Connect)),
		"CONNECT",
	)
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", []string{"type"}, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&pb.Connack)),
		"CONNACK",
	)
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", []string{"type"}, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&pb.Disconnect)),
		"DISCONNECT",
	)
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", []string{"type"}, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&pb.Pingreq)),
		"PINGREQ",
	)
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", []string{"type"}, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&pb.Pingresp)),
		"PINGRESP",
	)
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", []string{"type"}, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&pb.Puback)),
		"PUBACK",
	)
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", []string{"type"}, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&pb.Pubcomp)),
		"PUBCOMP",
	)
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", []string{"type"}, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&pb.Publish)),
		"PUBLISH",
	)
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", []string{"type"}, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&pb.Pubrec)),
		"PUBREC",
	)
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", []string{"type"}, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&pb.Pubrel)),
		"PUBREL",
	)
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", []string{"type"}, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&pb.Suback)),
		"SUBACK",
	)
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", []string{"type"}, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&pb.Subscribe)),
		"SUBSCRIBE",
	)
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", []string{"type"}, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&pb.Unsuback)),
		"UNSUBACK",
	)
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", []string{"type"}, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&pb.Unsubscribe)),
		"UNSUBSCRIBE",
	)
}
func collectPacketsStatsCounter(metricName string, pc *server.PacketCount, m chan<- prometheus.Metric) {
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", []string{"type"}, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&pc.Connect)),
		"CONNECT",
	)
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", []string{"type"}, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&pc.Connack)),
		"CONNACK",
	)
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", []string{"type"}, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&pc.Disconnect)),
		"DISCONNECT",
	)
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", []string{"type"}, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&pc.Pingreq)),
		"PINGREQ",
	)
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", []string{"type"}, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&pc.Pingresp)),
		"PINGRESP",
	)
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", []string{"type"}, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&pc.Puback)),
		"PUBACK",
	)
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", []string{"type"}, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&pc.Pubcomp)),
		"PUBCOMP",
	)
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", []string{"type"}, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&pc.Publish)),
		"PUBLISH",
	)
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", []string{"type"}, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&pc.Pubrec)),
		"PUBREC",
	)
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", []string{"type"}, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&pc.Pubrel)),
		"PUBREL",
	)
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", []string{"type"}, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&pc.Suback)),
		"SUBACK",
	)
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", []string{"type"}, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&pc.Subscribe)),
		"SUBSCRIBE",
	)
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", []string{"type"}, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&pc.Unsuback)),
		"UNSUBACK",
	)
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", []string{"type"}, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&pc.Unsubscribe)),
		"UNSUBSCRIBE",
	)
}

func collectClientStats(c *server.ConnectionStats, m chan<- prometheus.Metric) {
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricPrefix+"clients_connected_total", "", nil, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&c.ConnectedTotal)),
	)
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricPrefix+"sessions_active_current", "", nil, nil),
		prometheus.GaugeValue,
		float64(atomic.LoadUint64(&c.ActiveCurrent)),
	)
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricPrefix+"sessions_inactive_current", "", nil, nil),
		prometheus.GaugeValue,
		float64(atomic.LoadUint64(&c.InactiveCurrent)),
	)
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricPrefix+"clients_disconnected_total", "", nil, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&c.DisconnectedTotal)),
	)
	//m <- prometheus.MustNewConstMetric(
	//	prometheus.NewDesc(metricPrefix+"sessions_expired_total", "", nil, nil),
	//	prometheus.CounterValue,
	//	float64(atomic.LoadUint64(&c.ExpiredTotal)),
	//)
}
func collectMessageStats(ms *server.MessageStats, m chan<- prometheus.Metric) {
	collectMessageStatsDropped(ms, m)
	collectMessageStatsQueued(ms, m)
	collectMessageStatsReceived(ms, m)
	collectMessageStatsSent(ms, m)
}

func collectQoSDropped(metricName string, qos string, stats *server.MessageQosStats, m chan<- prometheus.Metric) {
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", []string{"qos", "type"}, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&stats.DroppedTotal.Internal)), qos, "internal",
	)
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", []string{"qos", "type"}, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&stats.DroppedTotal.Expired)), qos, "expired",
	)

	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", []string{"qos", "type"}, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&stats.DroppedTotal.QueueFull)), qos, "queue_full",
	)

	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", []string{"qos", "type"}, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&stats.DroppedTotal.ExceedsMaxPacketSize)), qos, "exceeds_max_size",
	)
}

func collectMessageStatsDropped(ms *server.MessageStats, m chan<- prometheus.Metric) {
	metricName := metricPrefix + "messages_dropped_total"
	collectQoSDropped(metricName, "0", &ms.Qos0, m)
	collectQoSDropped(metricName, "1", &ms.Qos1, m)
	collectQoSDropped(metricName, "2", &ms.Qos2, m)
}

func collectMessageStatsQueued(ms *server.MessageStats, m chan<- prometheus.Metric) {
	metricName := "messages_queued_current"
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", nil, nil),
		prometheus.GaugeValue,
		float64(atomic.LoadUint64(&ms.QueuedCurrent)),
	)
}
func collectMessageStatsReceived(ms *server.MessageStats, m chan<- prometheus.Metric) {
	metricName := "messages_received_total"
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", []string{"qos"}, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&ms.Qos0.ReceivedTotal)), "0",
	)
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", []string{"qos"}, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&ms.Qos1.ReceivedTotal)), "1",
	)
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", []string{"qos"}, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&ms.Qos2.ReceivedTotal)), "2",
	)
}
func collectMessageStatsSent(ms *server.MessageStats, m chan<- prometheus.Metric) {
	metricName := "messages_sent_total"
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", []string{"qos"}, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&ms.Qos0.SentTotal)), "0",
	)
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", []string{"qos"}, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&ms.Qos1.SentTotal)), "1",
	)
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "", []string{"qos"}, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&ms.Qos2.SentTotal)), "2",
	)
}

func collectSubscriptionStats(s *subscription.Stats, m chan<- prometheus.Metric) {
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricPrefix+"subscriptions_total", "", nil, nil),
		prometheus.CounterValue,
		float64(atomic.LoadUint64(&s.SubscriptionsTotal)),
	)
	m <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricPrefix+"subscriptions_current", "", nil, nil),
		prometheus.GaugeValue,
		float64(atomic.LoadUint64(&s.SubscriptionsCurrent)),
	)
}
