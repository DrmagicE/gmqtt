package main

import (
	"github.com/DrmagicE/gmqtt/server"
	"net"
	"os"
	"os/signal"
	"syscall"
	"fmt"
	"context"
	"log"
	"sync"
	"github.com/DrmagicE/gmqtt/pkg/packets"
)


var validUserMu sync.Mutex
var validUser = map[string]string{
	"root":"rootpwd",
	"qos0":"0pwd",
	"qos1":"1pwd",
	"publishonly":"ppwd",
	"subscribeonly":"spwd",
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
	s := server.NewServer()
	ln, err := net.Listen("tcp",":1883")
	if err != nil {
		log.Fatalln(err.Error())
		return
	}

	//authentication
	s.OnConnect = func(client *server.Client) (code uint8) {
		username := client.ClientOptions().Username
		password := client.ClientOptions().Password
		if validateUser(username, password) {
			return packets.CODE_ACCEPTED
		} else {
			return packets.CODE_BAD_USERNAME_OR_PSW
		}
	}

	//acl
	s.OnSubscribe = func(client *server.Client, topic packets.Topic) uint8 {
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

	s.OnPublish = func(client *server.Client, publish *packets.Publish)  bool {
		if client.ClientOptions().Username == "subscribeonly" {
			client.Close()
			return false
		}
		//Only qos1 & qos0 are acceptable(will be delivered)
		if publish.Qos == packets.QOS_2 {
			return false
		}
		return true
	}

	s.OnClose = func(client *server.Client, err error) {
		log.Println("client id:"+ client.ClientOptions().ClientId + "is closed",err)
	}

	s.OnStop = func() {
		log.Println("server stopped...")
	}

	s.AddTCPListenner(ln)
	s.Run()
	fmt.Println("started...")
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	<-signalCh
	s.Stop(context.Background())
	fmt.Println("stopped")
}



