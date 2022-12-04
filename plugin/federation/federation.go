package federation

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/hashicorp/logutils"
	"github.com/hashicorp/serf/serf"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/config"
	"github.com/DrmagicE/gmqtt/persistence/subscription"
	"github.com/DrmagicE/gmqtt/persistence/subscription/mem"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/DrmagicE/gmqtt/retained"
	"github.com/DrmagicE/gmqtt/server"
)

var _ server.Plugin = (*Federation)(nil)

const Name = "federation"

func init() {
	server.RegisterPlugin(Name, New)
	config.RegisterDefaultPluginConfig(Name, &DefaultConfig)
}

func getSerfLogger(level string) (io.Writer, error) {
	logLevel := strings.ToUpper(level)
	var zapLevel zapcore.Level
	err := zapLevel.UnmarshalText([]byte(logLevel))
	if err != nil {
		return nil, err
	}
	zp, err := zap.NewStdLogAt(log, zapLevel)
	if err != nil {
		return nil, err
	}
	filter := &logutils.LevelFilter{
		Levels:   []logutils.LogLevel{"DEBUG", "INFO", "WARN", "ERROR"},
		MinLevel: logutils.LogLevel(logLevel),
		Writer:   zp.Writer(),
	}
	return filter, nil
}

func getSerfConfig(cfg *Config, eventCh chan serf.Event, logOut io.Writer) *serf.Config {
	serfCfg := serf.DefaultConfig()
	serfCfg.SnapshotPath = cfg.SnapshotPath
	serfCfg.RejoinAfterLeave = cfg.RejoinAfterLeave
	serfCfg.NodeName = cfg.NodeName
	serfCfg.EventCh = eventCh
	host, port, _ := net.SplitHostPort(cfg.GossipAddr)
	if host != "" {
		serfCfg.MemberlistConfig.BindAddr = host
	}
	p, _ := strconv.Atoi(port)
	serfCfg.MemberlistConfig.BindPort = p

	// set advertise
	host, port, _ = net.SplitHostPort(cfg.AdvertiseGossipAddr)
	if host != "" {
		serfCfg.MemberlistConfig.AdvertiseAddr = host
	}
	p, _ = strconv.Atoi(port)
	serfCfg.MemberlistConfig.AdvertisePort = p

	serfCfg.Tags = map[string]string{"fed_addr": cfg.AdvertiseFedAddr}
	serfCfg.LogOutput = logOut
	serfCfg.MemberlistConfig.LogOutput = logOut
	return serfCfg
}

func New(config config.Config) (server.Plugin, error) {
	log = server.LoggerWithField(zap.String("plugin", Name))
	cfg := config.Plugins[Name].(*Config)
	f := &Federation{
		config:        cfg,
		nodeName:      cfg.NodeName,
		localSubStore: &localSubStore{},
		fedSubStore: &fedSubStore{
			TrieDB:     mem.NewStore(),
			sharedSent: map[string]uint64{},
		},
		serfEventCh: make(chan serf.Event, 10000),
		sessionMgr: &sessionMgr{
			sessions: map[string]*session{},
		},
		peers: make(map[string]*peer),
		exit:  make(chan struct{}),
		wg:    &sync.WaitGroup{},
	}
	logOut, err := getSerfLogger(config.Log.Level)
	if err != nil {
		return nil, err
	}
	serfCfg := getSerfConfig(cfg, f.serfEventCh, logOut)
	s, err := serf.Create(serfCfg)
	if err != nil {
		return nil, err
	}
	f.serf = s
	return f, nil
}

var log *zap.Logger

