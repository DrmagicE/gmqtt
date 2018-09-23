package logger

import (
	"io"
	"log"
	"os"
)

type Logger struct {
	log *log.Logger
}

func NewLogger(stdout io.Writer, prefix string, flag int) *Logger {
	if stdout == nil {
		stdout = os.Stderr
	}
	return &Logger{log.New(stdout, prefix, flag)}
}

func (l *Logger) Print(v ...interface{}) {
	if l.log != nil {
		l.log.Print(v...)
	}
}

func (l *Logger) Println(v ...interface{}) {
	if l.log != nil {
		l.log.Println(v...)
	}
}

func (l *Logger) Printf(format string, v ...interface{}) {
	if l.log != nil {
		l.log.Printf(format, v...)
	}
}
