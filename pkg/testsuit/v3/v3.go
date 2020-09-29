package v3

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/pkg/codes"
	"github.com/DrmagicE/gmqtt/pkg/packets"
)

var (
	Addr             = "127.0.0.1:1883"
	aclient, bclient *Client
	topics           = []string{"TopicA", "TopicA/B", "Topic/C", "TopicA/C", "/TopicA"}
)

func InvalidPid(got packets.PacketID) error {
	return fmt.Errorf("receive invalid pid: %d", got)
}
func MismatchedPid(want, got packets.PacketID) error {
	return fmt.Errorf("receive mismatched pid, want: %d, got %d", want, got)
}

func init() {
	var err error
	aclient, err = Connect(Addr, &packets.Connect{
		Version:       packets.Version311,
		ClientID:      []byte("aclientid"),
		ProtocolName:  []byte("MQTT"),
		ProtocolLevel: packets.Version311,
		KeepAlive:     60,
	})
	if err != nil {
		panic(err)
	}
	bclient, err = Connect(Addr, &packets.Connect{
		Version:       packets.Version311,
		ClientID:      []byte("bclientid"),
		ProtocolName:  []byte("MQTT"),
		ProtocolLevel: packets.Version311,
		KeepAlive:     60,
	})
	if err != nil {
		panic(err)
	}
}

type Subscribed func(suback *packets.Suback)

type sub struct {
	sub *gmqtt.Subscription
	pid packets.PacketID
}
type Callback struct {
	Subscribed
}

type Client struct {
	conn    net.Conn
	r       *packets.Reader
	w       *packets.Writer
	connack *packets.Connack
	connect *packets.Connect
	pid     packets.PacketID

	subscribed    map[string]*sub
	subscribedPid map[packets.PacketID]*packets.Subscribe

	pub []*packets.Publish

	sync.Once
	err error
	Callback
}

func (c *Client) setError(err error) {
	c.Once.Do(func() {
		c.err = err
		c.conn.Close()
	})

}
func (c *Client) NextPID() packets.PacketID {
	if c.pid == 65535 {
		c.pid = 1
	} else {
		c.pid++
	}
	return c.pid
}

func (c *Client) readHandler() {
	var err error
	defer func() {
		if err != nil {
			c.setError(err)
		}
	}()
	for {
		var p packets.Packet
		p, err = c.r.ReadPacket()
		if err != nil {
			return
		}
		switch p.(type) {
		case *packets.Suback:
			pa := p.(*packets.Suback)
			if _, ok := c.subscribedPid[pa.PacketID]; !ok {
				err = InvalidPid(pa.PacketID)
				return
			}
			c.Callback.Subscribed(pa)
		case *packets.Puback:
			pa := p.(*packets.Puback)
			if len(c.pub) == 0 {
				err = InvalidPid(pa.PacketID)
				return
			}
			if c.pub[0].PacketID != pa.PacketID {
				err = MismatchedPid(c.pub[0].PacketID, pa.PacketID)
			}
			c.pub = c.pub[1:]
		case *packets.Pubrel:
		case *packets.Pubrec:

		case *packets.Pubcomp:
		}
	}

}

func Connect(addr string, connect *packets.Connect) (client *Client, err error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			conn.Close()
		}
	}()
	c := &Client{
		pid: 1,
	}
	c.conn = conn
	c.connect = connect
	c.r = packets.NewReader(c.conn)
	c.w = packets.NewWriter(c.conn)

	err = c.w.WriteAndFlush(connect)
	if err != nil {
		return nil, err
	}
	p, err := c.r.ReadPacket()
	if err != nil {
		return nil, err
	}
	if connack, ok := p.(*packets.Connack); ok {
		c.connack = connack

		go c.readHandler()
		return c, nil
	}
	return nil, errors.New("receive invalid packet")
}
func (c *Client) Write(packet packets.Packet) error {
	return c.w.WriteAndFlush(packet)
}
func (c *Client) Read() (packets.Packet, error) {
	return c.r.ReadPacket()
}

func (c *Client) Subscribe(subscription ...*gmqtt.Subscription) error {
	var topics []packets.Topic
	for _, v := range subscription {
		topics = append(topics, packets.Topic{
			SubOptions: packets.SubOptions{
				Qos: v.QoS,
			},
			Name: v.TopicFilter,
		})
	}
	pid := c.NextPID()
	subp := &packets.Subscribe{
		Version:  packets.Version311,
		PacketID: c.NextPID(),
		Topics:   topics,
	}
	err := c.Write(subp)
	if err != nil {
		return err
	}
	for _, v := range subscription {
		c.subscribed[v.TopicFilter] = &sub{
			sub: v,
			pid: pid,
		}
	}
	c.subscribedPid[pid] = subp
	return nil
}

func (c *Client) Publish(message *gmqtt.Message) error {
	pub := gmqtt.MessageToPublish(message, packets.Version311)
	pub.PacketID = c.pid
	err := c.w.WriteAndFlush(pub)
	if err != nil {
		return err
	}
	switch pub.Qos {
	case packets.Qos0:
		return nil
	case packets.Qos1:
		p, err := c.r.ReadPacket()
		if err != nil {
			return err
		}
		if ack, ok := p.(*packets.Puback); ok {

		}

	case packets.Qos2:

	}
}

func TestBasic(t *testing.T) {
	a := assert.New(t)
	a.False(aclient.connack.SessionPresent)
	suback, err := aclient.Subscribe(&gmqtt.Subscription{
		TopicFilter: topics[0],
	})
	a.Nil(err)
	a.Equal(codes.Success, suback.Payload[0])

}
