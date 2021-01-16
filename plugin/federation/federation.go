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
		config:      config.Plugins[Name].(*Config),
		nodeName:    config.Plugins[Name].(*Config).NodeName,
		serfEventCh: make(chan serf.Event, 10000),
		members:     make(map[string]serf.Member),
		sessions:    make(map[string]*session),
		peers:       make(map[string]*peer),
		exit:        make(chan struct{}),
		wg:          &sync.WaitGroup{},
	}, nil
}

var log *zap.Logger

type Federation struct {
	config      *Config
	nodeName    string
	serfEventCh chan serf.Event
	members     map[string]serf.Member

	sessMu   sync.Mutex
	sessions map[string]*session

	// localSubStore is a copy of broker subscription tree, the difference is localSubStore will take local node name as clientID.
	// If the remote node requests a full subscription state, the plugin will iterate it and send them to the node.
	localSubStore server.SubscriptionService
	// feSubStore store federation subscription tree which take nodeName as clientID, It is used to determine which node the incoming message should be routed to.
	feSubStore    *mem.TrieDB
	retainedStore retained.Store
	peers         map[string]*peer
	publisher     server.Publisher
	exit          chan struct{}
	mu            sync.Mutex
	wg            *sync.WaitGroup
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
			ShareName:         sub.ShareName,
			TopicFilter:       sub.TopicFilter,
			ID:                sub.Id,
			QoS:               byte(sub.Qos),
			NoLocal:           sub.NoLocal,
			RetainAsPublished: sub.RetainAsPublished,
			RetainHandling:    byte(sub.RetainHandling),
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
		log.Info("event received", zap.String("event", in.String()))
		ack := f.eventStreamHandler(sess, in)
		err = stream.Send(ack)
		if err != nil {
			return err
		}
		sess.nextEventID = ack.EventId + 1
	}
}

func (f *Federation) mustEmbedUnimplementedFederationServer() {
	return
}

func (f *Federation) Load(service server.Server) error {
	// copy and convert subscription tree into localSubStore
	service.SubscriptionService().Iterate(func(clientID string, sub *gmqtt.Subscription) bool {
		_, _ = f.localSubStore.Subscribe(f.nodeName, sub)
		return true
	}, subscription.IterationOptions{
		Type: subscription.TypeAll,
	})
	f.retainedStore = service.RetainedService()
	f.publisher = service.Publisher()
	f.feSubStore = mem.NewStore()
	f.localSubStore = mem.NewStore()
	log = server.LoggerWithField(zap.String("plugin", Name))
	log.Info("local node", zap.String("node_name", f.nodeName))
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

func subscriptionToEvent(subscription *gmqtt.Subscription) *Subscribe {
	return &Subscribe{
		ShareName:         subscription.ShareName,
		TopicFilter:       subscription.TopicFilter,
		Id:                subscription.ID,
		Qos:               uint32(subscription.QoS),
		NoLocal:           subscription.NoLocal,
		RetainAsPublished: subscription.RetainAsPublished,
		RetainHandling:    uint32(subscription.RetainHandling),
	}
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
		ppt := &UserProperties{}
		copy(ppt.K, v.K)
		copy(ppt.V, v.V)
		eventMsg.UserProperties = append(eventMsg.UserProperties, ppt)
	}
	return eventMsg
}
