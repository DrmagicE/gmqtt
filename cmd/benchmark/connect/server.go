package connect

import (
	"context"
	"github.com/eclipse/paho.mqtt.golang"
	"log"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Server represents the connect benchmark server.
type Server struct {
	Options    Options
	wg         sync.WaitGroup
	StartAt    time.Time
	FinishedAt time.Time
	close      chan struct{}
	connected  int64 //connected client
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

func (srv *Server) printProgress() {
	c := atomic.LoadInt64(&srv.connected)
	log.Printf("%d clients connected,", c)
}

func (srv *Server) printFinished() {
	c := atomic.LoadInt64(&srv.connected)
	runningTime := (srv.FinishedAt.Unix() - srv.StartAt.Unix())
	qps := c / runningTime
	log.Printf("benchmark testing finished in %d seconds", runningTime)
	log.Printf("%d clients connected, QPS: %d", c, qps)
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
				_, err := srv.connect(strconv.Itoa(i))
				if err != nil {
					log.Println("connect error:", err)
					return
				}
			}(i)
		}
	}
	srv.wg.Wait()
	srv.FinishedAt = time.Now()
	srv.printFinished()

}
