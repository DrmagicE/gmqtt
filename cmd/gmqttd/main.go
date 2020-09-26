package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"

	"github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"

	_ "github.com/DrmagicE/gmqtt/topicalias" // set default topicalias manager

	"github.com/DrmagicE/gmqtt/cmd/gmqttd/command"
)

var (
	rootCmd = &cobra.Command{
		Use:     "gmqttd",
		Long:    "Gmqtt is a MQTT broker that fully implements MQTT V5.0 and V3.1.1 protocol",
		Version: Version,
	}
)

func must(err error) {
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	d, err := homedir.Dir()
	must(err)
	rootCmd.PersistentFlags().StringVarP(&command.ConfigFile, "config", "c", d+"/gmqtt.yml", "The configration file path")

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
