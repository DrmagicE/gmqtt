package server

import (
	"sync"
	"sync/atomic"

	"github.com/DrmagicE/gmqtt/persistence/queue"
	"github.com/DrmagicE/gmqtt/persistence/subscription"
	"github.com/DrmagicE/gmqtt/pkg/packets"
)

type statsManager struct {
	subStatsReader subscription.StatsReader
	totalStats     *GlobalStats
	clientMu       sync.Mutex
	clientStats    map[string]*ClientStats
}

func (s *statsManager) getClientStats(clientID string) (stats *ClientStats) {
	if stats = s.clientStats[clientID]; stats == nil {
		subStats, _ := s.subStatsReader.GetClientStats(clientID)

		stats = &ClientStats{
			SubscriptionStats: subStats,
		}
		s.clientStats[clientID] = stats
	}
	return stats
}
func (s *statsManager) packetReceived(packet packets.Packet, clientID string) {
	s.totalStats.PacketStats.add(packet, true)
	s.clientMu.Lock()
	defer s.clientMu.Unlock()
	s.getClientStats(clientID).PacketStats.add(packet, true)
}
func (s *statsManager) packetSent(packet packets.Packet, clientID string) {
	s.totalStats.PacketStats.add(packet, false)
	s.clientMu.Lock()
	defer s.clientMu.Unlock()
	s.getClientStats(clientID).PacketStats.add(packet, false)
}
func (s *statsManager) clientPacketReceived(packet packets.Packet, clientID string) {
	s.clientMu.Lock()
	defer s.clientMu.Unlock()
	s.getClientStats(clientID).PacketStats.add(packet, true)
}
func (s *statsManager) clientPacketSent(packet packets.Packet, clientID string) {
	s.clientMu.Lock()
	defer s.clientMu.Unlock()
	s.getClientStats(clientID).PacketStats.add(packet, false)
}

func (s *statsManager) clientConnected(clientID string) {
	atomic.AddUint64(&s.totalStats.ConnectionStats.ConnectedTotal, 1)
}

func (s *statsManager) clientDisconnected(clientID string) {
	atomic.AddUint64(&s.totalStats.ConnectionStats.DisconnectedTotal, 1)
	s.sessionInActive()
}

func (s *statsManager) sessionActive(create bool) {
	if create {
		atomic.AddUint64(&s.totalStats.ConnectionStats.SessionCreatedTotal, 1)
	} else {
		atomic.AddUint64(&s.totalStats.ConnectionStats.InactiveCurrent, ^uint64(0))
	}
	atomic.AddUint64(&s.totalStats.ConnectionStats.ActiveCurrent, 1)
}

func (s *statsManager) sessionInActive() {
	atomic.AddUint64(&s.totalStats.ConnectionStats.ActiveCurrent, ^uint64(0))
	atomic.AddUint64(&s.totalStats.ConnectionStats.InactiveCurrent, 1)
}

func (s *statsManager) sessionTerminated(clientID string, reason SessionTerminatedReason) {
	var i *uint64
	switch reason {
	case NormalTermination:
		i = &s.totalStats.ConnectionStats.SessionTerminated.Normal
	case ExpiredTermination:
		i = &s.totalStats.ConnectionStats.SessionTerminated.Expired
	case TakenOverTermination:
		i = &s.totalStats.ConnectionStats.SessionTerminated.TakenOver
	}
	atomic.AddUint64(i, 1)
	atomic.AddUint64(&s.totalStats.ConnectionStats.InactiveCurrent, ^uint64(0))
	s.clientMu.Lock()
	defer s.clientMu.Unlock()
	delete(s.clientStats, clientID)
}

func (s *statsManager) messageDropped(qos uint8, clientID string, err error) {
	switch qos {
	case packets.Qos0:
		s.totalStats.MessageStats.Qos0.DroppedTotal.messageDropped(err)
		s.clientMu.Lock()
		defer s.clientMu.Unlock()
		s.getClientStats(clientID).MessageStats.Qos0.DroppedTotal.messageDropped(err)
	case packets.Qos1:
		s.totalStats.MessageStats.Qos1.DroppedTotal.messageDropped(err)
		s.clientMu.Lock()
		defer s.clientMu.Unlock()
		s.getClientStats(clientID).MessageStats.Qos1.DroppedTotal.messageDropped(err)
	case packets.Qos2:
		s.totalStats.MessageStats.Qos2.DroppedTotal.messageDropped(err)
		s.clientMu.Lock()
		defer s.clientMu.Unlock()
		s.getClientStats(clientID).MessageStats.Qos2.DroppedTotal.messageDropped(err)
	}
}
func (d *DroppedTotal) messageDropped(err error) {
	switch err {
	case queue.ErrDropExceedsMaxPacketSize:
		atomic.AddUint64(&d.ExceedsMaxPacketSize, 1)
	case queue.ErrDropQueueFull:
		atomic.AddUint64(&d.QueueFull, 1)
	case queue.ErrDropExpired:
		atomic.AddUint64(&d.Expired, 1)
	case queue.ErrDropExpiredInflight:
		atomic.AddUint64(&d.InflightExpired, 1)
	default:
		atomic.AddUint64(&d.Internal, 1)
	}
}

