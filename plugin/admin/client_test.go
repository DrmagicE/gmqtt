package admin

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/DrmagicE/gmqtt/config"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/DrmagicE/gmqtt/server"
)

var mockConfig = config.Config{
	MQTT: config.MQTT{
		MaxQueuedMsg: 10,
	},
}

type dummyConn struct {
	net.Conn
}

// LocalAddr returns the local network address.
func (d *dummyConn) LocalAddr() net.Addr {
	return &net.TCPAddr{}
}
func (d *dummyConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{}
}

func TestClientService_List_Get(t *testing.T) {
	a := assert.New(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cs := server.NewMockClientService(ctrl)
	sr := server.NewMockStatsReader(ctrl)

	admin := &Admin{
		statsReader:   sr,
		clientService: cs,
		store:         newStore(sr, mockConfig),
	}
	c := &clientService{
		a: admin,
	}
	now := time.Now()
	client := server.NewMockClient(ctrl)
	client.EXPECT().Version().Return(packets.Version5).AnyTimes()
	client.EXPECT().Connection().Return(&dummyConn{}).AnyTimes()
	client.EXPECT().ConnectedAt().Return(now).AnyTimes()
	created := admin.OnSessionCreatedWrapper(func(ctx context.Context, client server.Client) {})
	for i := 0; i < 10; i++ {
		sr.EXPECT().GetClientStats(strconv.Itoa(i)).AnyTimes()
		client.EXPECT().ClientOptions().Return(&server.ClientOptions{
			ClientID:            strconv.Itoa(i),
			Username:            strconv.Itoa(i),
			KeepAlive:           uint16(i),
			SessionExpiry:       uint32(i),
			MaxInflight:         uint16(i),
			ReceiveMax:          uint16(i),
			ClientMaxPacketSize: uint32(i),
			ServerMaxPacketSize: uint32(i),
			ClientTopicAliasMax: uint16(i),
			ServerTopicAliasMax: uint16(i),
			RequestProblemInfo:  true,
			UserProperties: []*packets.UserProperty{
				{
					K: []byte{1, 2},
					V: []byte{1, 2},
				},
			},
			RetainAvailable:      true,
			WildcardSubAvailable: true,
			SubIDAvailable:       true,
			SharedSubAvailable:   true,
		})
		created(context.Background(), client)
	}

	resp, err := c.List(context.Background(), &ListClientRequest{
		PageSize: 0,
		Page:     0,
	})
	a.Nil(err)
	a.Len(resp.Clients, 10)
	for k, v := range resp.Clients {
		addr := net.TCPAddr{}
		a.Equal(&Client{
			ClientId:             strconv.Itoa(k),
			Username:             strconv.Itoa(k),
			KeepAlive:            int32(k),
			Version:              int32(packets.Version5),
			RemoteAddr:           addr.String(),
			LocalAddr:            addr.String(),
			ConnectedAt:          timestamppb.New(now),
			DisconnectedAt:       nil,
			SessionExpiry:        uint32(k),
			MaxInflight:          uint32(k),
			MaxQueue:             uint32(mockConfig.MQTT.MaxQueuedMsg),
			PacketsReceivedBytes: 0,
			PacketsReceivedNums:  0,
			PacketsSendBytes:     0,
			PacketsSendNums:      0,
			MessageDropped:       0,
		}, v)
	}

	getResp, err := c.Get(context.Background(), &GetClientRequest{
		ClientId: "1",
	})
	a.Nil(err)
	a.Equal(resp.Clients[1], getResp.Client)

	pagingResp, err := c.List(context.Background(), &ListClientRequest{
		PageSize: 2,
		Page:     2,
	})
	a.Nil(err)
	a.Len(pagingResp.Clients, 2)
	a.Equal(resp.Clients[2], pagingResp.Clients[0])
	a.Equal(resp.Clients[3], pagingResp.Clients[1])
}

func TestClientService_Delete(t *testing.T) {
	a := assert.New(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cs := server.NewMockClientService(ctrl)
	sr := server.NewMockStatsReader(ctrl)

	admin := &Admin{
		statsReader:   sr,
		clientService: cs,
		store:         newStore(sr, mockConfig),
	}
	c := &clientService{
		a: admin,
	}
	client := server.NewMockClient(ctrl)
	client.EXPECT().Close()
	cs.EXPECT().GetClient("1").Return(client)
	_, err := c.Delete(context.Background(), &DeleteClientRequest{
		ClientId:     "1",
		CleanSession: false,
	})
	a.Nil(err)
}

func TestClientService_Delete_CleanSession(t *testing.T) {
	a := assert.New(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cs := server.NewMockClientService(ctrl)
	sr := server.NewMockStatsReader(ctrl)

	admin := &Admin{
		statsReader:   sr,
		clientService: cs,
		store:         newStore(sr, mockConfig),
	}
	c := &clientService{
		a: admin,
	}
	cs.EXPECT().TerminateSession("1")
	_, err := c.Delete(context.Background(), &DeleteClientRequest{
		ClientId:     "1",
		CleanSession: true,
	})
	a.Nil(err)
}
