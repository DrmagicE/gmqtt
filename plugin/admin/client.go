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

func (c *clientService) List(ctx context.Context, req *ListClientRequest) (*ListClientResponse, error) {
	page, pageSize := getPage(req.Page, req.PageSize)
	clients, total, err := c.a.store.GetClients(page-1, pageSize)
	if err != nil {
		return &ListClientResponse{}, err
	}
	return &ListClientResponse{
		Clients:    clients,
		TotalCount: total,
	}, nil
}

func (c *clientService) Get(context.Context, *GetClientRequest) (*ListClientResponse, error) {
	return &ListClientResponse{}, nil
}

func (c *clientService) Delete(context.Context, *DeleteClientRequest) (*empty.Empty, error) {
	return nil, nil
}
