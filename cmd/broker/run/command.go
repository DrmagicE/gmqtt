package run

import (
	"context"
	"flag"
	"fmt"
	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/cmd/broker/restapi"
	"log"
	"os"
	"runtime"
	"runtime/pprof"
)

// Command represents the command executed by `go run main.go`
type Command struct {
	Server *gmqtt.Server
}

// Options contains the path of configuration file
type Options struct {
	ConfigPath string
}

// NewCommand returns Command
func NewCommand() *Command {
	return &Command{}
}

// ParseFlags parses the args into Options.
func (cmd *Command) ParseFlags(args ...string) (Options, error) {
	var options Options
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	fs.StringVar(&options.ConfigPath, "config", "", "")
	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}
	return options, nil
}

// ParseConfig parses the config at path.
// It returns a demo configuration if path is blank.
func (cmd *Command) ParseConfig(path string) (*Config, error) {

	//default path
	if path == "" {
		path = "config.yaml"
	}
	config := NewConfig()
	if err := config.FromConfigFile(path); err != nil {
		return nil, err
	}
	return config, nil
}

// Run parses the command arguments and starts the server.
func (cmd *Command) Run(args ...string) error {
	options, err := cmd.ParseFlags(args...)
	if err != nil {
		return err
	}
	config, err := cmd.ParseConfig(options.ConfigPath)
	if err != nil {
		return fmt.Errorf("parse config: %s", err)
	}
	// Validate the configuration.
	if err := config.Validate(); err != nil {
		return err
	}

	s, err := NewServer(config)
	if err != nil {
		return fmt.Errorf("create server: %s", err)
	}
	cmd.Server = s

	if s.Monitor != nil && config.HttpServerConfig.Addr != "" {

		api := &restapi.RestServer{
			Addr: config.HttpServerConfig.Addr,
			Srv:  s,
			User: config.HttpServerConfig.User,
		}
		go api.Run()
	}
	s.Run()
	return nil
}

// Close shuts down the server.
func (cmd *Command) Close() error {
	stopProfile()
	if cmd.Server != nil {
		return cmd.Server.Stop(context.Background())
	}
	return nil
}

// prof stores the file locations of active profiles.
var prof struct {
	cpu *os.File
	mem *os.File
}

// StartProfile initializes the cpu and memory profile, if specified.
func startProfile(cpuprofile, memprofile string) {
	if cpuprofile != "" {
		f, err := os.Create(cpuprofile)
		if err != nil {
			log.Fatalf("cpuprofile: %v", err)
		}
		log.Printf("writing CPU profile to: %s\n", cpuprofile)
		prof.cpu = f
		pprof.StartCPUProfile(prof.cpu)
	}

	if memprofile != "" {
		f, err := os.Create(memprofile)
		if err != nil {
			log.Fatalf("memprofile: %v", err)
		}
		log.Printf("writing mem profile to: %s\n", memprofile)
		prof.mem = f
		runtime.MemProfileRate = 4096
	}
}

// StopProfile closes the cpu and memory profiles if they are running.
func stopProfile() {
	if prof.cpu != nil {
		pprof.StopCPUProfile()
		prof.cpu.Close()
		log.Println("CPU profile stopped")
	}
	if prof.mem != nil {
		pprof.Lookup("heap").WriteTo(prof.mem, 0)
		prof.mem.Close()
		log.Println("mem profile stopped")
	}
}
