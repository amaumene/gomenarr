package utils

import (
	"os"

	"github.com/sirupsen/logrus"
)

// NewLogger creates a new configured logger
func NewLogger(level string) *logrus.Logger {
	logger := logrus.New()
	logger.SetOutput(os.Stdout)
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	// Parse log level
	logLevel, err := logrus.ParseLevel(level)
	if err != nil {
		logLevel = logrus.InfoLevel
	}
	logger.SetLevel(logLevel)

	return logger
}
