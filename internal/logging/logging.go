package logging

import (
	"log"
	"os"
)

var logger *log.Logger

func GetLogger() *log.Logger {
	if logger == nil {
		logger = log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)
	}

	return logger
}

// SetLogger sets the logger facilities (mainly for testing)
func SetLogger(newLogger *log.Logger) {
	logger = newLogger
}
