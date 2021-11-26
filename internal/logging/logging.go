package logging

import (
	"log"

	"gitlab.com/axdx/raft-sandbox/internal/config"
)

var logger *Logger

func GetLogger() *Logger {
	if logger == nil {
		logger = &Logger{}
	}

	return logger
}

type Logger struct{}

func (l *Logger) Log(str string) {
	if config.Debug() {
		log.Println(str)
	}
}
