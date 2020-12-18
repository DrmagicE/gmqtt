package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	gen_plugin "github.com/DrmagicE/gmqtt/cmd/gmqctl/command/gen-plugin"
)

var (
	rootCmd = &cobra.Command{
		Use:     "gmqctl",
		Long:    "gmqctl is a command line tool for gmqtt",
		Version: Version,
	}
)

func init() {
	rootCmd.AddCommand(gen_plugin.Command)
}

func must(err error) {
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func main() {
	must(rootCmd.Execute())
}
