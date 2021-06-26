package admin

import (
	"go.uber.org/zap"

	"github.com/DrmagicE/gmqtt/config"
	"github.com/DrmagicE/gmqtt/server"
)

var _ server.Plugin = (*Admin)(nil)

const Name = "admin"

func init() {
	server.RegisterPlugin(Name, New)
}

func New(config config.Config) (server.Plugin, error) {
	return &Admin{}, nil
}

var log *zap.Logger

// Admin providers gRPC and HTTP API that enables the external system to interact with the broker.
type Admin struct {
	statsReader   server.StatsReader
	publisher     server.Publisher
	clientService server.ClientService
	store         *store
}

func (a *Admin) registerHTTP(g server.APIRegistrar) (err error) {
	err = g.RegisterHTTPHandler(RegisterClientServiceHandlerFromEndpoint)
	if err != nil {
		return err
	}
	err = g.RegisterHTTPHandler(RegisterSubscriptionServiceHandlerFromEndpoint)
	if err != nil {
		return err
	}
	err = g.RegisterHTTPHandler(RegisterPublishServiceHandlerFromEndpoint)
	if err != nil {
		return err
	}
	return nil
}

func (a *Admin) Load(service server.Server) error {
	log = server.LoggerWithField(zap.String("plugin", Name))
	apiRegistrar := service.APIRegistrar()
	RegisterClientServiceServer(apiRegistrar, &clientService{a: a})
	RegisterSubscriptionServiceServer(apiRegistrar, &subscriptionService{a: a})
	RegisterPublishServiceServer(apiRegistrar, &publisher{a: a})
	err := a.registerHTTP(apiRegistrar)
	if err != nil {
		return err
	}
	a.statsReader = service.StatsManager()
	a.store = newStore(a.statsReader, service.GetConfig())
	a.store.subscriptionService = service.SubscriptionService()
	a.publisher = service.Publisher()
	a.clientService = service.ClientService()
	return nil
}

func (a *Admin) Unload() error {
	return nil
}

func (a *Admin) Name() string {
	return Name
}
