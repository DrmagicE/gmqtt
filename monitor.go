package gmqtt

import (
	"sort"
	"sync"
	"time"
)

// Client Status
const (
	StatusOnline  = "online"
	StatusOffline = "offline"
)

// MonitorRepository is an interface which can be used to provide a persistence mechanics for the monitor data
type MonitorRepository interface {
	//Open opens the repository
	Open() error
	//Close closes the repository
	Close() error
	//PutClient puts a ClientInfo into the repository when the client connects
	PutClient(info ClientInfo)
	//GetClient returns the ClientInfo for the given clientID
	GetClient(clientID string) (ClientInfo, bool)
	//Clients returns ClientList which is the list for all connected clients, this method should be idempotency
	Clients() ClientList
	//DelClient deletes the ClientInfo from repository
	DelClient(clientID string)
	//PutSession puts a SessionInfo into monitor repository when the client is connects
	PutSession(info SessionInfo)
	//GetSession returns the SessionInfo for the given clientID
	GetSession(clientID string) (SessionInfo, bool)
	//Sessions returns SessionList which is the list for all sessions including online sessions and offline sessions, this method should be idempotency
	Sessions() SessionList
	//DelSession deletes the SessionInfo from repository
	DelSession(clientID string)
	//ClientSubscriptions returns the SubscriptionList for given clientID, this method should be idempotency
	ClientSubscriptions(clientID string) SubscriptionList
	//DelClientSubscriptions deletes the subscription info for given clientID from the repository
	DelClientSubscriptions(clientID string)
	//PutSubscription puts the SubscriptionsInfo into the repository when a new subscription is made
	PutSubscription(info SubscriptionsInfo)
	//DelSubscription deletes the topic for given clientID from repository
	DelSubscription(clientID string, topicName string)
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
	subscriptions map[string]map[string]SubscriptionsInfo //[clientID][topicName]
}

