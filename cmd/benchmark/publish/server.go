package publish

import (
	"context"
	"github.com/eclipse/paho.mqtt.golang"
	"log"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Server represents the publish benchmark server.
type Server struct {
	Options     Options
	wg          sync.WaitGroup
	StartAt     time.Time
	FinishedAt  time.Time
	close       chan struct{}
	connected   int64 //connected client
	published   int64 //published  msg
	distributed int64 //distributed msg
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

func (srv *Server) publish(ctx context.Context, client mqtt.Client) {
	b := make([]byte, srv.Options.Size)
	for j := 0; j < srv.Options.Size; j++ {
		b[j] = 31
	}
	for n := 0; n < srv.Options.Number; n++ {
		select {
		case <-ctx.Done():
			return
		default:
			time.Sleep(time.Duration(srv.Options.PublishInterval) * time.Microsecond)
			t := client.Publish(srv.Options.Topic, byte(srv.Options.Qos), false, b)
			t.Wait()
			if t.Error() != nil {
				log.Println("publish error:", t.Error())
				return
			}
			atomic.AddInt64(&srv.published, 1)
		}
	}
}

func (srv *Server) subscribe(clientID string) (mqtt.Client, error) {
	c, err := srv.connect(clientID)
	if err != nil {
		log.Println("subscriber connect error:", err)
		return nil, err
	}
	t := c.Subscribe("#", byte(srv.Options.Qos), func(client mqtt.Client, message mqtt.Message) {
		atomic.AddInt64(&srv.distributed, 1)
	})
	t.Wait()
	return c, t.Error()
}

func (srv *Server) printProgress() {
	c := atomic.LoadInt64(&srv.connected)
	pc := atomic.LoadInt64(&srv.published)
	d := atomic.LoadInt64(&srv.distributed)
	log.Printf("%d clients connected,%d messages published,%d messages distributed", c, pc, d)
}

func (srv *Server) printFinished() {
	c := atomic.LoadInt64(&srv.connected)
	pc := atomic.LoadInt64(&srv.published)
	d := atomic.LoadInt64(&srv.distributed)
	runningTime := (srv.FinishedAt.Unix() - srv.StartAt.Unix())
	qps := (pc + c + d) / runningTime
	log.Printf("benchmark testing finished in %d seconds", runningTime)
	log.Printf("%d clients connected,%d messages published,%d messages distributed,QPS: %d", c, pc, d, qps)
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
	for i := 0; i < srv.Options.SubCount; i++ {
		srv.wg.Add(1)
		go func(i int) {
			defer srv.wg.Done()
			_, err := srv.subscribe("sub" + strconv.Itoa(i))
			if err != nil {
				log.Println("subscriber connect error:", err)
			}
		}(i)
	}
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
				srv.publish(ctx, c)
			}(i)
		}
	}
	srv.wg.Wait()
	srv.FinishedAt = time.Now()
	srv.printFinished()

}
