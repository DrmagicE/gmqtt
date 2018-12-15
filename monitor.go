package gmqtt

import (
	"sort"
	"sync"
	"time"
)

const STATUS_ONLINE = "online"
const STATUS_OFFLINE = "offline"

// MonitorRepository is an interface which can be used to provide a persistence mechanics for the monitor data
type MonitorRepository interface {
	Open() error
	Close() error
	//PutClient puts a ClientInfo into the repository when the client connects
	PutClient(info ClientInfo)
	//GetClient returns the ClientInfo for the given clientId
	GetClient(clientId string) (ClientInfo, bool)
	//Clients returns ClientList which is the list for all connected clients, this method should be idempotency
	Clients() ClientList
	//DelClient deletes the ClientInfo from repository
	DelClient(clientId string)
	//PutSession puts a SessionInfo into monitor repository when the client is connects
	PutSession(info SessionInfo)
	//GetSession returns the SessionInfo for the given clientId
	GetSession(clientId string) (SessionInfo, bool)
	//Sessions returns SessionList which is the list for all sessions including online sessions and offline sessions, this method should be idempotency
	Sessions() SessionList
	//DelSession deletes the SessionInfo from repository
	DelSession(clientId string)
	//ClientSubscriptions returns the SubscriptionList for given clientId, this method should be idempotency
	ClientSubscriptions(clientId string) SubscriptionList
	//DelClientSubscriptions deletes the subscription info for given clientId from the repository
	DelClientSubscriptions(clientId string)
	//PutSubscription puts the SubscriptionsInfo into the repository when a new subscription is made
	PutSubscription(info SubscriptionsInfo)
	//DelSubscription deletes the topic for given clientId from repository
	DelSubscription(clientId string, topicName string)
	//Subscriptions returns all  subscriptions of the server
	Subscriptions() SubscriptionList
}

// Monitor is used internally to save and get monitor data
type Monitor struct {
	sync.Mutex
	Repository MonitorRepository
}

// MonitorStore implements the MonitorRepository interface to provide an in-memory monitor repository
type MonitorStore struct {
	clients       map[string]ClientInfo
	sessions      map[string]SessionInfo
	subscriptions map[string]map[string]SubscriptionsInfo //[clientId][topicName]
}

// register puts the session and client info into repository when a new client connects
func (m *Monitor) register(client *Client, sessionReuse bool) {
	m.Lock()
	defer m.Unlock()
	clientId := client.opts.ClientId
	username := client.opts.Username
	cinfo := ClientInfo{
		ClientId:     clientId,
		Username:     username,
		RemoteAddr:   client.rwc.RemoteAddr().String(),
		CleanSession: client.opts.CleanSession,
		KeepAlive:    client.opts.KeepAlive,
		ConnectedAt:  time.Now(),
	}
	m.Repository.PutClient(cinfo)
	if sessionReuse {
		client.session.inflightMu.Lock()
		client.session.msgQueueMu.Lock()
		inflightLen := client.session.inflight.Len()
		msgQueueLen := client.session.msgQueue.Len()
		client.session.inflightMu.Unlock()
		client.session.msgQueueMu.Unlock()
		/*		sub := m.Repository.ClientSubscriptions(clientId)
				m.Repository.PutClientSubscriptions(clientId ,sub)*/
		if c, ok := m.Repository.GetSession(clientId); ok {
			c.ConnectedAt = time.Now()
			c.Status = STATUS_ONLINE
			c.InflightLen = inflightLen
			c.MsgQueueLen = msgQueueLen
			m.Repository.PutSession(c)
			return
		}
	}
	m.Repository.PutSession(SessionInfo{
		ClientId:        clientId,
		Status:          STATUS_ONLINE,
		RemoteAddr:      client.rwc.RemoteAddr().String(),
		CleanSession:    client.opts.CleanSession,
		Subscriptions:   0,
		MaxInflight:     client.session.maxInflightMessages,
		InflightLen:     0,
		MaxMsgQueue:     client.session.maxQueueMessages,
		MsgQueueDropped: 0,
		MsgQueueLen:     0,
		ConnectedAt:     time.Now(),
	})

}

