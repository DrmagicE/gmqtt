package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"

	"github.com/spf13/cobra"

	"github.com/DrmagicE/gmqtt/cmd/gmqttd/command"
)

var rootCmd = &cobra.Command{
	Use:     "gmqttd",
	Long:    "Gmqtt is a MQTT broker that fully supports MQTT V5.0/V3.1.1 protocol",
	Version: Version,
}

func init() {
	rootCmd.AddCommand(command.NewStartCmd())
	rootCmd.AddCommand(command.NewReloadCommand())
}

func main() {
	go func() {
		http.ListenAndServe(":6060", nil)
	}()
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
