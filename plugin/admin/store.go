package admin

import (
	"container/list"
	"sync"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/config"
	"github.com/DrmagicE/gmqtt/server"
)

type store struct {
	clientMu            sync.RWMutex
	clientIndexer       *Indexer
	subMu               sync.RWMutex
	subIndexer          *Indexer
	config              config.Config
	statsReader         server.StatsReader
	subscriptionService server.SubscriptionService
}

func newStore(statsReader server.StatsReader, config config.Config) *store {
	return &store{
		clientIndexer: NewIndexer(),
		subIndexer:    NewIndexer(),
		statsReader:   statsReader,
		config:        config,
	}
}

func (s *store) addSubscription(clientID string, sub *gmqtt.Subscription) {
	s.subMu.Lock()
	defer s.subMu.Unlock()

	subInfo := &Subscription{
		TopicName:         sub.GetFullTopicName(),
		Id:                sub.ID,
		Qos:               uint32(sub.QoS),
		NoLocal:           sub.NoLocal,
		RetainAsPublished: sub.RetainAsPublished,
		RetainHandling:    uint32(sub.RetainHandling),
		ClientId:          clientID,
	}
	key := clientID + "_" + sub.GetFullTopicName()
	s.subIndexer.Set(key, subInfo)

}

func (s *store) removeSubscription(clientID string, topicName string) {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	s.subIndexer.Remove(clientID + "_" + topicName)
}

func (s *store) addClient(client server.Client) {
	c := newClientInfo(client, uint32(s.config.MQTT.MaxQueuedMsg))
	s.clientMu.Lock()
	s.clientIndexer.Set(c.ClientId, c)
	s.clientMu.Unlock()
}

func (s *store) setClientDisconnected(clientID string) {
	s.clientMu.Lock()
	defer s.clientMu.Unlock()
	l := s.clientIndexer.GetByID(clientID)
	if l == nil {
		return
	}
	l.Value.(*Client).DisconnectedAt = timestamppb.Now()
}

func (s *store) removeClient(clientID string) {
	s.clientMu.Lock()
	s.clientIndexer.Remove(clientID)
	s.clientMu.Unlock()
}

// GetClientByID returns the client information for the given client id.
func (s *store) GetClientByID(clientID string) *Client {
	s.clientMu.RLock()
	defer s.clientMu.RUnlock()
	c := s.getClientByIDLocked(clientID)
	fillClientInfo(c, s.statsReader)
	return c
}

func newClientInfo(client server.Client, maxQueue uint32) *Client {
	clientOptions := client.ClientOptions()
	rs := &Client{
		ClientId:       clientOptions.ClientID,
		Username:       clientOptions.Username,
		KeepAlive:      int32(clientOptions.KeepAlive),
		Version:        int32(client.Version()),
		RemoteAddr:     client.Connection().RemoteAddr().String(),
		LocalAddr:      client.Connection().LocalAddr().String(),
		ConnectedAt:    timestamppb.New(client.ConnectedAt()),
		DisconnectedAt: nil,
		SessionExpiry:  clientOptions.SessionExpiry,
		MaxInflight:    uint32(clientOptions.MaxInflight),
		MaxQueue:       maxQueue,
	}
	return rs
}

func (s *store) getClientByIDLocked(clientID string) *Client {
	if i := s.clientIndexer.GetByID(clientID); i != nil {
		return i.Value.(*Client)
	}
	return nil
}

func fillClientInfo(c *Client, stsReader server.StatsReader) {
	if c == nil {
		return
	}
	sts, ok := stsReader.GetClientStats(c.ClientId)
	if !ok {
		return
	}
	c.SubscriptionsCurrent = uint32(sts.SubscriptionStats.SubscriptionsCurrent)
	c.SubscriptionsTotal = uint32(sts.SubscriptionStats.SubscriptionsTotal)
	c.PacketsReceivedBytes = sts.PacketStats.BytesReceived.Total
	c.PacketsReceivedNums = sts.PacketStats.ReceivedTotal.Total
	c.PacketsSendBytes = sts.PacketStats.BytesSent.Total
	c.PacketsSendNums = sts.PacketStats.SentTotal.Total
	c.MessageDropped = sts.MessageStats.GetDroppedTotal()
	c.InflightLen = uint32(sts.MessageStats.InflightCurrent)
	c.QueueLen = uint32(sts.MessageStats.QueuedCurrent)
}

// GetClients
func (s *store) GetClients(page, pageSize uint) (rs []*Client, total uint32, err error) {
	rs = make([]*Client, 0)
	fn := func(elem *list.Element) {
		c := elem.Value.(*Client)
		fillClientInfo(c, s.statsReader)
		rs = append(rs, elem.Value.(*Client))
	}
	s.clientMu.RLock()
	defer s.clientMu.RUnlock()
	offset, n := GetOffsetN(page, pageSize)
	s.clientIndexer.Iterate(fn, offset, n)
	return rs, uint32(s.clientIndexer.Len()), nil
}

// GetSubscriptions
func (s *store) GetSubscriptions(page, pageSize uint) (rs []*Subscription, total uint32, err error) {
	rs = make([]*Subscription, 0)
	fn := func(elem *list.Element) {
		rs = append(rs, elem.Value.(*Subscription))
	}
	s.subMu.RLock()
	defer s.subMu.RUnlock()
	offset, n := GetOffsetN(page, pageSize)
	s.subIndexer.Iterate(fn, offset, n)
	return rs, uint32(s.subIndexer.Len()), nil
}
