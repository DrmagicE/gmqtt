package federation

import (
	"context"
	"io"
	"net"
	"sync"

	"github.com/hashicorp/serf/serf"
	"go.uber.org/zap"
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

func New(config config.Config) (server.Plugin, error) {
	return &Federation{
		config:        config.Plugins[Name].(*Config),
		nodeName:      config.Plugins[Name].(*Config).NodeName,
		localSubStore: &localSubStore{},
		feSubStore:    mem.NewStore(),
		serfEventCh:   make(chan serf.Event, 10000),
		sessions:      make(map[string]*session),
		peers:         make(map[string]*peer),
		exit:          make(chan struct{}),
		wg:            &sync.WaitGroup{},
	}, nil
}

var log *zap.Logger

type Federation struct {
	config      *Config
	nodeName    string
	serfEventCh chan serf.Event

	sessMu   sync.Mutex
	sessions map[string]*session

	localSubStore *localSubStore
	// feSubStore store federation subscription tree which take nodeName as clientID, It is used to determine which node the incoming message should be routed to.
	feSubStore    *mem.TrieDB
	retainedStore retained.Store
	peers         map[string]*peer
	publisher     server.Publisher
	exit          chan struct{}
	mu            sync.Mutex
	wg            *sync.WaitGroup
}

type localSubStore struct {
	sync.Mutex
	// [clientID][topicName]
	index map[string]map[string]struct{}
	// topics store the reference counter for each topic
	topics map[string]uint64
}

func (l *localSubStore) init(sub server.SubscriptionService) {
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
	var nextID uint64
	var cleanStart bool
	f.sessMu.Lock()
	defer f.sessMu.Unlock()
	if v, ok := f.sessions[nodeName]; ok && v.id == req.SessionId {
		nextID = v.nextEventID
	} else {
		// v.id != req.SessionId indicates that the client side may recover from crash and need to rebuild the full state.
		cleanStart = true
	}
	if cleanStart == true {
		f.sessions[nodeName] = &session{
			id:          req.SessionId,
			nodeName:    nodeName,
			nextEventID: 0,
		}
	}
	resp = &ServerHello{
		CleanStart:  cleanStart,
		NextEventId: nextID,
	}
	return resp, nil

}

func (f *Federation) eventStreamHandler(sess *session, in *Event) (ack *Ack) {
	eventID := in.Id
	if sub := in.GetSubscribe(); sub != nil {
		_, _ = f.feSubStore.Subscribe(sess.nodeName, &gmqtt.Subscription{
			ShareName:   sub.ShareName,
			TopicFilter: sub.TopicFilter,
		})
		return &Ack{EventId: eventID}
	}
	if msg := in.GetMessage(); msg != nil {
		pubMsg := &gmqtt.Message{
			QoS:             byte(msg.Qos),
			Retained:        msg.Retained,
			Topic:           msg.TopicName,
			Payload:         []byte(msg.Payload),
			ContentType:     msg.ContentType,
			CorrelationData: []byte(msg.CorrelationData),
			MessageExpiry:   msg.MessageExpiry,
			PayloadFormat:   packets.PayloadFormat(msg.PayloadFormat),
			ResponseTopic:   msg.ResponseTopic,
		}
		for _, v := range msg.UserProperties {
			pubMsg.UserProperties = append(pubMsg.UserProperties, packets.UserProperty{
				K: v.K,
				V: v.V,
			})
		}
		f.publisher.Publish(pubMsg)
		if pubMsg.Retained {
			f.retainedStore.AddOrReplace(pubMsg)
		}
		return &Ack{EventId: eventID}
	}
	if unsub := in.GetUnsubscribe(); unsub != nil {
		_ = f.feSubStore.Unsubscribe(sess.nodeName, unsub.TopicName)
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
	f.sessMu.Lock()
	sess := f.sessions[nodeName]
	f.sessMu.Unlock()
	if sess == nil {
		return status.Errorf(codes.Internal, "EventStream: node not exist")
	}
	for {
		var in *Event
		in, err = stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		log.Debug("event received", zap.String("event", in.String()))
		ack := f.eventStreamHandler(sess, in)

		err = stream.Send(ack)
		if err != nil {
			return err
		}
		log.Debug("event ack sent", zap.Uint64("id", ack.EventId))
		sess.nextEventID = ack.EventId + 1
	}
}

func (f *Federation) mustEmbedUnimplementedFederationServer() {
	return
}

func (f *Federation) Load(service server.Server) error {
	f.localSubStore.init(service.SubscriptionService())
	f.retainedStore = service.RetainedService()
	f.publisher = service.Publisher()
	log = server.LoggerWithField(zap.String("plugin", Name))
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
	return f.startSerf()
}

func (f *Federation) Unload() error {
	return nil
}

func (f *Federation) Name() string {
	return Name
}

func messageToEvent(msg *gmqtt.Message) *Message {
	eventMsg := &Message{
		TopicName:       msg.Topic,
		Payload:         string(msg.Payload),
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