type Federation struct {
	config      *Config
	nodeName    string
	serfMu      sync.Mutex
	serf        iSerf
	serfEventCh chan serf.Event
	sessionMgr  *sessionMgr
	// localSubStore store the subscriptions for the local node.
	// The local node will only broadcast "new subscriptions" to other nodes.
	// "New subscription" is the first subscription for a topic name.
	// It means that if two client in the local node subscribe the same topic, only the first subscription will be broadcast.
	localSubStore *localSubStore
	// fedSubStore store federation subscription tree which take nodeName as the subscriber identifier.
	// It is used to determine which node the incoming message should be routed to.
	fedSubStore *fedSubStore
	// retainedStore store is the retained store of the gmqtt core.
	// Retained message will be broadcast to other nodes in the federation.
	retainedStore retained.Store
	publisher     server.Publisher
	exit          chan struct{}
	memberMu      sync.Mutex
	peers         map[string]*peer
	wg            *sync.WaitGroup
}

type fedSubStore struct {
	*mem.TrieDB
	sharedMu sync.Mutex
	// sharedSent store the number of shared topic sent.
	// It is used to select which node the message should be send to with round-robin strategy
	sharedSent map[string]uint64
}

type sessionMgr struct {
	sync.RWMutex
	sessions map[string]*session
}

func (s *sessionMgr) add(nodeName string, id string) (cleanStart bool, nextID uint64) {
	s.Lock()
	defer s.Unlock()
	if v, ok := s.sessions[nodeName]; ok && v.id == id {
		nextID = v.nextEventID
	} else {
		// v.id != id indicates that the client side may recover from crash and need to rebuild the full state.
		cleanStart = true
	}
	if cleanStart {
		s.sessions[nodeName] = &session{
			id:       id,
			nodeName: nodeName,
			// TODO config
			seenEvents:  newLRUCache(100),
			nextEventID: 0,
			close:       make(chan struct{}),
		}
	}
	return
}

func (s *sessionMgr) del(nodeName string) {
	s.Lock()
	defer s.Unlock()
	if sess := s.sessions[nodeName]; sess != nil {
		close(sess.close)
	}
	delete(s.sessions, nodeName)
}

func (s *sessionMgr) get(nodeName string) *session {
	s.RLock()
	defer s.RUnlock()
	return s.sessions[nodeName]
}

// ForceLeave forces a member of a Serf cluster to enter the "left" state.
// Note that if the member is still actually alive, it will eventually rejoin the cluster.
// The true purpose of this method is to force remove "failed" nodes
// See https://www.serf.io/docs/commands/force-leave.html for details.
func (f *Federation) ForceLeave(ctx context.Context, req *ForceLeaveRequest) (*empty.Empty, error) {
	if req.NodeName == "" {
		return nil, errors.New("host can not be empty")
	}
	return &empty.Empty{}, f.serf.RemoveFailedNode(req.NodeName)
}

// ListMembers lists all known members in the Serf cluster.
func (f *Federation) ListMembers(ctx context.Context, req *empty.Empty) (resp *ListMembersResponse, err error) {
	resp = &ListMembersResponse{}
	for _, v := range f.serf.Members() {
		resp.Members = append(resp.Members, &Member{
			Name:   v.Name,
			Addr:   net.JoinHostPort(v.Addr.String(), strconv.Itoa(int(v.Port))),
			Tags:   v.Tags,
			Status: Status(v.Status),
		})
	}
	return resp, nil
}

// Leave triggers a graceful leave for the local node.
// This is used to ensure other nodes see the node as "left" instead of "failed".
// Note that a leaved node cannot re-join the cluster unless you restart the leaved node.
func (f *Federation) Leave(ctx context.Context, req *empty.Empty) (resp *empty.Empty, err error) {
	return &empty.Empty{}, f.serf.Leave()
}

func (f *Federation) mustEmbedUnimplementedMembershipServer() {
	return
}

// Join tells the local node to join the an existing cluster.
// See https://www.serf.io/docs/commands/join.html for details.
func (f *Federation) Join(ctx context.Context, req *JoinRequest) (resp *empty.Empty, err error) {
	for k, v := range req.Hosts {
		req.Hosts[k], err = getAddr(v, DefaultGossipPort, "hosts", false)
		if err != nil {
			return &empty.Empty{}, status.Error(codes.InvalidArgument, err.Error())
		}
	}
	_, err = f.serf.Join(req.Hosts, true)
	if err != nil {
		return nil, err
	}
	return &empty.Empty{}, nil
}

