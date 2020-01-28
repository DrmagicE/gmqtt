package management

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/gin-gonic/gin"
)

const CodeOK = 0
const CodeErr = -1

// Response is the response for the api server
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

func newResponse(result interface{}, pager *Pager, err error) *Response {
	resp := &Response{}
	if err == nil {
		resp.Code = CodeOK
		resp.Message = ""
		if pager != nil {
			rs := make(map[string]interface{})
			rs["result"] = result
			rs["pager"] = pager
			resp.Data = rs
		} else {
			resp.Data = result
		}
	} else {
		resp.Code = CodeErr
		resp.Data = struct{}{}
		resp.Message = err.Error()
	}
	return resp
}

type Management struct {
	monitor *monitor
	server  gmqtt.Server
	addr    string
	user    gin.Accounts //BasicAuth user info,username => password
}

// OnSessionCreatedWrapper store the client when session created
func (m *Management) OnSessionCreatedWrapper(created gmqtt.OnSessionCreated) gmqtt.OnSessionCreated {
	return func(ctx context.Context, client gmqtt.Client) {
		m.monitor.addClient(client)
		created(ctx, client)
	}
}

// OnSessionResumedWrapper refresh the client when session resumed
func (m *Management) OnSessionResumedWrapper(resumed gmqtt.OnSessionResumed) gmqtt.OnSessionResumed {
	return func(ctx context.Context, client gmqtt.Client) {
		m.monitor.addClient(client)
		resumed(ctx, client)
	}
}

// OnSessionTerminated remove the client when session terminated
func (m *Management) OnSessionTerminatedWrapper(terminated gmqtt.OnSessionTerminated) gmqtt.OnSessionTerminated {
	return func(ctx context.Context, client gmqtt.Client, reason gmqtt.SessionTerminatedReason) {
		m.monitor.deleteClient(client.OptionsReader().ClientID())
		m.monitor.deleteClientSubscriptions(client.OptionsReader().ClientID())
		terminated(ctx, client, reason)
	}
}

// OnSubscribedWrapper store the subscription
func (m *Management) OnSubscribedWrapper(subscribed gmqtt.OnSubscribed) gmqtt.OnSubscribed {
	return func(ctx context.Context, client gmqtt.Client, topic packets.Topic) {
		m.monitor.addSubscription(client.OptionsReader().ClientID(), topic)
		subscribed(ctx, client, topic)
	}
}

// OnUnsubscribedWrapper remove the subscription
func (m *Management) OnUnsubscribedWrapper(unsubscribe gmqtt.OnUnsubscribed) gmqtt.OnUnsubscribed {
	return func(ctx context.Context, client gmqtt.Client, topicName string) {
		m.monitor.deleteSubscription(client.OptionsReader().ClientID(), topicName)
		unsubscribe(ctx, client, topicName)
	}
}

func (m *Management) Load(server gmqtt.Server) error {
	m.monitor = newMonitor(server.SubscriptionStore())
	m.server = server
	m.monitor.config = server.GetConfig()
	var router gin.IRouter
	gin.SetMode(gin.ReleaseMode)
	e := gin.Default()

	if m.user != nil {
		router = e.Group("/", gin.BasicAuth(m.user))
	} else {
		router = e
	}
	router.GET("/clients", m.GetClients)
	router.GET("/client/:id", m.GetClient)
	router.GET("/sessions", m.GetSessions)
	router.GET("/session/:id", m.GetSession)
	router.GET("/subscriptions/:clientID", m.GetSubscriptions)
	router.POST("/subscribe", m.Subscribe)
	router.POST("/unsubscribe", m.Unsubscribe)
	router.POST("/publish", m.Publish)
	router.DELETE("/client/:id", m.CloseClient)
	go func() {
		err := e.Run(m.addr)
		if err != http.ErrServerClosed {
			panic(err)
		}
	}()
	return nil
}
func (m *Management) Unload() error {
	return nil
}
func (m *Management) HookWrapper() gmqtt.HookWrapper {
	return gmqtt.HookWrapper{
		OnSessionCreatedWrapper:    m.OnSessionCreatedWrapper,
		OnSessionResumedWrapper:    m.OnSessionResumedWrapper,
		OnSessionTerminatedWrapper: m.OnSessionTerminatedWrapper,
		OnSubscribedWrapper:        m.OnSubscribedWrapper,
		OnUnsubscribedWrapper:      m.OnUnsubscribedWrapper,
	}
}
func (m *Management) Name() string {
	return "management"
}

//Pager
type Pager struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	Count    int `json:"count"`
}

// newPager creates and returns the Pagination by the http GET params.
func newPager(c *gin.Context) *Pager {
	// default pager
	pager := &Pager{
		PageSize: 20,
		Page:     1,
	}
	page := c.Query("page")
	pageSize := c.Query("page_size")
	p, err := strconv.Atoi(page)
	if err == nil && p > 0 {
		pager.Page = p
	}
	ps, err := strconv.Atoi(pageSize)
	if err == nil && ps > 0 {
		pager.PageSize = ps
	}
	return pager
}

