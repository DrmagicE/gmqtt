package restapi

import (
	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
)

// RestServer represents the REST API server.
type RestServer struct {
	Addr string
	Srv  *gmqtt.Server
	User gin.Accounts //BasicAuth user info,username => password
}

// OkResponse is the response for a successful request.
type OkResponse struct {
	Code   int           `json:"code"`
	Result []interface{} `json:"result"`
}

// Run starts the REST server.
func (rest *RestServer) Run() {
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()

	basicAuth := router.Group("/", gin.BasicAuth(rest.User))
	basicAuth.GET("/clients", func(c *gin.Context) {
		pager := NewHttpPager(c.Request)
		provider := new(SliceDataProvider)
		list := rest.Srv.Monitor.Clients()
		provider.Pager = pager
		provider.SetModels(list)
		b := make(gmqtt.ClientList, 0)
		provider.Models(&b)
		obj := PageAbleObj{
			Models:       b,
			Page:         pager.Page + 1,
			PageSize:     pager.PageSize,
			CurrentCount: provider.Count(),
			TotalCount:   pager.TotalCount,
			TotalPage:    pager.PageCount(),
		}
		c.JSON(http.StatusOK, obj)

	})
	basicAuth.GET("/client/:id", func(c *gin.Context) {

		model, exist := rest.Srv.Monitor.GetClient(c.Param("id"))
		if !exist {
			c.String(http.StatusNotFound, "%s", "Client Not Found")
			return
		}
		c.JSON(http.StatusOK, model)
	})
	basicAuth.GET("/sessions", func(c *gin.Context) {
		pager := NewHttpPager(c.Request)
		provider := new(SliceDataProvider)
		list := rest.Srv.Monitor.Sessions()
		provider.Pager = pager
		provider.SetModels(list)
		b := make(gmqtt.SessionList, 0)
		provider.Models(&b)
		obj := PageAbleObj{
			Models:       b,
			Page:         pager.Page + 1,
			PageSize:     pager.PageSize,
			CurrentCount: provider.Count(),
			TotalCount:   pager.TotalCount,
			TotalPage:    pager.PageCount(),
		}
		c.JSON(http.StatusOK, obj)
	})
	basicAuth.GET("/session/:id", func(c *gin.Context) {
		model, ok := rest.Srv.Monitor.GetSession(c.Param("id"))
		if !ok {
			c.String(http.StatusNotFound, "%s", "Session Not Found")
			return
		}
		c.JSON(http.StatusOK, model)
	})
	basicAuth.GET("/subscriptions/:id", func(c *gin.Context) {
		pager := NewHttpPager(c.Request)
		provider := new(SliceDataProvider)
		list := rest.Srv.Monitor.ClientSubscriptions(c.Param("id"))
		provider.Pager = pager
		provider.SetModels(list)
		b := make(gmqtt.SubscriptionList, 0)
		provider.Models(&b)
		obj := PageAbleObj{
			Models:       b,
			Page:         pager.Page + 1,
			PageSize:     pager.PageSize,
			CurrentCount: provider.Count(),
			TotalCount:   pager.TotalCount,
			TotalPage:    pager.PageCount(),
		}
		c.JSON(http.StatusOK, obj)
	})
	basicAuth.GET("/subscriptions", func(c *gin.Context) {
		pager := NewHttpPager(c.Request)
		provider := new(SliceDataProvider)
		list := rest.Srv.Monitor.Subscriptions()
		provider.Pager = pager
		provider.SetModels(list)
		b := make(gmqtt.SubscriptionList, 0)
		provider.Models(&b)
		obj := PageAbleObj{
			Models:       b,
			Page:         pager.Page + 1,
			PageSize:     pager.PageSize,
			CurrentCount: provider.Count(),
			TotalCount:   pager.TotalCount,
			TotalPage:    pager.PageCount(),
		}
		c.JSON(http.StatusOK, obj)
	})
	basicAuth.POST("/subscribe", func(c *gin.Context) {
		qosParam := c.PostForm("qos")
		topic := c.PostForm("topic")
		cid := c.PostForm("clientID")
		qos, err := strconv.Atoi(qosParam)
		if err != nil {
			c.String(http.StatusBadRequest, "%s", packets.ErrInvalQos)
			return
		}
		if qos < 0 || qos > 2 {
			c.String(http.StatusBadRequest, "%s", packets.ErrInvalQos)
			return
		}

		if !packets.ValidTopicFilter([]byte(topic)) {
			c.String(http.StatusBadRequest, "%s", packets.ErrInvalTopicFilter)
			return
		}

		if cid == "" {
			c.String(http.StatusBadRequest, "%s", "invalid clientID")
			return
		}
		rest.Srv.Subscribe(cid, []packets.Topic{
			{Qos: uint8(qos), Name: topic},
		})
		c.JSON(http.StatusOK, OkResponse{
			Code:   0,
			Result: make([]interface{}, 0),
		})
	})
	basicAuth.POST("/unsubscribe", func(c *gin.Context) {
		topic := c.PostForm("topic")
		cid := c.PostForm("clientID")

		if !packets.ValidTopicFilter([]byte(topic)) {
			c.String(http.StatusBadRequest, "%s", packets.ErrInvalTopicFilter)
			return
		}

		if cid == "" {
			c.String(http.StatusBadRequest, "%s", "invalid clientID")
			return
		}
		rest.Srv.UnSubscribe(cid, []string{topic})
		c.JSON(http.StatusOK, OkResponse{
			Code:   0,
			Result: make([]interface{}, 0),
		})
	})
	basicAuth.POST("/publish", func(c *gin.Context) {
		qosParam := c.PostForm("qos")
		topic := c.PostForm("topic")
		payload := c.PostForm("payload")
		qos, err := strconv.Atoi(qosParam)
		if err != nil {
			c.String(http.StatusBadRequest, "%s", packets.ErrInvalQos)
			return
		}
		if qos < 0 || qos > 2 {
			c.String(http.StatusBadRequest, "%s", packets.ErrInvalQos)
			return
		}
		if !packets.ValidTopicName([]byte(topic)) {
			c.String(http.StatusBadRequest, "%s", packets.ErrInvalTopicName)
			return
		}
		if !packets.ValidUTF8([]byte(payload)) {
			c.String(http.StatusBadRequest, "%s", packets.ErrInvalUtf8)
			return
		}
		pub := &packets.Publish{
			Qos:       uint8(qos),
			TopicName: []byte(topic),
			Payload:   []byte(payload),
		}
		rest.Srv.Publish(pub)
		c.JSON(http.StatusOK, OkResponse{
			Code:   0,
			Result: make([]interface{}, 0),
		})

	})

	router.Run(rest.Addr)

}