type localSubStore struct {
	localStore server.SubscriptionService
	sync.Mutex
	// [clientID][topicName]
	index map[string]map[string]struct{}
	// topics store the reference counter for each topic. (map[topicName]uint64)
	topics map[string]uint64
}

// init loads all subscriptions from gmqtt core into federation plugin.
func (l *localSubStore) init(sub server.SubscriptionService) {
	l.localStore = sub
	l.index = make(map[string]map[string]struct{})
	l.topics = make(map[string]uint64)
	l.Lock()
	defer l.Unlock()
	// copy and convert subscription tree into localSubStore
	sub.Iterate(func(clientID string, sub *gmqtt.Subscription) bool {
		l.subscribeLocked(clientID, sub.GetFullTopicName())
		return true
	}, subscription.IterationOptions{
		Type: subscription.TypeAll,
	})
}

// subscribe subscribe the topicName for the client and increase the reference counter of the topicName.
// It returns whether the subscription is new
func (l *localSubStore) subscribe(clientID string, topicName string) (new bool) {
	l.Lock()
	defer l.Unlock()
	return l.subscribeLocked(clientID, topicName)
}

func (l *localSubStore) subscribeLocked(clientID string, topicName string) (new bool) {
	if _, ok := l.index[clientID]; !ok {
		l.index[clientID] = make(map[string]struct{})
	}
	if _, ok := l.index[clientID][topicName]; !ok {
		l.index[clientID][topicName] = struct{}{}
		l.topics[topicName]++
		if l.topics[topicName] == 1 {
			return true
		}
	}
	return false
}

func (l *localSubStore) decTopicCounterLocked(topicName string) {
	if _, ok := l.topics[topicName]; ok {
		l.topics[topicName]--
		if l.topics[topicName] <= 0 {
			delete(l.topics, topicName)
		}
	}
}

// unsubscribe unsubscribe the topicName for the client and decrease the reference counter of the topicName.
// It returns whether the topicName is removed (reference counter == 0)
func (l *localSubStore) unsubscribe(clientID string, topicName string) (remove bool) {
	l.Lock()
	defer l.Unlock()
	if v, ok := l.index[clientID]; ok {
		if _, ok := v[topicName]; ok {
			delete(v, topicName)
			if len(v) == 0 {
				delete(l.index, clientID)
			}
			l.decTopicCounterLocked(topicName)
			return l.topics[topicName] == 0
		}
	}
	return false

}

// unsubscribeAll unsubscribes all topics for the given client.
// Typically, this function is called when the client session has terminated.
// It returns any topic that is removed。
func (l *localSubStore) unsubscribeAll(clientID string) (remove []string) {
	l.Lock()
	defer l.Unlock()
	for topicName := range l.index[clientID] {
		l.decTopicCounterLocked(topicName)
		if l.topics[topicName] == 0 {
			remove = append(remove, topicName)
		}
	}
	delete(l.index, clientID)
	return remove
}

type session struct {
	id          string
	nodeName    string
	nextEventID uint64
	// seenEvents cache recently seen events to avoid duplicate events.
	seenEvents *lruCache
	close      chan struct{}
}

// lruCache is the cache for recently seen events.
type lruCache struct {
	l     *list.List
	items map[uint64]struct{}
	size  int
}

func newLRUCache(size int) *lruCache {
	return &lruCache{
		l:     list.New(),
		items: make(map[uint64]struct{}),
		size:  size,
	}
}

func (l *lruCache) set(id uint64) (exist bool) {
	if _, ok := l.items[id]; ok {
		return true
	}
	if l.size == len(l.items) {
		elem := l.l.Front()
		delete(l.items, elem.Value.(uint64))
		l.l.Remove(elem)
	}
	l.items[id] = struct{}{}
	l.l.PushBack(id)
	return false
}

