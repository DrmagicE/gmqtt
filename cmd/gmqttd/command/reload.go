package command

import (
	"io/ioutil"
	"os"
	"strconv"
	"syscall"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/DrmagicE/gmqtt/config"
)

// NewReloadCommand creates a *cobra.Command object for reload command.
func NewReloadCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reload",
		Short: "Reload gmqtt broker",
		Run: func(cmd *cobra.Command, args []string) {
			var c config.Config
			var err error
			c, err = config.ParseConfig(ConfigFile)
			if os.IsNotExist(err) {
				c = config.DefaultConfig()
			} else {
				must(err)
			}
			b, err := ioutil.ReadFile(c.PidFile)
			must(errors.Wrap(err, "read pid file error"))
			pid, err := strconv.Atoi(string(b))
			must(errors.Wrap(err, "read pid file error"))
			p, err := os.FindProcess(pid)
			must(errors.Wrap(err, "find process error"))
			err = p.Signal(syscall.SIGHUP)
			must(err)
		},
	}
	return cmd
}
