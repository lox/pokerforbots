package shared

import (
	"os"
	"time"

	"github.com/rs/zerolog"
)

// SetupLogger configures zerolog with pretty console output
func SetupLogger(debug bool) zerolog.Logger {
	level := zerolog.InfoLevel
	if debug {
		level = zerolog.DebugLevel
	}

	return zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).
		Level(level).
		With().
		Timestamp().
		Logger()
}

// SetupStructuredLogger configures zerolog for structured (JSON) output
func SetupStructuredLogger(debug bool) zerolog.Logger {
	zerolog.TimeFieldFormat = time.RFC3339Nano

	level := zerolog.InfoLevel
	if debug {
		level = zerolog.DebugLevel
	}

	return zerolog.New(os.Stderr).
		Level(level).
		With().
		Timestamp().
		Logger()
}