func New(addr string, user gin.Accounts) *Management {
	m := &Management{
		user: user,
		addr: addr,
	}
	return m
}

// GetClients is the handle function for "/clients"
func (m *Management) GetClients(c *gin.Context) {
	pager := newPager(c)
	rs, err := m.monitor.GetClients(pager.Page-1, pager.PageSize)
	pager.Count = len(rs)
	resp := newResponse(rs, pager, err)
	c.JSON(http.StatusOK, resp)
}

// GetSessions is the handle function for "/sessions"
func (m *Management) GetSessions(c *gin.Context) {
	pager := newPager(c)
	rs, err := m.monitor.GetSessions(pager.Page-1, pager.PageSize)
	pager.Count = len(rs)
	resp := newResponse(rs, pager, err)
	c.JSON(http.StatusOK, resp)
}

// GetSessions is the handle function for "/client/:id"
func (m *Management) GetClient(c *gin.Context) {
	id := c.Param("id")
	rs, err := m.monitor.GetClientByID(id)
	resp := newResponse(rs, nil, err)
	c.JSON(http.StatusOK, resp)
}

// GetSessions is the handle function for "/session/:id"
func (m *Management) GetSession(c *gin.Context) {
	id := c.Param("id")
	rs, err := m.monitor.GetSessionByID(id)
	resp := newResponse(rs, nil, err)
	c.JSON(http.StatusOK, resp)
}

// GetSessions is the handle function for "/subscriptions/:clientID"
func (m *Management) GetSubscriptions(c *gin.Context) {
	cid := c.Param("clientID")
	pager := newPager(c)
	rs, err := m.monitor.GetClientSubscriptions(cid, pager.Page-1, pager.PageSize)
	pager.Count = len(rs)
	resp := newResponse(rs, nil, err)
	c.JSON(http.StatusOK, resp)
}

// Subscribe is the handle function for "/subscribe" which make a subscription for a client
func (m *Management) Subscribe(c *gin.Context) {
	qosParam := c.PostForm("qos")
	topic := c.PostForm("topic")
	cid := c.PostForm("clientID")
	qos, err := strconv.Atoi(qosParam)
	if err != nil || qos < 0 || qos > 2 {
		c.JSON(http.StatusOK, newResponse(nil, nil, packets.ErrInvalQos))
		return
	}
	if !packets.ValidTopicFilter([]byte(topic)) {
		c.JSON(http.StatusOK, newResponse(nil, nil, packets.ErrInvalTopicFilter))
		return
	}

	if cid == "" {
		c.JSON(http.StatusOK, newResponse(nil, nil, errors.New("invalid clientID")))
		return
	}
	m.server.SubscriptionStore().Subscribe(cid, packets.Topic{
		Qos: uint8(qos), Name: topic,
	})
	c.JSON(http.StatusOK, newResponse(struct{}{}, nil, nil))
}

// Unsubscribe is the handle function for "/unsubscribe" which unsubscribe the topic for the client
func (m *Management) Unsubscribe(c *gin.Context) {
	topic := c.PostForm("topic")
	cid := c.PostForm("clientID")

	if !packets.ValidTopicFilter([]byte(topic)) {
		c.JSON(http.StatusOK, newResponse(nil, nil, packets.ErrInvalTopicFilter))
		return
	}

	if cid == "" {
		c.JSON(http.StatusOK, newResponse(nil, nil, errors.New("invalid clientID")))
		return
	}
	m.server.SubscriptionStore().Unsubscribe(cid, topic)
	c.JSON(http.StatusOK, newResponse(struct{}{}, nil, nil))
}

// Publish is the handle function for "/publish" which publish a message to the server
func (m *Management) Publish(c *gin.Context) {
	qosParam := c.PostForm("qos")
	topic := c.PostForm("topic")
	payload := c.PostForm("payload")
	retainFlag := c.PostForm("retain")
	var retain bool
	if retainFlag != "" {
		retain = true
	}
	qos, err := strconv.Atoi(qosParam)
	if err != nil || qos < 0 || qos > 2 {
		c.JSON(http.StatusOK, newResponse(nil, nil, packets.ErrInvalQos))
		return
	}
	if !packets.ValidTopicName([]byte(topic)) {
		c.JSON(http.StatusOK, newResponse(nil, nil, packets.ErrInvalTopicFilter))
		return
	}
	if !packets.ValidUTF8([]byte(payload)) {
		c.JSON(http.StatusOK, newResponse(nil, nil, packets.ErrInvalUtf8))
		return
	}
	msg := gmqtt.NewMessage(topic, []byte(payload), uint8(qos), gmqtt.Retained(retain))
	m.server.PublishService().Publish(msg)
	c.JSON(http.StatusOK, newResponse(struct{}{}, nil, nil))
}

// CloseClient is the handle function for "Delete /client/:id" which close the client specified by the id
func (m *Management) CloseClient(c *gin.Context) {
	id := c.Param("id")
	client := m.server.Client(id)
	if client != nil {
		client.Close()
	}
	c.JSON(http.StatusOK, newResponse(struct{}{}, nil, nil))
}
