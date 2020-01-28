package gmqtt

import (
	"sync/atomic"

	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/DrmagicE/gmqtt/subscription"
)

// StatsManager interface provides the ability to access the statistics of the server
type StatsManager interface {
	packetStatsManager
	clientStatsManager
	messageStatsManager
	// GetStats return the server statistics
	GetStats() *ServerStats
}

// SessionStatsManager interface provides the ability to access the statistics of the session
type SessionStatsManager interface {
	messageStatsManager
	addInflightCurrent(delta uint64)
	decInflightCurrent(delta uint64)
	addAwaitCurrent(delta uint64)
	decAwaitCurrent(delta uint64)
	// GetStats return the session statistics
	GetStats() *SessionStats
}

// SessionStats the collection of statistics of each session.
type SessionStats struct {
	// InflightCurrent, the current length of the inflight queue.
	InflightCurrent uint64
	// AwaitRelCurrent, the current length of the awaitRel queue.
	AwaitRelCurrent uint64

	MessageStats
}

func (s *SessionStats) copy() *SessionStats {
	return &SessionStats{
		InflightCurrent: atomic.LoadUint64(&s.InflightCurrent),
		AwaitRelCurrent: atomic.LoadUint64(&s.AwaitRelCurrent),
		MessageStats:    *s.MessageStats.copy(),
	}
}

type packetStatsManager interface {
	packetReceived(packet packets.Packet)
	packetSent(packet packets.Packet)
}

type clientStatsManager interface {
	addClientConnected()
	addClientDisconnected()
	addSessionActive()
	decSessionActive()
	addSessionInactive()
	decSessionInactive()
	addSessionExpired()
}
type messageStatsManager interface {
	messageDropped(qos uint8)
	messageReceived(qos uint8)
	messageSent(qos uint8)
	messageEnqueue(delta uint64)
	messageDequeue(delta uint64)
}

// PacketStats represents  the statistics of MQTT Packet.
type PacketStats struct {
	BytesReceived *PacketBytes
	ReceivedTotal *PacketCount
	BytesSent     *PacketBytes
	SentTotal     *PacketCount
}

