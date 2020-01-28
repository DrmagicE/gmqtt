package management

import (
	"container/list"
	"errors"
	"sync"
	"time"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/DrmagicE/gmqtt/subscription"
)

const (
	Online  = "online"
	Offline = "offline"
)

type monitor struct {
	clientMu       sync.Mutex
	clientList     *quickList
	subMu          sync.Mutex
	subscriptions  map[string]*quickList // key by clientID
	config         gmqtt.Config
	subStatsReader subscription.StatsReader
}

// newMonitor
func newMonitor(subStatsReader subscription.StatsReader) *monitor {
	return &monitor{
		clientList:     newQuickList(),
		subscriptions:  make(map[string]*quickList),
		subStatsReader: subStatsReader,
	}
}
func statusText(client gmqtt.Client) string {
	if client.IsConnected() {
		return Online
	} else {
		return Offline
	}
}

// addSubscription
func (m *monitor) addSubscription(clientID string, topic packets.Topic) {
	m.subMu.Lock()
	defer m.subMu.Unlock()
	subInfo := &SubscriptionInfo{
		ClientID: clientID,
		Qos:      topic.Qos,
		Name:     topic.Name,
		At:       time.Now(),
	}
	if _, ok := m.subscriptions[clientID]; !ok {
		m.subscriptions[clientID] = newQuickList()
	}
	m.subscriptions[clientID].set(subInfo.Name, subInfo)
}

// deleteSubscription
func (m *monitor) deleteSubscription(clientID string, topicName string) {
	m.subMu.Lock()
	defer m.subMu.Unlock()
	if _, ok := m.subscriptions[clientID]; ok {
		m.subscriptions[clientID].remove(topicName)
	}
}

// deleteClientSubscriptions
func (m *monitor) deleteClientSubscriptions(clientID string) {
	m.subMu.Lock()
	defer m.subMu.Unlock()
	delete(m.subscriptions, clientID)
}

// GetClientSubscriptions
func (m *monitor) GetClientSubscriptions(clientID string, offset, n int) ([]*SubscriptionInfo, error) {
	m.subMu.Lock()
	defer m.subMu.Unlock()
	rs := make([]*SubscriptionInfo, 0)
	var err error
	if _, ok := m.subscriptions[clientID]; ok {
		fn := func(elem *list.Element) {
			rs = append(rs, elem.Value.(*SubscriptionInfo))
		}
		err = m.subscriptions[clientID].iterate(fn, offset, n)
	}
	return rs, err
}

// ClientInfo represents the client information
type ClientInfo struct {
	ClientID       string    `json:"client_id"`
	Username       string    `json:"username"`
	Password       string    `json:"password"`
	KeepAlive      uint16    `json:"keep_alive"`
	CleanSession   bool      `json:"clean_session"`
	WillFlag       bool      `json:"will_flag"`
	WillRetain     bool      `json:"will_retain"`
	WillQos        uint8     `json:"will_qos"`
	WillTopic      string    `json:"will_topic"`
	WillPayload    string    `json:"will_payload"`
	RemoteAddr     string    `json:"remote_addr"`
	LocalAddr      string    `json:"local_addr"`
	ConnectedAt    time.Time `json:"connected_at"`
	DisconnectedAt time.Time `json:"disconnected_at"`
}

// SessionInfo represents the session information
type SessionInfo struct {
	ClientID              string    `json:"client_id"`
	Status                string    `json:"status"`
	CleanSession          bool      `json:"clean_session"`
	Subscriptions         uint64    `json:"subscriptions"`
	MaxInflight           int       `json:"max_inflight"`
	InflightLen           uint64    `json:"inflight_len"`
	MaxMsgQueue           int       `json:"max_msg_queue"`
	MsgQueueLen           uint64    `json:"msg_queue_len"`
	MaxAwaitRel           int       `json:"max_await_rel"`
	AwaitRelLen           uint64    `json:"await_rel_len"`
	Qos0MsgDroppedTotal   uint64    `json:"qos0_msg_dropped_total"`
	Qos1MsgDroppedTotal   uint64    `json:"qos1_msg_dropped_total"`
	Qos2MsgDroppedTotal   uint64    `json:"qos2_msg_dropped_total"`
	Qos0MsgDeliveredTotal uint64    `json:"qos0_msg_delivered_total"`
	Qos1MsgDeliveredTotal uint64    `json:"qos1_msg_delivered_total"`
	Qos2MsgDeliveredTotal uint64    `json:"qos2_msg_delivered_total"`
	ConnectedAt           time.Time `json:"connected_at"`
	DisconnectedAt        time.Time `json:"disconnected_at"`
}

// SubscriptionInfo represents the subscription information
type SubscriptionInfo struct {
	ClientID string    `json:"client_id"`
	Qos      uint8     `json:"qos"`
	Name     string    `json:"name"`
	At       time.Time `json:"at"`
}

var ErrNotFound = errors.New("not found")

type quickList struct {
	index map[string]*list.Element
	rows  *list.List
}

