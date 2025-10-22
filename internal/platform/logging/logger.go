package logging

import (
	"io"
	"os"
	"strings"

	"github.com/amaumene/gomenarr/internal/platform/config"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Setup initializes the global logger
func Setup(cfg config.LoggingConfig) {
	// Set log level
	level := parseLevel(cfg.Level)
	zerolog.SetGlobalLevel(level)

	// Set output
	var output io.Writer = os.Stdout
	if cfg.Output == "stderr" {
		output = os.Stderr
	}

	// Set format
	if cfg.Format == "console" {
		output = zerolog.ConsoleWriter{Out: output}
	}

	log.Logger = zerolog.New(output).With().Timestamp().Caller().Logger()
}

// parseLevel parses log level from string
func parseLevel(level string) zerolog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "fatal":
		return zerolog.FatalLevel
	case "panic":
		return zerolog.PanicLevel
	default:
		return zerolog.InfoLevel
	}
}

// NewLogger creates a new logger with component name
func NewLogger(component string) zerolog.Logger {
	return log.With().Str("component", component).Logger()
}
