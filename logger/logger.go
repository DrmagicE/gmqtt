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

// Print calls log.Print to print to the logger.
func (l *Logger) Print(v ...interface{}) {
	if l.log != nil {
		l.log.Print(v...)
	}
}

// Println calls log.Println to print to the logger.
func (l *Logger) Println(v ...interface{}) {
	if l.log != nil {
		l.log.Println(v...)
	}
}

// Printf calls log.Printf to print to the logger.
func (l *Logger) Printf(format string, v ...interface{}) {
	if l.log != nil {
		l.log.Printf(format, v...)
	}
}