func (s *statsManager) messageReceived(qos uint8, clientID string) {
	switch qos {
	case packets.Qos0:
		atomic.AddUint64(&s.totalStats.MessageStats.Qos0.ReceivedTotal, 1)
		s.clientMu.Lock()
		defer s.clientMu.Unlock()
		atomic.AddUint64(&s.getClientStats(clientID).MessageStats.Qos0.ReceivedTotal, 1)
	case packets.Qos1:
		atomic.AddUint64(&s.totalStats.MessageStats.Qos1.ReceivedTotal, 1)
		s.clientMu.Lock()
		defer s.clientMu.Unlock()
		atomic.AddUint64(&s.getClientStats(clientID).MessageStats.Qos0.ReceivedTotal, 1)
	case packets.Qos2:
		atomic.AddUint64(&s.totalStats.MessageStats.Qos2.ReceivedTotal, 1)
		s.clientMu.Lock()
		defer s.clientMu.Unlock()
		atomic.AddUint64(&s.getClientStats(clientID).MessageStats.Qos0.ReceivedTotal, 1)
	}
}

func (s *statsManager) messageSent(qos uint8, clientID string) {
	switch qos {
	case packets.Qos0:
		atomic.AddUint64(&s.totalStats.MessageStats.Qos0.SentTotal, 1)
		s.clientMu.Lock()
		defer s.clientMu.Unlock()
		atomic.AddUint64(&s.getClientStats(clientID).MessageStats.Qos0.SentTotal, 1)
	case packets.Qos1:
		atomic.AddUint64(&s.totalStats.MessageStats.Qos1.SentTotal, 1)
		s.clientMu.Lock()
		defer s.clientMu.Unlock()
		atomic.AddUint64(&s.getClientStats(clientID).MessageStats.Qos0.SentTotal, 1)
	case packets.Qos2:
		atomic.AddUint64(&s.totalStats.MessageStats.Qos2.SentTotal, 1)
		s.clientMu.Lock()
		defer s.clientMu.Unlock()
		atomic.AddUint64(&s.getClientStats(clientID).MessageStats.Qos0.SentTotal, 1)
	}
}

// StatsReader interface provides the ability to access the statistics of the server
type StatsReader interface {
	// GetGlobalStats returns the server statistics.
	GetGlobalStats() GlobalStats
	// GetClientStats returns the client statistics for the given client id
	GetClientStats(clientID string) (sts ClientStats, exist bool)
}

// PacketStats represents  the statistics of MQTT Packet.
type PacketStats struct {
	BytesReceived PacketBytes
	ReceivedTotal PacketCount
	BytesSent     PacketBytes
	SentTotal     PacketCount
}

