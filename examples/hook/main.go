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
	"github.com/DrmagicE/gmqtt/plugin/management"
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
	s := gmqtt.DefaultServer()
	ln, err := net.Listen("tcp", ":1883")
	if err != nil {
		log.Fatalln(err.Error())
		return
	}

	s.AddPlugins(management.New(":8080", nil))

	//authentication
	s.RegisterOnConnect(func(cs gmqtt.ChainStore, client gmqtt.Client) (code uint8) {
		username := client.OptionsReader().Username()
		password := client.OptionsReader().Password()
		if validateUser(username, password) {
			return packets.CodeAccepted
		}
		return packets.CodeBadUsernameorPsw
	})
	//acl
	s.RegisterOnSubscribe(func(cs gmqtt.ChainStore, client gmqtt.Client, topic packets.Topic) (qos uint8) {
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
	})

	s.RegisterOnMsgArrived(func(cs gmqtt.ChainStore, client gmqtt.Client, msg gmqtt.Message) (valid bool) {
		if client.OptionsReader().Username() == "subscribeonly" {
			client.Close()
			return false
		}
		//Only qos1 & qos0 are acceptable(will be delivered)
		if msg.Qos() == packets.QOS_2 {
			return false
		}
		return true
	})
	s.RegisterOnClose(func(cs gmqtt.ChainStore, client gmqtt.Client, err error) {
		log.Println("client id:"+client.OptionsReader().ClientID()+"is closed with error:", err)
	})

	s.RegisterOnStop(func(cs gmqtt.ChainStore) {
		log.Println("stop")
	})
	s.RegisterOnDeliver(func(cs gmqtt.ChainStore, client gmqtt.Client, msg gmqtt.Message) {
		log.Printf("delivering message %s to client %s", msg.Payload(), client.OptionsReader().ClientID())
	})

	s.AddTCPListenner(ln)
	log.Println("started...")
	s.Run()
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	<-signalCh
	s.Stop(context.Background())
	log.Println("stopped")
}
