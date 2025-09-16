package server

import (
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestStatsEndpoint verifies the enhanced stats endpoint
func TestStatsEndpoint(t *testing.T) {
	logger := testLogger()
	rng := rand.New(rand.NewSource(12345))

	t.Run("stats with no hand limit", func(t *testing.T) {
		// Create server with unlimited hands
		server := NewServer(logger, rng)

		req := httptest.NewRequest(http.MethodGet, "/stats", nil)
		recorder := httptest.NewRecorder()

		server.handleStats(recorder, req)

		if recorder.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", recorder.Code)
		}

		body := recorder.Body.String()
		t.Logf("Stats output (unlimited): %s", body)

		// Should contain basic info
		if !strings.Contains(body, "Connected bots: 0") {
			t.Errorf("Expected 'Connected bots: 0', got: %s", body)
		}
		if !strings.Contains(body, "Hands completed: 0") {
			t.Errorf("Expected 'Hands completed: 0', got: %s", body)
		}
		if !strings.Contains(body, "Hand limit: unlimited") {
			t.Errorf("Expected 'Hand limit: unlimited', got: %s", body)
		}
	})

	t.Run("stats with hand limit", func(t *testing.T) {
		// Create server with hand limit
		server := NewServerWithHandLimit(logger, rng, 10)

		// Simulate some hands completed
		server.pool.handCounter = 3

		req := httptest.NewRequest(http.MethodGet, "/stats", nil)
		recorder := httptest.NewRecorder()

		server.handleStats(recorder, req)

		if recorder.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", recorder.Code)
		}

		body := recorder.Body.String()
		t.Logf("Stats output (limited): %s", body)

		// Should contain hand limit info
		if !strings.Contains(body, "Connected bots: 0") {
			t.Errorf("Expected 'Connected bots: 0', got: %s", body)
		}
		if !strings.Contains(body, "Hands completed: 3") {
			t.Errorf("Expected 'Hands completed: 3', got: %s", body)
		}
		if !strings.Contains(body, "Hand limit: 10") {
			t.Errorf("Expected 'Hand limit: 10', got: %s", body)
		}
		if !strings.Contains(body, "Hands remaining: 7") {
			t.Errorf("Expected 'Hands remaining: 7', got: %s", body)
		}
	})

	t.Run("stats when limit reached", func(t *testing.T) {
		// Create server with hand limit
		server := NewServerWithHandLimit(logger, rng, 5)

		// Simulate hand limit reached
		server.pool.handCounter = 5

		req := httptest.NewRequest(http.MethodGet, "/stats", nil)
		recorder := httptest.NewRecorder()

		server.handleStats(recorder, req)

		if recorder.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", recorder.Code)
		}

		body := recorder.Body.String()
		t.Logf("Stats output (limit reached): %s", body)

		// Should show 0 hands remaining
		if !strings.Contains(body, "Hands completed: 5") {
			t.Errorf("Expected 'Hands completed: 5', got: %s", body)
		}
		if !strings.Contains(body, "Hand limit: 5") {
			t.Errorf("Expected 'Hand limit: 5', got: %s", body)
		}
		if !strings.Contains(body, "Hands remaining: 0") {
			t.Errorf("Expected 'Hands remaining: 0', got: %s", body)
		}
	})
}
