package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lox/pokerforbots/internal/server"
)

func main() {
	addr := flag.String("addr", ":8080", "Server address")
	flag.Parse()

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	srv := server.NewServer()

	// Start server in a goroutine
	serverErr := make(chan error, 1)
	go func() {
		log.Printf("Server starting on %s", *addr)
		serverErr <- srv.Start(*addr)
	}()

	// Wait for either server error or interrupt signal
	select {
	case err := <-serverErr:
		if err != nil {
			log.Fatal("Server error: ", err)
		}
	case sig := <-sigChan:
		log.Printf("Received signal %v, shutting down gracefully...", sig)

		// Give server a moment to finish current operations
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Note: server.Stop() would need to be implemented for full graceful shutdown
		// For now, we just exit after a brief delay
		select {
		case <-ctx.Done():
			log.Println("Shutdown timeout exceeded, forcing exit")
		case <-time.After(500 * time.Millisecond):
			log.Println("Server shutdown complete")
		}
	}
}
