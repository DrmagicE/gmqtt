package subscribe

import (
	"context"
	"flag"
	"time"
)

// Command represents the command executed by `go run sub_benchmark.go`
type Command struct {
	Server *Server
}

// Options represents the subscribe benchmark options
type Options struct {
	Host               string //default:  localhost
	Port               string //default: :1883
	Username           string
	Password           string
	Qos                int
	Topic              string
	ConnectionInterval int
	SubscribeInterval  int //
	Number             int //number of subscriptios per client
	Count              int //number of clients
	CleanSession       bool
	Time               int //time limit
}

//ParseFlags parses the args into Options.
func (cmd *Command) ParseFlags(args ...string) (Options, error) {
	var options Options
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	fs.StringVar(&options.Host, "h", "localhost", "host")
	fs.StringVar(&options.Port, "p", ":1883", "port")
	fs.StringVar(&options.Username, "u", "", "username")
	fs.StringVar(&options.Password, "pwd", "", "password")
	fs.IntVar(&options.Qos, "qos", 1, "qos")
	fs.StringVar(&options.Topic, "topic", "topic_name", "topic name prefix")
	fs.IntVar(&options.ConnectionInterval, "ci", 100, "connection interval (ms)")
	fs.IntVar(&options.SubscribeInterval, "i", 100, "subscribing interval (ms)")
	fs.IntVar(&options.Count, "c", 1000, "number of clients")
	fs.BoolVar(&options.CleanSession, "C", true, "clean session")
	fs.IntVar(&options.Number, "n", 200, "number of subscriptios to make per client")
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