func getNodeNameFromContext(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Errorf(codes.DataLoss, "EventStream: failed to get metadata")
	}
	s := md.Get("node_name")
	if len(s) == 0 {
		return "", status.Errorf(codes.InvalidArgument, "EventStream: missing node_name metadata")
	}
	nodeName := s[0]
	if nodeName == "" {
		return "", status.Errorf(codes.InvalidArgument, "EventStream: missing node_name metadata")
	}
	return nodeName, nil
}

// Hello is the handler for the handshake process before opening the event stream.
func (f *Federation) Hello(ctx context.Context, req *ClientHello) (resp *ServerHello, err error) {
	nodeName, err := getNodeNameFromContext(ctx)
	if err != nil {
		return nil, err
	}
	f.memberMu.Lock()
	p := f.peers[nodeName]
	f.memberMu.Unlock()
	if p == nil {
		return nil, status.Errorf(codes.Internal, "Hello: the node [%s] has not yet joined", nodeName)
	}

	cleanStart, nextID := f.sessionMgr.add(nodeName, req.SessionId)
	if cleanStart {
		_ = f.fedSubStore.UnsubscribeAll(nodeName)
	}
	resp = &ServerHello{
		CleanStart:  cleanStart,
		NextEventId: nextID,
	}
	return resp, nil
}

func (f *Federation) eventStreamHandler(sess *session, in *Event) (ack *Ack) {
	eventID := in.Id
	// duplicated event, ignore it
	if sess.seenEvents.set(eventID) {
		log.Warn("ignore duplicated event", zap.String("event", in.String()))
		return &Ack{
			EventId: eventID,
		}
	}
	if sub := in.GetSubscribe(); sub != nil {
		_, _ = f.fedSubStore.Subscribe(sess.nodeName, &gmqtt.Subscription{
			ShareName:   sub.ShareName,
			TopicFilter: sub.TopicFilter,
		})
		return &Ack{EventId: eventID}
	}
	if msg := in.GetMessage(); msg != nil {
		pubMsg := eventToMessage(msg)
		f.publisher.Publish(pubMsg)
		if pubMsg.Retained {
			f.retainedStore.AddOrReplace(pubMsg)
		}
		return &Ack{EventId: eventID}
	}
	if unsub := in.GetUnsubscribe(); unsub != nil {
		_ = f.fedSubStore.Unsubscribe(sess.nodeName, unsub.TopicName)
		return &Ack{EventId: eventID}
	}
	return nil
}

func (f *Federation) EventStream(stream Federation_EventStreamServer) (err error) {
	defer func() {
		if err != nil && err != io.EOF {
			log.Error("EventStream error", zap.Error(err))
		}
	}()
	md, ok := metadata.FromIncomingContext(stream.Context())
	if !ok {
		return status.Errorf(codes.DataLoss, "EventStream: failed to get metadata")
	}
	s := md.Get("node_name")
	if len(s) == 0 {
		return status.Errorf(codes.InvalidArgument, "EventStream: missing node_name metadata")
	}
	nodeName := s[0]
	if nodeName == "" {
		return status.Errorf(codes.InvalidArgument, "EventStream: missing node_name metadata")
	}
	sess := f.sessionMgr.get(nodeName)
	if sess == nil {
		return status.Errorf(codes.Internal, "EventStream: node [%s] does not exist", nodeName)
	}
	errCh := make(chan error, 1)
	done := make(chan struct{})
	// close the session if the client node has been mark as failed.
	go func() {
		<-sess.close
		errCh <- fmt.Errorf("EventStream: the session of node [%s] has been closed", nodeName)
		close(done)
	}()
	go func() {
		for {
			var in *Event
			select {
			case <-done:
			default:
				in, err = stream.Recv()
				if err != nil {
					errCh <- err
					return
				}
				if ce := log.Check(zapcore.DebugLevel, "event received"); ce != nil {
					ce.Write(zap.String("event", in.String()))
				}

				ack := f.eventStreamHandler(sess, in)

				err = stream.Send(ack)
				if err != nil {
					errCh <- err
					return
				}
				if ce := log.Check(zapcore.DebugLevel, "event ack sent"); ce != nil {
					ce.Write(zap.Uint64("id", ack.EventId))
				}
				sess.nextEventID = ack.EventId + 1
			}
		}
	}()
	err = <-errCh
	if err == io.EOF {
		return nil
	}
	return err
}