func (p *PacketStats) add(pt packets.Packet, receive bool) {
	b := packets.TotalBytes(pt)
	var bytes *PacketBytes
	var count *PacketCount
	if receive {
		bytes = &p.BytesReceived
		count = &p.ReceivedTotal
	} else {
		bytes = &p.BytesSent
		count = &p.SentTotal
	}
	switch pt.(type) {
	case *packets.Auth:
		atomic.AddUint64(&bytes.Auth, uint64(b))
		atomic.AddUint64(&count.Auth, 1)
	case *packets.Connect:
		atomic.AddUint64(&bytes.Connect, uint64(b))
		atomic.AddUint64(&count.Connect, 1)
	case *packets.Connack:
		atomic.AddUint64(&bytes.Connack, uint64(b))
		atomic.AddUint64(&count.Connack, 1)
	case *packets.Disconnect:
		atomic.AddUint64(&bytes.Disconnect, uint64(b))
		atomic.AddUint64(&count.Disconnect, 1)
	case *packets.Pingreq:
		atomic.AddUint64(&bytes.Pingreq, uint64(b))
		atomic.AddUint64(&count.Pingreq, 1)
	case *packets.Pingresp:
		atomic.AddUint64(&bytes.Pingresp, uint64(b))
		atomic.AddUint64(&count.Pingresp, 1)
	case *packets.Puback:
		atomic.AddUint64(&bytes.Puback, uint64(b))
		atomic.AddUint64(&count.Puback, 1)
	case *packets.Pubcomp:
		atomic.AddUint64(&bytes.Pubcomp, uint64(b))
		atomic.AddUint64(&count.Pubcomp, 1)
	case *packets.Publish:
		atomic.AddUint64(&bytes.Publish, uint64(b))
		atomic.AddUint64(&count.Publish, 1)
	case *packets.Pubrec:
		atomic.AddUint64(&bytes.Pubrec, uint64(b))
		atomic.AddUint64(&count.Pubrec, 1)
	case *packets.Pubrel:
		atomic.AddUint64(&bytes.Pubrel, uint64(b))
		atomic.AddUint64(&count.Pubrel, 1)
	case *packets.Suback:
		atomic.AddUint64(&bytes.Suback, uint64(b))
		atomic.AddUint64(&count.Suback, 1)
	case *packets.Subscribe:
		atomic.AddUint64(&bytes.Subscribe, uint64(b))
		atomic.AddUint64(&count.Subscribe, 1)
	case *packets.Unsuback:
		atomic.AddUint64(&bytes.Unsuback, uint64(b))
		atomic.AddUint64(&count.Unsuback, 1)
	case *packets.Unsubscribe:
		atomic.AddUint64(&bytes.Unsubscribe, uint64(b))
		atomic.AddUint64(&count.Unsubscribe, 1)
	}
	atomic.AddUint64(&bytes.Total, uint64(b))
	atomic.AddUint64(&count.Total, 1)

}
func (p *PacketStats) copy() *PacketStats {
	return &PacketStats{
		BytesReceived: p.BytesReceived.copy(),
		ReceivedTotal: p.ReceivedTotal.copy(),
		BytesSent:     p.BytesSent.copy(),
		SentTotal:     p.SentTotal.copy(),
	}
}

// PacketBytes represents total bytes of each in type have been received or sent.
type PacketBytes struct {
	Auth        uint64
	Connect     uint64
	Connack     uint64
	Disconnect  uint64
	Pingreq     uint64
	Pingresp    uint64
	Puback      uint64
	Pubcomp     uint64
	Publish     uint64
	Pubrec      uint64
	Pubrel      uint64
	Suback      uint64
	Subscribe   uint64
	Unsuback    uint64
	Unsubscribe uint64
	Total       uint64
}

func (p *PacketBytes) copy() PacketBytes {
	return PacketBytes{
		Connect:     atomic.LoadUint64(&p.Connect),
		Connack:     atomic.LoadUint64(&p.Connack),
		Disconnect:  atomic.LoadUint64(&p.Disconnect),
		Pingreq:     atomic.LoadUint64(&p.Pingreq),
		Pingresp:    atomic.LoadUint64(&p.Pingresp),
		Puback:      atomic.LoadUint64(&p.Puback),
		Pubcomp:     atomic.LoadUint64(&p.Pubcomp),
		Publish:     atomic.LoadUint64(&p.Publish),
		Pubrec:      atomic.LoadUint64(&p.Pubrec),
		Pubrel:      atomic.LoadUint64(&p.Pubrel),
		Suback:      atomic.LoadUint64(&p.Suback),
		Subscribe:   atomic.LoadUint64(&p.Subscribe),
		Unsuback:    atomic.LoadUint64(&p.Unsuback),
		Unsubscribe: atomic.LoadUint64(&p.Unsubscribe),
		Total:       atomic.LoadUint64(&p.Total),
	}
}

// PacketCount represents total number of each in type have been received or sent.
type PacketCount = PacketBytes

// ConnectionStats provides the statistics of client connections.
type ConnectionStats struct {
	ConnectedTotal      uint64
	DisconnectedTotal   uint64
	SessionCreatedTotal uint64
	SessionTerminated   struct {
		TakenOver uint64
		Expired   uint64
		Normal    uint64
	}
	// ActiveCurrent is the number of used active session.
	ActiveCurrent uint64
	// InactiveCurrent is the number of used inactive session.
	InactiveCurrent uint64
}

