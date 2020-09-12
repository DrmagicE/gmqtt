package main

import (
	"fmt"
	"net/http"
	"os"

	_ "net/http/pprof"

	"github.com/DrmagicE/gmqtt/cmd/gmqttd/command"
	"github.com/spf13/cobra"
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
