package admin

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
)

type clientService struct {
	a *Admin
}

func (c *clientService) mustEmbedUnimplementedClientServiceServer() {
	return
}

// List lists clients information which the session is valid in the broker (both connected and disconnected).
func (c *clientService) List(ctx context.Context, req *ListClientRequest) (*ListClientResponse, error) {
	page, pageSize := GetPage(req.Page, req.PageSize)
	clients, total, err := c.a.store.GetClients(page, pageSize)
	if err != nil {
		return &ListClientResponse{}, err
	}
	return &ListClientResponse{
		Clients:    clients,
		TotalCount: total,
	}, nil
}

// Get returns the client information for given request client id.
func (c *clientService) Get(ctx context.Context, req *GetClientRequest) (*GetClientResponse, error) {
	if req.ClientId == "" {
		return nil, ErrInvalidArgument("client_id", "")
	}
	client := c.a.store.GetClientByID(req.ClientId)
	if client == nil {
		return nil, ErrNotFound
	}
	return &GetClientResponse{
		Client: client,
	}, nil
}

// Delete force disconnect.
func (c *clientService) Delete(ctx context.Context, req *DeleteClientRequest) (*empty.Empty, error) {
	if req.ClientId == "" {
		return nil, ErrInvalidArgument("client_id", "")
	}
	if req.CleanSession {
		c.a.clientService.TerminateSession(req.ClientId)
	} else {
		client := c.a.clientService.GetClient(req.ClientId)
		if client != nil {
			client.Close()
		}
	}
	return &empty.Empty{}, nil
}