// register puts the session and client info into repository when a new client connects
func (m *Monitor) register(client *Client, sessionReuse bool) {
	m.Lock()
	defer m.Unlock()
	clientID := client.opts.ClientID
	username := client.opts.Username
	cinfo := ClientInfo{
		ClientID:     clientID,
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
		/*		sub := m.Repository.ClientSubscriptions(clientID)
				m.Repository.PutClientSubscriptions(clientID ,sub)*/
		if c, ok := m.Repository.GetSession(clientID); ok {
			c.ConnectedAt = time.Now()
			c.Status = StatusOnline
			c.InflightLen = inflightLen
			c.MsgQueueLen = msgQueueLen
			m.Repository.PutSession(c)
			return
		}
	}
	m.Repository.PutSession(SessionInfo{
		ClientID:        clientID,
		Status:          StatusOffline,
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
func (m *Monitor) unRegister(clientID string, cleanSession bool) {
	m.Lock()
	defer m.Unlock()
	m.Repository.DelClient(clientID)
	if cleanSession {
		m.Repository.DelSession(clientID)
		m.Repository.DelClientSubscriptions(clientID)
	} else {
		if s, ok := m.Repository.GetSession(clientID); ok {
			s.OfflineAt = time.Now()
			s.Status = StatusOffline
			m.Repository.PutSession(s)
		}
	}
}

// subscribe puts the subscription info into repository
func (m *Monitor) subscribe(info SubscriptionsInfo) {
	m.Lock()
	defer m.Unlock()
	m.Repository.PutSubscription(info)
	list := m.Repository.ClientSubscriptions(info.ClientID)
	if s, ok := m.Repository.GetSession(info.ClientID); ok {
		s.Subscriptions = len(list)
		m.Repository.PutSession(s)
	}

}

// unSubscribe deletes the subscription info from repository
func (m *Monitor) unSubscribe(clientID string, topicName string) {
	m.Lock()
	defer m.Unlock()
	m.Repository.DelSubscription(clientID, topicName)
	list := m.Repository.ClientSubscriptions(clientID)
	if s, ok := m.Repository.GetSession(clientID); ok {
		s.Subscriptions = len(list)
		m.Repository.PutSession(s)
	}
}
func (m *Monitor) addInflight(clientID string) {
	m.Lock()
	defer m.Unlock()
	if s, ok := m.Repository.GetSession(clientID); ok {
		s.InflightLen++
		m.Repository.PutSession(s)
	}
}
func (m *Monitor) delInflight(clientID string) {
	m.Lock()
	defer m.Unlock()
	if s, ok := m.Repository.GetSession(clientID); ok {
		s.InflightLen--
		m.Repository.PutSession(s)
	}
}
func (m *Monitor) msgEnQueue(clientID string) {
	m.Lock()
	defer m.Unlock()
	if s, ok := m.Repository.GetSession(clientID); ok {
		s.MsgQueueLen++
		m.Repository.PutSession(s)
	}
}
func (m *Monitor) msgDeQueue(clientID string) {
	m.Lock()
	defer m.Unlock()
	if s, ok := m.Repository.GetSession(clientID); ok {
		s.MsgQueueLen--
		m.Repository.PutSession(s)
	}
}
func (m *Monitor) msgQueueDropped(clientID string) {
	m.Lock()
	defer m.Unlock()
	if s, ok := m.Repository.GetSession(clientID); ok {
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

// GetClient returns the client info for the given clientID
func (m *Monitor) GetClient(clientID string) (ClientInfo, bool) {
	m.Lock()
	defer m.Unlock()
	return m.Repository.GetClient(clientID)
}

//Sessions returns the session info for all  sessions
func (m *Monitor) Sessions() SessionList {
	m.Lock()
	defer m.Unlock()
	return m.Repository.Sessions()
}

// GetSession returns the session info for the given clientID
func (m *Monitor) GetSession(clientID string) (SessionInfo, bool) {
	m.Lock()
	defer m.Unlock()
	return m.Repository.GetSession(clientID)
}

// ClientSubscriptions returns the subscription info for the given clientID
func (m *Monitor) ClientSubscriptions(clientID string) SubscriptionList {
	m.Lock()
	defer m.Unlock()
	return m.Repository.ClientSubscriptions(clientID)
}

// Subscriptions returns all  subscription info
func (m *Monitor) Subscriptions() SubscriptionList {
	m.Lock()
	defer m.Unlock()
	return m.Repository.Subscriptions()
}

// SubscriptionsInfo represents a subscription of a session
type SubscriptionsInfo struct {
	ClientID string    `json:"client_id"`
	Qos      uint8     `json:"qos"`
	Name     string    `json:"name"`
	At       time.Time `json:"at"`
}

// SubscriptionList is SubscriptionsInfo slice
type SubscriptionList []SubscriptionsInfo

func (s SubscriptionList) Len() int           { return len(s) }
func (s SubscriptionList) Less(i, j int) bool { return s[i].At.UnixNano() <= s[j].At.UnixNano() }
func (s SubscriptionList) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// ClientInfo represents a connected client
type ClientInfo struct {
	ClientID     string    `json:"client_id"`
	Username     string    `json:"username"`
	RemoteAddr   string    `json:"remote_addr"`
	CleanSession bool      `json:"clean_session"`
	KeepAlive    uint16    `json:"keep_alive"`
	ConnectedAt  time.Time `json:"connected_at"`
}

// ClientList represents ClientInfo slice
type ClientList []ClientInfo

func (c ClientList) Len() int { return len(c) }
func (c ClientList) Less(i, j int) bool {
	return c[i].ConnectedAt.UnixNano() <= c[j].ConnectedAt.UnixNano()
}
func (c ClientList) Swap(i, j int) { c[i], c[j] = c[j], c[i] }

// SessionInfo represents a session
type SessionInfo struct {
	ClientID        string    `json:"client_id"`
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

// SessionList represent SessionInfo slice
type SessionList []SessionInfo

func (s SessionList) Len() int { return len(s) }
func (s SessionList) Less(i, j int) bool {
	return s[i].ConnectedAt.UnixNano() <= s[j].ConnectedAt.UnixNano()
}
func (s SessionList) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

//PutClient puts a ClientInfo into the repository when the client connects
func (m *MonitorStore) PutClient(info ClientInfo) {
	m.clients[info.ClientID] = info
}

//GetClient returns the ClientInfo for the given clientID
func (m *MonitorStore) GetClient(clientID string) (ClientInfo, bool) {
	info, ok := m.clients[clientID]
	return info, ok
}

//Clients returns ClientList which is the list for all connected clients, this method should be idempotency
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

//DelClient deletes the ClientInfo from repository
func (m *MonitorStore) DelClient(clientID string) {
	delete(m.clients, clientID)
}

//PutSession puts a SessionInfo into monitor repository when the client is connects
func (m *MonitorStore) PutSession(info SessionInfo) {
	m.sessions[info.ClientID] = info
}

//GetSession returns the SessionInfo for the given clientID
func (m *MonitorStore) GetSession(clientID string) (SessionInfo, bool) {
	s, ok := m.sessions[clientID]
	return s, ok
}

//Sessions returns SessionList which is the list for all sessions including online sessions and offline sessions, this method should be idempotency
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

//DelSession deletes the SessionInfo from repository
func (m *MonitorStore) DelSession(clientID string) {
	delete(m.sessions, clientID)
}

//ClientSubscriptions returns the SubscriptionList for given clientID, this method should be idempotency
func (m *MonitorStore) ClientSubscriptions(clientID string) SubscriptionList {
	mlen := len(m.subscriptions[clientID])
	if mlen == 0 {
		return nil
	}
	list := make(SubscriptionList, 0, mlen)
	for _, v := range m.subscriptions[clientID] {
		list = append(list, v)
	}
	sort.Sort(list)
	return list
}

//DelClientSubscriptions deletes the subscription info for given clientID from the repository
func (m *MonitorStore) DelClientSubscriptions(clientID string) {
	delete(m.subscriptions, clientID)
}

//PutSubscription puts the SubscriptionsInfo into the repository when a new subscription is made
func (m *MonitorStore) PutSubscription(info SubscriptionsInfo) {
	if _, ok := m.subscriptions[info.ClientID]; !ok {
		m.subscriptions[info.ClientID] = make(map[string]SubscriptionsInfo)
	}
	m.subscriptions[info.ClientID][info.Name] = info
}

//DelSubscription deletes the topic for given clientID from repository
func (m *MonitorStore) DelSubscription(clientID string, topicName string) {
	if _, ok := m.subscriptions[clientID]; ok {
		delete(m.subscriptions[clientID], topicName)
	}
}

//Subscriptions returns all  subscriptions of the server
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

//Open opens the repository
func (m *MonitorStore) Open() error {
	return nil
}

//Close close the repository
func (m *MonitorStore) Close() error {
	return nil
}
