package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/pkg/packets"
)

var validUserMu sync.Mutex
var validUser = map[string]string{
	"root":          "rootpwd",
	"qos0":          "0pwd",
	"qos1":          "1pwd",
	"publishonly":   "ppwd",
	"subscribeonly": "spwd",
}

func validateUser(username string, password string) bool {
	validUserMu.Lock()
	defer validUserMu.Unlock()
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
	onConnect := func(ctx context.Context, client gmqtt.Client) (code uint8) {
		username := client.OptionsReader().Username()
		password := client.OptionsReader().Password()
		if validateUser(username, password) {
			return packets.CodeAccepted
		}
		return packets.CodeBadUsernameorPsw
	}

	// acl
	onSubscribe := func(ctx context.Context, client gmqtt.Client, topic packets.Topic) (qos uint8) {
		if client.OptionsReader().Username() == "root" {
			return topic.Qos
		}
		if client.OptionsReader().Username() == "qos0" {
			if topic.Qos <= packets.QOS_0 {
				return topic.Qos
			}
			return packets.QOS_0
		}
		if client.OptionsReader().Username() == "qos1" {
			if topic.Qos <= packets.QOS_1 {
				return topic.Qos
			}
			return packets.QOS_1
		}
		if client.OptionsReader().Username() == "publishonly" {
			return packets.SUBSCRIBE_FAILURE
		}
		return topic.Qos
	}
	onMsgArrived := func(ctx context.Context, client gmqtt.Client, msg packets.Message) (valid bool) {
		if client.OptionsReader().Username() == "subscribeonly" {
			client.Close()
			return false
		}
		//Only qos1 & qos0 are acceptable(will be delivered)
		if msg.Qos() == packets.QOS_2 {
			return false
		}
		return true
	}
	onClose := func(ctx context.Context, client gmqtt.Client, err error) {
		log.Println("client id:"+client.OptionsReader().ClientID()+"is closed with error:", err)
	}
	onStop := func(ctx context.Context) {
		log.Println("stop")
	}
	onDeliver := func(ctx context.Context, client gmqtt.Client, msg packets.Message) {
		log.Printf("delivering message %s to client %s", msg.Payload(), client.OptionsReader().ClientID())
	}
	hooks := gmqtt.Hooks{
		OnConnect:    onConnect,
		OnSubscribe:  onSubscribe,
		OnMsgArrived: onMsgArrived,
		OnClose:      onClose,
		OnStop:       onStop,
		OnDeliver:    onDeliver,
	}

	s := gmqtt.NewServer(
		gmqtt.WithTCPListener(ln),
		gmqtt.WithHook(hooks),
	)

	log.Println("started...")
	s.Run()
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	<-signalCh
	s.Stop(context.Background())
	log.Println("stopped")
}
