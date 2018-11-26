package main

import (
	"fmt"
	"github.com/DrmagicE/gmqtt/cmd/broker/run"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {

	cmd := &run.Command{}
	if err := cmd.Run(os.Args[1:]...); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	log.Println("running...")
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	// Block until one of the signals above is received
	<-signalCh
	err := cmd.Close()
	if err != nil {
		log.Println("stop error:", err)
	} else {
		log.Println("stopped")
	}

}
