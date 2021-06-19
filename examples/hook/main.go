package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/config"
	_ "github.com/DrmagicE/gmqtt/persistence"
	"github.com/DrmagicE/gmqtt/pkg/codes"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/DrmagicE/gmqtt/server"
	_ "github.com/DrmagicE/gmqtt/topicalias/fifo"
)

var validUser = map[string]string{
	"root":           "pwd",
	"qos0":           "pwd",
	"qos1":           "pwd",
	"publishonly":    "pwd",
	"subscribeonly":  "pwd",
	"disable_shared": "pwd",
}

func validateUser(username string, password string) bool {
	if pwd, ok := validUser[username]; ok {
		if pwd == password {
			return true
		}
	}
	return false

}

func main() {
	ln, err := net.Listen("tcp", ":1883")
	if err != nil {
		log.Fatalln(err.Error())
		return
	}
	//authentication
	var onBasicAuth server.OnBasicAuth = func(ctx context.Context, client server.Client, req *server.ConnectRequest) error {
		username := string(req.Connect.Username)
		password := string(req.Connect.Password)
		if validateUser(username, password) {
			if username == "disable_shared" {
				// disable shared subscription for this particular client
				req.Options.SharedSubAvailable = false
			}
			return nil
		}
		// check the client version, return a compatible reason code.
		v := client.Version()
		if packets.IsVersion3X(v) {
			return codes.NewError(codes.V3BadUsernameorPassword)
		}
		if packets.IsVersion5(v) {
			return codes.NewError(codes.BadUserNameOrPassword)
		}

		// return nil if pass authentication.
		return nil
	}

	// subscription acl
	var onSubscribe server.OnSubscribe = func(ctx context.Context, client server.Client, req *server.SubscribeRequest) error {
		username := client.ClientOptions().Username
		// iterate all subscriptions in the Subscribe packet.
		for k, v := range req.Subscriptions {
			switch username {
			case "root":
				// if root, there are not limit on the subscription qos level.
			case "qos0":
				// if qos0, grants qos0 level
				req.GrantQoS(k, packets.Qos0)
			case "qos1":
				// if qos1, grants at most qos 1 qos level.
				if v.Sub.QoS > packets.Qos1 {
					req.GrantQoS(k, packets.Qos1)
				}
			case "publishonly":
				// reject any subscriptions for the publishonly client.
				req.Reject(k, &codes.Error{
					Code: codes.NotAuthorized,
					ErrorDetails: codes.ErrorDetails{
						ReasonString: []byte("publish only"),
					},
				})
			}
		}
		return nil
	}

	var onMsgArrived server.OnMsgArrived = func(ctx context.Context, client server.Client, req *server.MsgArrivedRequest) error {
		version := client.Version()
		if client.ClientOptions().Username == "subscribeonly" {
			switch version {
			case packets.Version311:
				// For v3 client:
				// If a Server implementation does not authorize a PUBLISH to be performed by a Client;
				// it has no way of informing that Client. It MUST either make a positive acknowledgement,
				// according to the normal QoS rules, or close the Network Connection [MQTT-3.3.5-2].
				req.Drop()
				// Or close the client.
				// client.Close()
				return nil

			case packets.Version5:
				return &codes.Error{
					Code: codes.NotAuthorized,
				}
				// Or close the client. For V5 clients, it is recommended to use Disconnect() to send a disconnect packet to client, which is a good feature introduced by V5.
				//req.Drop()
				//client.Disconnect(&packets.Disconnect{
				//	Version: packets.Version5,
				//	Code:    codes.UnspecifiedError,
				//})
				//return
			}
		}

		if req.Message.QoS == packets.Qos2 {
			req.Drop()
			return &codes.Error{
				Code: codes.NotAuthorized,
				ErrorDetails: codes.ErrorDetails{
					ReasonString: []byte("not authorized"),
					UserProperties: []struct {
						K []byte
						V []byte
					}{
						{
							K: []byte("user property key"),
							V: []byte("user property value"),
						},
					},
				},
			}
		}
		return nil
	}
	OnClosed := func(ctx context.Context, client server.Client, err error) {
		log.Println("client id:"+client.ClientOptions().ClientID+"is closed with error:", err)
	}
	onStop := func(ctx context.Context) {
		log.Println("stop")
	}
	OnDelivered := func(ctx context.Context, client server.Client, msg *gmqtt.Message) {
		log.Printf("delivering message %s to client %s", msg.Payload, client.ClientOptions().ClientID)
	}
	hooks := server.Hooks{
		OnBasicAuth:  onBasicAuth,
		OnSubscribe:  onSubscribe,
		OnMsgArrived: onMsgArrived,
		OnClosed:     OnClosed,
		OnStop:       onStop,
		OnDelivered:  OnDelivered,
	}

	l, _ := zap.NewDevelopment()
	s := server.New(
		server.WithTCPListener(ln),
		server.WithHook(hooks),
		server.WithLogger(l),
		server.WithConfig(config.DefaultConfig()),
	)
	go func() {
		signalCh := make(chan os.Signal, 1)
		signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
		<-signalCh
		s.Stop(context.Background())
	}()
	err = s.Run()
	if err != nil {
		panic(err)
	}
}