// unRegister deletes the session(if cleanSession = true) and client info from repository when a client disconnects
func (m *Monitor) unRegister(clientId string, cleanSession bool) {
	m.Lock()
	defer m.Unlock()
	m.Repository.DelClient(clientId)
	if cleanSession {
		m.Repository.DelSession(clientId)
		m.Repository.DelClientSubscriptions(clientId)
	} else {
		if s, ok := m.Repository.GetSession(clientId); ok {
			s.OfflineAt = time.Now()
			s.Status = STATUS_OFFLINE
			m.Repository.PutSession(s)
		}
	}
}

// subscribe puts the subscription info into repository
func (m *Monitor) subscribe(info SubscriptionsInfo) {
	m.Lock()
	defer m.Unlock()
	m.Repository.PutSubscription(info)
	list := m.Repository.ClientSubscriptions(info.ClientId)
	if s, ok := m.Repository.GetSession(info.ClientId); ok {
		s.Subscriptions = len(list)
		m.Repository.PutSession(s)
	}

}

// unSubscribe deletes the subscription info from repository
func (m *Monitor) unSubscribe(clientId string, topicName string) {
	m.Lock()
	defer m.Unlock()
	m.Repository.DelSubscription(clientId, topicName)
	list := m.Repository.ClientSubscriptions(clientId)
	if s, ok := m.Repository.GetSession(clientId); ok {
		s.Subscriptions = len(list)
		m.Repository.PutSession(s)
	}
}
func (m *Monitor) addInflight(clientId string) {
	m.Lock()
	defer m.Unlock()
	if s, ok := m.Repository.GetSession(clientId); ok {
		s.InflightLen++
		m.Repository.PutSession(s)
	}
}
func (m *Monitor) delInflight(clientId string) {
	m.Lock()
	defer m.Unlock()
	if s, ok := m.Repository.GetSession(clientId); ok {
		s.InflightLen--
		m.Repository.PutSession(s)
	}
}
func (m *Monitor) msgEnQueue(clientId string) {
	m.Lock()
	defer m.Unlock()
	if s, ok := m.Repository.GetSession(clientId); ok {
		s.MsgQueueLen++
		m.Repository.PutSession(s)
	}
}
func (m *Monitor) msgDeQueue(clientId string) {
	m.Lock()
	defer m.Unlock()
	if s, ok := m.Repository.GetSession(clientId); ok {
		s.MsgQueueLen--
		m.Repository.PutSession(s)
	}
}
func (m *Monitor) msgQueueDropped(clientId string) {
	m.Lock()
	defer m.Unlock()
	if s, ok := m.Repository.GetSession(clientId); ok {
		s.MsgQueueDropped++
		m.Repository.PutSession(s)
	}
}

// Clients returns the info for all  connected clients
func (m *Monitor) Clients() ClientList {
	m.Lock()
	defer m.Unlock()
	return m.Repository.Clients()
}

// GetClient returns the client info for the given clientId
func (m *Monitor) GetClient(clientId string) (ClientInfo, bool) {
	m.Lock()
	defer m.Unlock()
	return m.Repository.GetClient(clientId)
}

//Sessions returns the session info for all  sessions
func (m *Monitor) Sessions() SessionList {
	m.Lock()
	defer m.Unlock()
	return m.Repository.Sessions()
}

// GetSession returns the session info for the given clientId
func (m *Monitor) GetSession(clientId string) (SessionInfo, bool) {
	m.Lock()
	defer m.Unlock()
	return m.Repository.GetSession(clientId)
}

// ClientSubscriptions returns the subscription info for the given clientId
func (m *Monitor) ClientSubscriptions(clientId string) SubscriptionList {
	m.Lock()
	defer m.Unlock()
	return m.Repository.ClientSubscriptions(clientId)
}

// Subscriptions returns all  subscription info
func (m *Monitor) Subscriptions() SubscriptionList {
	m.Lock()
	defer m.Unlock()
	return m.Repository.Subscriptions()
}

// SubscriptionsInfo represents a subscription of a session
type SubscriptionsInfo struct {
	ClientId string    `json:"client_id"`
	Qos      uint8     `json:"qos"`
	Name     string    `json:"name"`
	At       time.Time `json:"at"`
}
type SubscriptionList []SubscriptionsInfo

func (s SubscriptionList) Len() int           { return len(s) }
func (s SubscriptionList) Less(i, j int) bool { return s[i].At.UnixNano() <= s[j].At.UnixNano() }
func (s SubscriptionList) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// ClientInfo represents a connected client
type ClientInfo struct {
	ClientId     string    `json:"client_id"`
	Username     string    `json:"username"`
	RemoteAddr   string    `json:"remote_addr"`
	CleanSession bool      `json:"clean_session"`
	KeepAlive    uint16    `json:"keep_alive"`
	ConnectedAt  time.Time `json:"connected_at"`
}
type ClientList []ClientInfo