func newQuickList() *quickList {
	return &quickList{
		index: make(map[string]*list.Element),
		rows:  list.New(),
	}
}
func (q *quickList) set(id string, value interface{}) {
	if e, ok := q.index[id]; ok {
		e.Value = value
	} else {
		elem := q.rows.PushBack(value)
		q.index[id] = elem
	}
}
func (q *quickList) remove(id string) *list.Element {
	elem := q.index[id]
	if elem != nil {
		q.rows.Remove(elem)
	}
	delete(q.index, id)
	return elem
}
func (q *quickList) getByID(id string) (*list.Element, error) {
	if i, ok := q.index[id]; ok {
		return i, nil
	}
	return nil, ErrNotFound
}
func (q *quickList) iterate(fn func(elem *list.Element), offset, n int) error {
	if offset < 0 || n < 0 {
		return errors.New("invalid offset or n")
	}
	if q.rows.Len() <= offset {
		return errors.New("invalid offset")
	}
	var i int
	for e := q.rows.Front(); e != nil; e = e.Next() {
		if i >= offset && i < offset+n {
			fn(e)
		}
		if i == offset+n {
			break
		}
		i++
	}
	return nil
}

// addClient
func (m *monitor) addClient(client gmqtt.Client) {
	m.clientMu.Lock()
	m.clientList.set(client.OptionsReader().ClientID(), client)
	m.clientMu.Unlock()
}

// deleteClient
func (m *monitor) deleteClient(clientID string) {
	m.clientMu.Lock()
	m.clientList.remove(clientID)
	m.clientMu.Unlock()
}

// GetClientByID
func (m *monitor) GetClientByID(clientID string) (*ClientInfo, error) {
	m.clientMu.Lock()
	client, err := m.getClientByID(clientID)
	m.clientMu.Unlock()
	if err != nil {
		return nil, err
	}
	return newClientInfo(client), err
}
func newClientInfo(client gmqtt.Client) *ClientInfo {
	optsReader := client.OptionsReader()
	rs := &ClientInfo{
		ClientID:       optsReader.ClientID(),
		Username:       optsReader.Username(),
		Password:       optsReader.Password(),
		KeepAlive:      optsReader.KeepAlive(),
		CleanSession:   optsReader.CleanSession(),
		WillFlag:       optsReader.WillFlag(),
		WillRetain:     optsReader.WillRetain(),
		WillQos:        optsReader.WillQos(),
		WillTopic:      optsReader.WillTopic(),
		WillPayload:    string(optsReader.WillPayload()),
		RemoteAddr:     optsReader.RemoteAddr().String(),
		LocalAddr:      optsReader.LocalAddr().String(),
		ConnectedAt:    client.ConnectedAt(),
		DisconnectedAt: client.DisconnectedAt(),
	}
	return rs
}
func (m *monitor) newSessionInfo(client gmqtt.Client, c gmqtt.Config) *SessionInfo {
	optsReader := client.OptionsReader()
	stats := client.GetSessionStatsManager().GetStats()
	subStats, _ := m.subStatsReader.GetClientStats(optsReader.ClientID())
	rs := &SessionInfo{
		ClientID:              optsReader.ClientID(),
		Status:                statusText(client),
		CleanSession:          optsReader.CleanSession(),
		Subscriptions:         subStats.SubscriptionsCurrent,
		MaxInflight:           c.MaxInflight,
		InflightLen:           stats.InflightCurrent,
		MaxMsgQueue:           c.MaxMsgQueue,
		MsgQueueLen:           stats.MessageStats.QueuedCurrent,
		MaxAwaitRel:           c.MaxAwaitRel,
		AwaitRelLen:           stats.AwaitRelCurrent,
		Qos0MsgDroppedTotal:   stats.Qos0.DroppedTotal,
		Qos1MsgDroppedTotal:   stats.Qos1.DroppedTotal,
		Qos2MsgDroppedTotal:   stats.Qos2.DroppedTotal,
		Qos0MsgDeliveredTotal: stats.Qos0.SentTotal,
		Qos1MsgDeliveredTotal: stats.Qos1.SentTotal,
		Qos2MsgDeliveredTotal: stats.Qos2.SentTotal,
		ConnectedAt:           client.ConnectedAt(),
		DisconnectedAt:        client.DisconnectedAt(),
	}
	return rs
}

func (m *monitor) getClientByID(clientID string) (gmqtt.Client, error) {
	if i, err := m.clientList.getByID(clientID); i != nil {
		return i.Value.(gmqtt.Client), nil
	} else {
		return nil, err
	}
}

// GetClients
func (m *monitor) GetClients(offset, n int) ([]*ClientInfo, error) {
	rs := make([]*ClientInfo, 0)
	fn := func(elem *list.Element) {
		rs = append(rs, newClientInfo(elem.Value.(gmqtt.Client)))
	}
	m.clientMu.Lock()
	m.clientList.iterate(fn, offset, n)
	m.clientMu.Unlock()
	return rs, nil
}

// GetSessionByID
func (m *monitor) GetSessionByID(clientID string) (*SessionInfo, error) {
	m.clientMu.Lock()
	client, err := m.getClientByID(clientID)
	m.clientMu.Unlock()
	if err != nil {
		return nil, err
	}
	return m.newSessionInfo(client, m.config), err
}

// GetSessions
func (m *monitor) GetSessions(offset, n int) ([]*SessionInfo, error) {
	rs := make([]*SessionInfo, 0)
	fn := func(elem *list.Element) {
		rs = append(rs, m.newSessionInfo(elem.Value.(gmqtt.Client), m.config))
	}
	m.clientMu.Lock()
	m.clientList.iterate(fn, offset, n)
	m.clientMu.Unlock()
	return rs, nil
}
