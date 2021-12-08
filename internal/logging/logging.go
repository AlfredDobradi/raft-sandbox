package logging

import (
	"log"
	"os"
)

var logger *log.Logger

// GetLogger returns the currently set logger facility.
func GetLogger() *log.Logger {
	if logger == nil {
		logger = log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)
	}

	return logger
}

// SetLogger sets the logger facility.
func SetLogger(newLogger *log.Logger) {
	logger = newLogger
}