func (p *PacketStats) add(pt packets.Packet, receive bool) {
	b := packets.TotalBytes(pt)
	var bytes *PacketBytes
	var count *PacketCount
	if receive {
		bytes = p.BytesReceived
		count = p.ReceivedTotal
	} else {
		bytes = p.BytesSent
		count = p.SentTotal
	}
	switch pt.(type) {
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
}
func (p *PacketStats) copy() *PacketStats {
	return &PacketStats{
		BytesReceived: p.BytesReceived.copy(),
		ReceivedTotal: p.ReceivedTotal.copy(),
		BytesSent:     p.BytesSent.copy(),
		SentTotal:     p.SentTotal.copy(),
	}
}

// PacketBytes represents total bytes of each packet type have been received or sent.
type PacketBytes struct {
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
}

func (p *PacketBytes) copy() *PacketBytes {
	return &PacketBytes{
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
	}
}

// PacketCount represents total number of each packet type have been received or sent.
type PacketCount struct {
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
}

func (p *PacketCount) copy() *PacketCount {
	return &PacketCount{
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
	}
}

// ClientStats provides the statistics of client connections.
type ClientStats struct {
	ConnectedTotal    uint64
	DisconnectedTotal uint64
	// ActiveCurrent is the number of current active session.
	ActiveCurrent uint64
	// InactiveCurrent is the number of current inactive session.
	InactiveCurrent uint64
	// ExpiredTotal is the number of expired session.
	ExpiredTotal uint64
}

func (c *ClientStats) copy() *ClientStats {
	return &ClientStats{
		ConnectedTotal:    atomic.LoadUint64(&c.ConnectedTotal),
		DisconnectedTotal: atomic.LoadUint64(&c.DisconnectedTotal),
		ActiveCurrent:     atomic.LoadUint64(&c.ActiveCurrent),
		InactiveCurrent:   atomic.LoadUint64(&c.InactiveCurrent),
		ExpiredTotal:      atomic.LoadUint64(&c.ExpiredTotal),
	}
}

// MessageStats represents the statistics of PUBLISH packet, separated by QOS.
type MessageStats struct {
	Qos0 struct {
		DroppedTotal  uint64
		ReceivedTotal uint64
		SentTotal     uint64
	}
	Qos1 struct {
		DroppedTotal  uint64
		ReceivedTotal uint64
		SentTotal     uint64
	}
	Qos2 struct {
		DroppedTotal  uint64
		ReceivedTotal uint64
		SentTotal     uint64
	}
	QueuedCurrent uint64
}

func (m *MessageStats) copy() *MessageStats {
	return &MessageStats{
		Qos0: struct {
			DroppedTotal  uint64
			ReceivedTotal uint64
			SentTotal     uint64
		}{
			DroppedTotal:  atomic.LoadUint64(&m.Qos0.DroppedTotal),
			ReceivedTotal: atomic.LoadUint64(&m.Qos0.ReceivedTotal),
			SentTotal:     atomic.LoadUint64(&m.Qos0.SentTotal),
		},
		Qos1: struct {
			DroppedTotal  uint64
			ReceivedTotal uint64
			SentTotal     uint64
		}{
			DroppedTotal:  atomic.LoadUint64(&m.Qos1.DroppedTotal),
			ReceivedTotal: atomic.LoadUint64(&m.Qos1.ReceivedTotal),
			SentTotal:     atomic.LoadUint64(&m.Qos1.SentTotal),
		},
		Qos2: struct {
			DroppedTotal  uint64
			ReceivedTotal uint64
			SentTotal     uint64
		}{
			DroppedTotal:  atomic.LoadUint64(&m.Qos2.DroppedTotal),
			ReceivedTotal: atomic.LoadUint64(&m.Qos2.ReceivedTotal),
			SentTotal:     atomic.LoadUint64(&m.Qos2.SentTotal),
		},
		QueuedCurrent: atomic.LoadUint64(&m.QueuedCurrent),
	}
}

// ServerStats is the collection of global  statistics.
type ServerStats struct {
	PacketStats       *PacketStats
	ClientStats       *ClientStats
	MessageStats      *MessageStats
	SubscriptionStats *subscription.Stats
}

type statsManager struct {
	subStatsReader    subscription.StatsReader
	packetStats       PacketStats
	clientStats       ClientStats
	messageStats      MessageStats
	subscriptionStats subscription.Stats
}

func (s *statsManager) GetStats() *ServerStats {
	substats := s.subStatsReader.GetStats()
	return &ServerStats{
		PacketStats:       s.packetStats.copy(),
		ClientStats:       s.clientStats.copy(),
		MessageStats:      s.messageStats.copy(),
		SubscriptionStats: &substats,
	}
}
func (s *statsManager) packetReceived(p packets.Packet) {
	s.packetStats.add(p, true)
}
func (s *statsManager) packetSent(p packets.Packet) {
	s.packetStats.add(p, false)
}

func (s *statsManager) addSubscriptionTotal(delta int) {
	atomic.AddUint64(&s.subscriptionStats.SubscriptionsTotal, uint64(delta))
}
func (s *statsManager) addSubscriptionCurrent(delta int) {
	if delta >= 0 {
		atomic.AddUint64(&s.subscriptionStats.SubscriptionsCurrent, uint64(delta))
	} else {
		atomic.AddUint64(&s.subscriptionStats.SubscriptionsCurrent, ^uint64(-delta-1))
	}
}

func (s *statsManager) addClientConnected() {
	atomic.AddUint64(&s.clientStats.ConnectedTotal, 1)
}
func (s *statsManager) addSessionActive() {
	atomic.AddUint64(&s.clientStats.ActiveCurrent, 1)
}
func (s *statsManager) decSessionActive() {
	atomic.AddUint64(&s.clientStats.ActiveCurrent, ^uint64(0))
}
func (s *statsManager) addSessionInactive() {
	atomic.AddUint64(&s.clientStats.InactiveCurrent, 1)
}
func (s *statsManager) decSessionInactive() {
	atomic.AddUint64(&s.clientStats.InactiveCurrent, ^uint64(0))
}
func (s *statsManager) addClientDisconnected() {
	atomic.AddUint64(&s.clientStats.DisconnectedTotal, 1)
}
func (s *statsManager) addSessionExpired() {
	atomic.AddUint64(&s.clientStats.ExpiredTotal, 1)
}

func (s *statsManager) messageDropped(qos uint8) {
	switch qos {
	case packets.QOS_0:
		atomic.AddUint64(&s.messageStats.Qos0.DroppedTotal, 1)
	case packets.QOS_1:
		atomic.AddUint64(&s.messageStats.Qos1.DroppedTotal, 1)
	case packets.QOS_2:
		atomic.AddUint64(&s.messageStats.Qos2.DroppedTotal, 1)
	}
}
func (s *statsManager) messageReceived(qos uint8) {
	switch qos {
	case packets.QOS_0:
		atomic.AddUint64(&s.messageStats.Qos0.ReceivedTotal, 1)
	case packets.QOS_1:
		atomic.AddUint64(&s.messageStats.Qos1.ReceivedTotal, 1)
	case packets.QOS_2:
		atomic.AddUint64(&s.messageStats.Qos2.ReceivedTotal, 1)
	}
}
func (s *statsManager) messageSent(qos uint8) {
	switch qos {
	case packets.QOS_0:
		atomic.AddUint64(&s.messageStats.Qos0.SentTotal, 1)
	case packets.QOS_1:
		atomic.AddUint64(&s.messageStats.Qos1.SentTotal, 1)
	case packets.QOS_2:
		atomic.AddUint64(&s.messageStats.Qos2.SentTotal, 1)
	}
}
func (s *statsManager) messageEnqueue(delta uint64) {
	atomic.AddUint64(&s.messageStats.QueuedCurrent, delta)
}
func (s *statsManager) messageDequeue(delta uint64) {
	atomic.AddUint64(&s.messageStats.QueuedCurrent, ^uint64(delta-1))
}

func newStatsManager(subStatsReader subscription.StatsReader) *statsManager {
	return &statsManager{
		subStatsReader: subStatsReader,
		packetStats: PacketStats{
			BytesReceived: &PacketBytes{},
			ReceivedTotal: &PacketCount{},
			BytesSent:     &PacketBytes{},
			SentTotal:     &PacketCount{},
		},
		clientStats:       ClientStats{},
		messageStats:      MessageStats{},
		subscriptionStats: subscription.Stats{},
	}
}

type sessionStatsManager struct {
	SessionStats
}

func (s *sessionStatsManager) messageDropped(qos uint8) {
	switch qos {
	case packets.QOS_0:
		atomic.AddUint64(&s.Qos0.DroppedTotal, 1)
	case packets.QOS_1:
		atomic.AddUint64(&s.Qos1.DroppedTotal, 1)
	case packets.QOS_2:
		atomic.AddUint64(&s.Qos2.DroppedTotal, 1)
	}
}
func (s *sessionStatsManager) messageReceived(qos uint8) {
	switch qos {
	case packets.QOS_0:
		atomic.AddUint64(&s.Qos0.ReceivedTotal, 1)
	case packets.QOS_1:
		atomic.AddUint64(&s.Qos1.ReceivedTotal, 1)
	case packets.QOS_2:
		atomic.AddUint64(&s.Qos2.ReceivedTotal, 1)
	}
}
func (s *sessionStatsManager) messageSent(qos uint8) {
	switch qos {
	case packets.QOS_0:
		atomic.AddUint64(&s.Qos0.SentTotal, 1)
	case packets.QOS_1:
		atomic.AddUint64(&s.Qos1.SentTotal, 1)
	case packets.QOS_2:
		atomic.AddUint64(&s.Qos2.SentTotal, 1)
	}
}
func (s *sessionStatsManager) messageEnqueue(delta uint64) {
	atomic.AddUint64(&s.QueuedCurrent, delta)
}
func (s *sessionStatsManager) messageDequeue(delta uint64) {
	atomic.AddUint64(&s.QueuedCurrent, ^uint64(delta-1))
}
func (s *sessionStatsManager) addInflightCurrent(delta uint64) {
	atomic.AddUint64(&s.InflightCurrent, delta)
}

func (s *sessionStatsManager) decInflightCurrent(delta uint64) {
	atomic.AddUint64(&s.InflightCurrent, ^uint64(delta-1))
}

func (s *sessionStatsManager) addAwaitCurrent(delta uint64) {
	atomic.AddUint64(&s.AwaitRelCurrent, delta)
}

func (s *sessionStatsManager) decAwaitCurrent(delta uint64) {
	atomic.AddUint64(&s.AwaitRelCurrent, ^uint64(delta-1))
}

func (s *sessionStatsManager) GetStats() *SessionStats {
	return s.copy()
}

func newSessionStatsManager() *sessionStatsManager {
	return &sessionStatsManager{}
}
