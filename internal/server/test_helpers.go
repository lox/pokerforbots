package server

import (
	"io"

	"github.com/rs/zerolog"
)

// testLogger creates a logger that discards output for tests
func testLogger() zerolog.Logger {
	return zerolog.New(io.Discard).Level(zerolog.Disabled)
}
