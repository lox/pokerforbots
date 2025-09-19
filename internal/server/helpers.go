package server

import (
	"context"
	"net/http"
	"time"
)

// WaitForHealthy polls the /health endpoint until it returns 200 OK or the context is cancelled.
// baseURL should be the server's base URL (e.g., "http://localhost:8080").
func WaitForHealthy(ctx context.Context, baseURL string) error {
	healthURL := baseURL + "/health"
	client := &http.Client{Timeout: 1 * time.Second}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			resp, err := client.Get(healthURL)
			if err == nil && resp.StatusCode == http.StatusOK {
				resp.Body.Close()
				return nil
			}
			if resp != nil {
				resp.Body.Close()
			}
		}
	}
}
