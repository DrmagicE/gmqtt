package subscribe

import (
	"context"
	"github.com/eclipse/paho.mqtt.golang"
	"log"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Server represents the subscribe benchmark server.
type Server struct {
	Options    Options
	wg         sync.WaitGroup
	StartAt    time.Time
	FinishedAt time.Time
	close      chan struct{}
	connected  int64 //connected client
	subscribed int64 //subscribed  msg
}

func (srv *Server) connect(clientID string) (mqtt.Client, error) {
	opts := mqtt.NewClientOptions()
	opts.SetUsername(srv.Options.Username)
	opts.SetClientID(clientID)
	opts.SetPassword(srv.Options.Password)
	opts.SetCleanSession(srv.Options.CleanSession)
	opts.SetProtocolVersion(4)
	opts.SetAutoReconnect(false)
	opts.OnConnectionLost = func(client mqtt.Client, e error) {
		log.Println("connection lost:", e)
	}
	opts.AddBroker("tcp://" + srv.Options.Host + srv.Options.Port)
	c := mqtt.NewClient(opts)
	t := c.Connect()
	t.Wait()
	if t.Error() == nil {
		atomic.AddInt64(&srv.connected, 1)
	}
	return c, t.Error()
}

func (srv *Server) subscribe(ctx context.Context, client mqtt.Client) {
	for n := 0; n < srv.Options.Number; n++ {
		select {
		case <-ctx.Done():
			return
		default:
			time.Sleep(time.Duration(srv.Options.SubscribeInterval) * time.Microsecond)
			t := client.Subscribe(srv.Options.Topic+strconv.Itoa(n), byte(srv.Options.Qos), nil)
			t.Wait()
			if t.Error() != nil {
				log.Println("subscribe error:", t.Error())
				return
			}
			atomic.AddInt64(&srv.subscribed, 1)
		}
	}
}

func (srv *Server) printProgress() {
	c := atomic.LoadInt64(&srv.connected)
	pc := atomic.LoadInt64(&srv.subscribed)
	log.Printf("%d clients connected,%d topics subscribed,", c, pc)
}

func (srv *Server) printFinished() {
	c := atomic.LoadInt64(&srv.connected)
	pc := atomic.LoadInt64(&srv.subscribed)
	runningTime := (srv.FinishedAt.Unix() - srv.StartAt.Unix())
	qps := (pc + c) / runningTime
	log.Printf("benchmark testing finished in %d seconds", runningTime)
	log.Printf("%d clients connected,%d topics subscribed,QPS: %d", c, pc, qps)
}

func (srv *Server) displayProgress(ctx context.Context) {
	log.Printf("starting benchmark testing:")
	t := time.NewTicker(2 * time.Second)
	for {
		select {
		case <-t.C:
			srv.printProgress()
		case <-ctx.Done():
			return
		}
	}
}

// Run starts the server.
func (srv *Server) Run(ctx context.Context) {
	srv.StartAt = time.Now()
	go srv.displayProgress(ctx)
loop:
	for i := 0; i < srv.Options.Count; i++ {
		select {
		case <-ctx.Done():
			break loop
		default:
			time.Sleep(time.Duration(srv.Options.ConnectionInterval) * time.Microsecond)
			srv.wg.Add(1)
			go func(i int) {
				defer srv.wg.Done()
				c, err := srv.connect(strconv.Itoa(i))
				if err != nil {
					log.Println("connect error:", err)
					return
				}
				srv.subscribe(ctx, c)
			}(i)
		}
	}
	srv.wg.Wait()
	srv.FinishedAt = time.Now()
	srv.printFinished()

}
