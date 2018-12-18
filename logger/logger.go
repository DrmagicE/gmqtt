package logger

import (
	"io"
	"log"
	"os"
)

// Logger is mainly used to print in debug mode
type Logger struct {
	log *log.Logger
}

// NewLogger returns a Logger
func NewLogger(stdout io.Writer, prefix string, flag int) *Logger {
	if stdout == nil {
		stdout = os.Stderr
	}
	return &Logger{log.New(stdout, prefix, flag)}
}

// Print
func (l *Logger) Print(v ...interface{}) {
	if l.log != nil {
		l.log.Print(v...)
	}
}

// Println
func (l *Logger) Println(v ...interface{}) {
	if l.log != nil {
		l.log.Println(v...)
	}
}

// Printf
func (l *Logger) Printf(format string, v ...interface{}) {
	if l.log != nil {
		l.log.Printf(format, v...)
	}
}
