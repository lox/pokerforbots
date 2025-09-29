package shared

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
)

// SetupSignalHandler creates a context that is cancelled on interrupt signals
func SetupSignalHandler() context.Context {
	ctx, cancel := context.WithCancel(context.Background())

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		cancel()
	}()

	return ctx
}

// SetupSignalHandlerWithLogger creates a context that is cancelled on interrupt signals and logs
func SetupSignalHandlerWithLogger(logger zerolog.Logger) context.Context {
	ctx, cancel := context.WithCancel(context.Background())

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		logger.Info().Str("signal", sig.String()).Msg("Received signal, shutting down gracefully")
		cancel()
	}()

	return ctx
}