func (c *ConnectionStats) copy() *ConnectionStats {
	return &ConnectionStats{
		ConnectedTotal:      atomic.LoadUint64(&c.ConnectedTotal),
		DisconnectedTotal:   atomic.LoadUint64(&c.DisconnectedTotal),
		SessionCreatedTotal: atomic.LoadUint64(&c.SessionCreatedTotal),
		SessionTerminated: struct {
			TakenOver uint64
			Expired   uint64
			Normal    uint64
		}{
			TakenOver: atomic.LoadUint64(&c.SessionTerminated.TakenOver),
			Expired:   atomic.LoadUint64(&c.SessionTerminated.Expired),
			Normal:    atomic.LoadUint64(&c.SessionTerminated.Normal),
		},
		ActiveCurrent:   atomic.LoadUint64(&c.ActiveCurrent),
		InactiveCurrent: atomic.LoadUint64(&c.InactiveCurrent),
	}
}

type DroppedTotal struct {
	Internal             uint64
	ExceedsMaxPacketSize uint64
	QueueFull            uint64
	Expired              uint64
	InflightExpired      uint64
}

type MessageQosStats struct {
	DroppedTotal  DroppedTotal
	ReceivedTotal uint64
	SentTotal     uint64
}

func (m *MessageQosStats) GetDroppedTotal() uint64 {
	return m.DroppedTotal.Internal + m.DroppedTotal.Expired + m.DroppedTotal.ExceedsMaxPacketSize + m.DroppedTotal.QueueFull + m.DroppedTotal.InflightExpired
}

// MessageStats represents the statistics of PUBLISH in, separated by QOS.
type MessageStats struct {
	Qos0            MessageQosStats
	Qos1            MessageQosStats
	Qos2            MessageQosStats
	InflightCurrent uint64
	QueuedCurrent   uint64
}

func (m *MessageStats) GetDroppedTotal() uint64 {
	return m.Qos0.GetDroppedTotal() + m.Qos1.GetDroppedTotal() + m.Qos2.GetDroppedTotal()
}

func (s *statsManager) addInflight(clientID string, delta uint64) {
	s.clientMu.Lock()
	defer s.clientMu.Unlock()
	sts := s.getClientStats(clientID)
	atomic.AddUint64(&sts.MessageStats.InflightCurrent, delta)
	atomic.AddUint64(&s.totalStats.MessageStats.InflightCurrent, 1)
}
func (s *statsManager) decInflight(clientID string, delta uint64) {
	s.clientMu.Lock()
	defer s.clientMu.Unlock()
	sts := s.getClientStats(clientID)
	// Avoid the counter to be negative.
	// This could happen if the broker is start with persistence data loaded and send messages from the persistent queue.
	// Because the statistic data is not persistent, the init value is always 0.
	if atomic.LoadUint64(&sts.MessageStats.InflightCurrent) == 0 {
		return
	}
	atomic.AddUint64(&sts.MessageStats.InflightCurrent, ^uint64(delta-1))
	atomic.AddUint64(&s.totalStats.MessageStats.InflightCurrent, ^uint64(delta-1))
}

func (s *statsManager) addQueueLen(clientID string, delta uint64) {
	s.clientMu.Lock()
	defer s.clientMu.Unlock()
	sts := s.getClientStats(clientID)
	atomic.AddUint64(&sts.MessageStats.QueuedCurrent, delta)
	atomic.AddUint64(&s.totalStats.MessageStats.QueuedCurrent, delta)
}
func (s *statsManager) decQueueLen(clientID string, delta uint64) {
	s.clientMu.Lock()
	defer s.clientMu.Unlock()
	sts := s.getClientStats(clientID)
	// Avoid the counter to be negative.
	// This could happen if the broker is start with persistence data loaded and send messages from the persistent queue.
	// Because the statistic data is not persistent, the init value is always 0.
	if atomic.LoadUint64(&sts.MessageStats.QueuedCurrent) == 0 {
		return
	}
	atomic.AddUint64(&sts.MessageStats.QueuedCurrent, ^uint64(delta-1))
	atomic.AddUint64(&s.totalStats.MessageStats.QueuedCurrent, ^uint64(delta-1))
}

