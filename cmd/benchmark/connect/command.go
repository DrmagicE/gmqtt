package connect

import (
	"context"
	"flag"
	"time"
)

// Command represents the command executed by `go run connect_benchmark.go`
type Command struct {
	Server *Server
}

// Options represents the connect benchmark options
type Options struct {
	Host               string //default:  localhost
	Port               string //default: :1883
	Username           string
	Password           string
	ConnectionInterval int
	Count              int //number of clients
	CleanSession       bool
	Time               int //timeout
}

//ParseFlags parses the args into Options.
func (cmd *Command) ParseFlags(args ...string) (Options, error) {
	var options Options
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	fs.StringVar(&options.Host, "h", "localhost", "host")
	fs.StringVar(&options.Port, "p", ":1883", "port")
	fs.StringVar(&options.Username, "u", "", "username")
	fs.StringVar(&options.Password, "pwd", "", "password")
	fs.IntVar(&options.ConnectionInterval, "ci", 100, "connection interval (ms)")
	fs.IntVar(&options.Count, "c", 1000, "number of clients")
	fs.BoolVar(&options.CleanSession, "C", true, "clean session")
	fs.IntVar(&options.Time, "t", 0, "timeout (second)")
	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}
	return options, nil
}

// Run parses the command arguments and starts the server.
func (cmd *Command) Run(args ...string) error {
	options, err := cmd.ParseFlags(args...)
	if err != nil {
		return err
	}
	srv := &Server{
		Options: options,
	}
	var ctx context.Context
	var cancel context.CancelFunc
	if options.Time > 0 {
		ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(time.Duration(options.Time)*time.Second))
		defer cancel()
	} else {
		ctx = context.Background()
	}
	srv.Run(ctx)
	return nil
}