func (c ClientList) Len() int { return len(c) }
func (c ClientList) Less(i, j int) bool {
	return c[i].ConnectedAt.UnixNano() <= c[j].ConnectedAt.UnixNano()
}
func (c ClientList) Swap(i, j int) { c[i], c[j] = c[j], c[i] }

// SessionInfo represents a session
type SessionInfo struct {
	ClientId        string    `json:"client_id"`
	Status          string    `json:"status"`
	RemoteAddr      string    `json:"remote_addr"`
	CleanSession    bool      `json:"clean_session"`
	Subscriptions   int       `json:"subscriptions"`
	MaxInflight     int       `json:"max_inflight"`
	InflightLen     int       `json:"inflight_len"`
	MaxMsgQueue     int       `json:"max_msg_queue"`
	MsgQueueLen     int       `json:"msg_queue_len"`
	MsgQueueDropped int       `json:"msg_queue_dropped"`
	ConnectedAt     time.Time `json:"connected_at"`
	OfflineAt       time.Time `json:"offline_at,omitempty"`
}
type SessionList []SessionInfo

func (s SessionList) Len() int { return len(s) }
func (s SessionList) Less(i, j int) bool {
	return s[i].ConnectedAt.UnixNano() <= s[j].ConnectedAt.UnixNano()
}
func (s SessionList) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

func (m *MonitorStore) PutClient(info ClientInfo) {
	m.clients[info.ClientId] = info
}
func (m *MonitorStore) GetClient(clientId string) (ClientInfo, bool) {
	info, ok := m.clients[clientId]
	return info, ok
}
func (m *MonitorStore) Clients() ClientList {
	mlen := len(m.clients)
	if mlen == 0 {
		return nil
	}
	list := make(ClientList, 0, mlen)
	for _, v := range m.clients {
		list = append(list, v)
	}
	sort.Sort(list)
	return list
}
func (m *MonitorStore) DelClient(clientId string) {
	delete(m.clients, clientId)
}
func (m *MonitorStore) PutSession(info SessionInfo) {
	m.sessions[info.ClientId] = info
}
func (m *MonitorStore) GetSession(clientId string) (SessionInfo, bool) {
	s, ok := m.sessions[clientId]
	return s, ok
}
func (m *MonitorStore) Sessions() SessionList {
	mlen := len(m.sessions)
	if mlen == 0 {
		return nil
	}
	list := make(SessionList, 0, mlen)
	for _, v := range m.sessions {
		list = append(list, v)
	}
	sort.Sort(list)
	return list
}
func (m *MonitorStore) DelSession(clientId string) {
	delete(m.sessions, clientId)
}
func (m *MonitorStore) ClientSubscriptions(clientId string) SubscriptionList {
	mlen := len(m.subscriptions[clientId])
	if mlen == 0 {
		return nil
	}
	list := make(SubscriptionList, 0, mlen)
	for _, v := range m.subscriptions[clientId] {
		list = append(list, v)
	}
	sort.Sort(list)
	return list
}
func (m *MonitorStore) DelClientSubscriptions(clientId string) {
	delete(m.subscriptions, clientId)
}
func (m *MonitorStore) PutSubscription(info SubscriptionsInfo) {
	if _, ok := m.subscriptions[info.ClientId]; !ok {
		m.subscriptions[info.ClientId] = make(map[string]SubscriptionsInfo)
	}
	m.subscriptions[info.ClientId][info.Name] = info
}
func (m *MonitorStore) DelSubscription(clientId string, topicName string) {
	if _, ok := m.subscriptions[clientId]; ok {
		delete(m.subscriptions[clientId], topicName)
	}
}
func (m *MonitorStore) Subscriptions() SubscriptionList {
	mlen := len(m.subscriptions)
	if mlen == 0 {
		return nil
	}
	list := make(SubscriptionList, 0, mlen)
	for k := range m.subscriptions {
		for _, vv := range m.subscriptions[k] {
			list = append(list, vv)
		}
	}
	sort.Sort(list)
	return list
}
func (m *MonitorStore) Open() error {
	return nil
}
func (m *MonitorStore) Close() error {
	return nil
}