func (m *MessageStats) copy() *MessageStats {
	return &MessageStats{
		Qos0: MessageQosStats{
			DroppedTotal: DroppedTotal{
				Internal:             atomic.LoadUint64(&m.Qos0.DroppedTotal.Internal),
				ExceedsMaxPacketSize: atomic.LoadUint64(&m.Qos0.DroppedTotal.ExceedsMaxPacketSize),
				QueueFull:            atomic.LoadUint64(&m.Qos0.DroppedTotal.QueueFull),
				Expired:              atomic.LoadUint64(&m.Qos0.DroppedTotal.Expired),
				InflightExpired:      atomic.LoadUint64(&m.Qos0.DroppedTotal.InflightExpired),
			},
			ReceivedTotal: atomic.LoadUint64(&m.Qos0.ReceivedTotal),
			SentTotal:     atomic.LoadUint64(&m.Qos0.SentTotal),
		},
		Qos1: MessageQosStats{
			DroppedTotal: DroppedTotal{
				Internal:             atomic.LoadUint64(&m.Qos1.DroppedTotal.Internal),
				ExceedsMaxPacketSize: atomic.LoadUint64(&m.Qos1.DroppedTotal.ExceedsMaxPacketSize),
				QueueFull:            atomic.LoadUint64(&m.Qos1.DroppedTotal.QueueFull),
				Expired:              atomic.LoadUint64(&m.Qos1.DroppedTotal.Expired),
				InflightExpired:      atomic.LoadUint64(&m.Qos1.DroppedTotal.InflightExpired),
			},
			ReceivedTotal: atomic.LoadUint64(&m.Qos1.ReceivedTotal),
			SentTotal:     atomic.LoadUint64(&m.Qos1.SentTotal),
		},
		Qos2: MessageQosStats{
			DroppedTotal: DroppedTotal{
				Internal:             atomic.LoadUint64(&m.Qos2.DroppedTotal.Internal),
				ExceedsMaxPacketSize: atomic.LoadUint64(&m.Qos2.DroppedTotal.ExceedsMaxPacketSize),
				QueueFull:            atomic.LoadUint64(&m.Qos2.DroppedTotal.QueueFull),
				Expired:              atomic.LoadUint64(&m.Qos2.DroppedTotal.Expired),
				InflightExpired:      atomic.LoadUint64(&m.Qos2.DroppedTotal.InflightExpired),
			},
			ReceivedTotal: atomic.LoadUint64(&m.Qos2.ReceivedTotal),
			SentTotal:     atomic.LoadUint64(&m.Qos2.SentTotal),
		},
		InflightCurrent: atomic.LoadUint64(&m.InflightCurrent),
		QueuedCurrent:   atomic.LoadUint64(&m.QueuedCurrent),
	}
}

// GlobalStats is the collection of global statistics.
type GlobalStats struct {
	ConnectionStats   ConnectionStats
	PacketStats       PacketStats
	MessageStats      MessageStats
	SubscriptionStats subscription.Stats
}

// ClientStats is the statistic information of one client.
type ClientStats struct {
	PacketStats       PacketStats
	MessageStats      MessageStats
	SubscriptionStats subscription.Stats
}

func (c ClientStats) GetDroppedTotal() uint64 {
	return c.MessageStats.Qos0.GetDroppedTotal() + c.MessageStats.Qos1.GetDroppedTotal() + c.MessageStats.Qos2.GetDroppedTotal()
}

// GetGlobalStats returns the GlobalStats
func (s *statsManager) GetGlobalStats() GlobalStats {
	return GlobalStats{
		PacketStats:       *s.totalStats.PacketStats.copy(),
		ConnectionStats:   *s.totalStats.ConnectionStats.copy(),
		MessageStats:      *s.totalStats.MessageStats.copy(),
		SubscriptionStats: s.subStatsReader.GetStats(),
	}
}

// GetClientStats returns the client statistic information for given client id.
func (s *statsManager) GetClientStats(clientID string) (ClientStats, bool) {
	s.clientMu.Lock()
	defer s.clientMu.Unlock()
	if stats := s.clientStats[clientID]; stats == nil {
		return ClientStats{}, false
	} else {
		s, _ := s.subStatsReader.GetClientStats(clientID)
		return ClientStats{
			PacketStats:       *stats.PacketStats.copy(),
			MessageStats:      *stats.MessageStats.copy(),
			SubscriptionStats: s,
		}, true
	}

}

func newStatsManager(subStatsReader subscription.StatsReader) *statsManager {
	return &statsManager{
		subStatsReader: subStatsReader,
		totalStats:     &GlobalStats{},
		clientMu:       sync.Mutex{},
		clientStats:    make(map[string]*ClientStats),
	}
}
