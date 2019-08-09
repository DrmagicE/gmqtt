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

	s := gmqtt.NewServer()
	ln, err := net.Listen("tcp", ":1883")
	if err != nil {
		log.Fatalln(err.Error())
		return
	}

	s.AddPlugins(management.New(":8080", nil))

	//authentication
	/*	s.RegisterOnConnect(func(connect gmqtt.OnConnect) gmqtt.OnConnect {
			return func(cs gmqtt.ChainStore, client gmqtt.Client) (code uint8) {
				username := client.ClientOptions().Username
				password := client.ClientOptions().Password
				fmt.Println("onConnect")
				if validateUser(username, password) {
					return packets.CodeAccepted
				}
				return packets.CodeBadUsernameorPsw
			}
		})
		//acl
		s.RegisterOnSubscribe(func(subscribe gmqtt.OnSubscribe) gmqtt.OnSubscribe {
			return func(cs gmqtt.ChainStore, client gmqtt.Client, topic packets.Topic) (qos uint8) {
				if client.ClientOptions().Username == "root" {
					return topic.Qos
				}
				if client.ClientOptions().Username == "qos0" {
					if topic.Qos <= packets.QOS_0 {
						return topic.Qos
					}
					return packets.QOS_0
				}
				if client.ClientOptions().Username == "qos1" {
					if topic.Qos <= packets.QOS_1 {
						return topic.Qos
					}
					return packets.QOS_1
				}
				if client.ClientOptions().Username == "publishonly" {
					return packets.SUBSCRIBE_FAILURE
				}
				return topic.Qos
			}
		})

		s.RegisterOnMsgArrived(func(arrived gmqtt.OnMsgArrived) gmqtt.OnMsgArrived {
			return func(cs gmqtt.ChainStore, client gmqtt.Client, msg gmqtt.Message) (valid bool) {
				if client.ClientOptions().Username == "subscribeonly" {
					client.Close()
					return false
				}
				//Only qos1 & qos0 are acceptable(will be delivered)
				if msg.Qos() == packets.QOS_2 {
					return false
				}
				return true
			}
		})
		s.RegisterOnClose(func(onClose gmqtt.OnClose) gmqtt.OnClose {
			return func(cs gmqtt.ChainStore, client gmqtt.Client, err error) {
				log.Println("client id:"+client.ClientOptions().ClientID+"is closed with error:", err)
			}
		})

		s.RegisterOnStop(func(stop gmqtt.OnStop) gmqtt.OnStop {
			return func(cs gmqtt.ChainStore) {
				log.Println("stop")
			}
		})

		s.RegisterOnDeliver(func(deliver gmqtt.OnDeliver) gmqtt.OnDeliver {
			return func(cs gmqtt.ChainStore, client gmqtt.Client, msg gmqtt.Message) {
				log.Printf("delivering message %s to client %s", msg.Payload(), client.ClientOptions().ClientID)
			}
		})

		s.RegisterOnAcked(func(acked gmqtt.OnAcked) gmqtt.OnAcked {
			return func(cs gmqtt.ChainStore, client gmqtt.Client, msg gmqtt.Message) {
				log.Println("msg ack", msg)
			}
		})*/

	s.AddTCPListenner(ln)
	log.Println("started...")
	s.Run()
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	<-signalCh
	s.Stop(context.Background())
	log.Println("stopped")
}