func (f *Federation) mustEmbedUnimplementedFederationServer() {
	return
}

var registerAPI = func(service server.Server, f *Federation) error {
	apiRegistrar := service.APIRegistrar()
	RegisterMembershipServer(apiRegistrar, f)
	err := apiRegistrar.RegisterHTTPHandler(RegisterMembershipHandlerFromEndpoint)
	return err
}

func (f *Federation) Load(service server.Server) error {
	err := registerAPI(service, f)
	if err != nil {
		return err
	}
	f.localSubStore.init(service.SubscriptionService())
	f.retainedStore = service.RetainedService()
	f.publisher = service.Publisher()
	srv := grpc.NewServer()
	RegisterFederationServer(srv, f)
	l, err := net.Listen("tcp", f.config.FedAddr)
	if err != nil {
		return err
	}
	go func() {
		err := srv.Serve(l)
		if err != nil {
			panic(err)
		}
	}()
	t := time.NewTimer(0)
	timeout := time.NewTimer(f.config.RetryTimeout)
	for {
		select {
		case <-timeout.C:
			log.Error("retry timeout", zap.Error(err))
			if err != nil {
				err = fmt.Errorf("retry timeout: %s", err.Error())
				return err
			}
			return errors.New("retry timeout")
		case <-t.C:
			err = f.startSerf(t)
			if err == nil {
				log.Info("retry join succeed")
				return nil
			}
			log.Info("retry join failed", zap.Error(err))
		}
	}
}

func (f *Federation) Unload() error {
	err := f.serf.Leave()
	if err != nil {
		return err
	}
	return f.serf.Shutdown()
}

func (f *Federation) Name() string {
	return Name
}

func messageToEvent(msg *gmqtt.Message) *Message {
	eventMsg := &Message{
		TopicName:       msg.Topic,
		Payload:         msg.Payload,
		Qos:             uint32(msg.QoS),
		Retained:        msg.Retained,
		ContentType:     msg.ContentType,
		CorrelationData: string(msg.CorrelationData),
		MessageExpiry:   msg.MessageExpiry,
		PayloadFormat:   uint32(msg.PayloadFormat),
		ResponseTopic:   msg.ResponseTopic,
	}
	for _, v := range msg.UserProperties {
		ppt := &UserProperty{
			K: make([]byte, len(v.K)),
			V: make([]byte, len(v.V)),
		}
		copy(ppt.K, v.K)
		copy(ppt.V, v.V)
		eventMsg.UserProperties = append(eventMsg.UserProperties, ppt)
	}
	return eventMsg
}

func eventToMessage(event *Message) *gmqtt.Message {
	pubMsg := &gmqtt.Message{
		QoS:             byte(event.Qos),
		Retained:        event.Retained,
		Topic:           event.TopicName,
		Payload:         event.Payload,
		ContentType:     event.ContentType,
		CorrelationData: []byte(event.CorrelationData),
		MessageExpiry:   event.MessageExpiry,
		PayloadFormat:   packets.PayloadFormat(event.PayloadFormat),
		ResponseTopic:   event.ResponseTopic,
	}
	for _, v := range event.UserProperties {
		pubMsg.UserProperties = append(pubMsg.UserProperties, packets.UserProperty{
			K: v.K,
			V: v.V,
		})
	}
	return pubMsg
}
