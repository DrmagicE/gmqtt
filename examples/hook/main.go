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
	"github.com/DrmagicE/gmqtt/pkg/codes"
	"github.com/DrmagicE/gmqtt/pkg/packets"
)

var validUser = map[string]string{
	"root":          "rootpwd",
	"qos0":          "0pwd",
	"qos1":          "1pwd",
	"publishonly":   "ppwd",
	"subscribeonly": "spwd",
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
	var onBasicAuth gmqtt.OnBasicAuth = func(ctx context.Context, client gmqtt.Client, req *gmqtt.ConnectRequest) (resp *gmqtt.ConnectResponse) {
		username := string(req.Connect.Username)
		password := string(req.Connect.Password)
		if validateUser(username, password) {
			return &gmqtt.ConnectResponse{
				Code: codes.Success,
			}
		}
		switch client.Version() {
		case packets.Version5:
			return &gmqtt.ConnectResponse{
				Code: codes.BadUserNameOrPassword,
			}
		case packets.Version311:
			return &gmqtt.ConnectResponse{
				Code: codes.V3BadUsernameorPassword,
			}
		}
		return nil
	}

	var onSubscribe gmqtt.OnSubscribe = func(ctx context.Context, client gmqtt.Client, subscribe *packets.Subscribe) (resp *gmqtt.SubscribeResponse, errDetails *codes.ErrorDetails) {
		resp = &gmqtt.SubscribeResponse{
			Topics: subscribe.Topics,
		}
		username := client.ClientOptions().Username
		for k := range resp.Topics {
			switch username {
			case "root":
			case "qos0":
				resp.Topics[k].Qos = packets.Qos0
			case "qos1":
				resp.Topics[k].Qos = packets.Qos1
			case "publishonly":
				resp.Topics[k].Qos = packets.SubscribeFailure
				if client.Version() == packets.Version5 {
					errDetails = &codes.ErrorDetails{
						ReasonString: []byte("publish only"),
						UserProperties: []struct {
							K []byte
							V []byte
						}{
							{
								K: []byte("key"),
								V: []byte("value"),
							},
						},
					}
				}

			}
		}
		return resp, errDetails
	}

	var onMsgArrived gmqtt.OnMsgArrived = func(ctx context.Context, client gmqtt.Client, publish *packets.Publish) (message packets.Message, e error) {
		version := client.Version()
		if client.ClientOptions().Username == "subscribeonly" {
			switch version {
			case packets.Version311:
				client.Close()
			case packets.Version5:
				client.Disconnect(&packets.Disconnect{
					Version: packets.Version5,
					Code:    codes.UnspecifiedError,
				})

			}
		}
		//Only qos1 & qos0 are acceptable(will be delivered)
		if publish.Qos == packets.Qos2 {
			switch version {
			case packets.Version311:
				return nil, nil
			case packets.Version5:
				return nil, &codes.Error{
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
		}
		return gmqtt.MessageFromPublish(publish), nil
	}
	onClose := func(ctx context.Context, client gmqtt.Client, err error) {
		log.Println("client id:"+client.ClientOptions().ClientID+"is closed with error:", err)
	}
	onStop := func(ctx context.Context) {
		log.Println("stop")
	}
	onDeliver := func(ctx context.Context, client gmqtt.Client, msg packets.Message) {
		log.Printf("delivering message %s to client %s", msg.Payload(), client.ClientOptions().ClientID)
	}
	hooks := gmqtt.Hooks{
		OnBasicAuth:  onBasicAuth,
		OnSubscribe:  onSubscribe,
		OnMsgArrived: onMsgArrived,
		OnClose:      onClose,
		OnStop:       onStop,
		OnDeliver:    onDeliver,
	}

	l, _ := zap.NewDevelopment()
	s := gmqtt.NewServer(
		gmqtt.WithTCPListener(ln),
		gmqtt.WithHook(hooks),
		gmqtt.WithLogger(l),
	)
	s.Run()
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	<-signalCh
	s.Stop(context.Background())
}
