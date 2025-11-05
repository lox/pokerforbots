package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPValidator_ValidToken(t *testing.T) {
	// Mock auth server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req validateRequest
		json.NewDecoder(r.Body).Decode(&req)

		if req.Token == "valid-token" {
			json.NewEncoder(w).Encode(validateResponse{
				Valid:   true,
				BotID:   "bot-123",
				BotName: "test-bot",
				OwnerID: "github:456",
			})
		} else {
			json.NewEncoder(w).Encode(validateResponse{Valid: false})
		}
	}))
	defer server.Close()

	validator := NewHTTPValidator(server.URL, "")

	// Valid token
	identity, err := validator.Validate(context.Background(), "valid-token")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if identity.BotID != "bot-123" {
		t.Errorf("expected bot-123, got %s", identity.BotID)
	}
	if identity.BotName != "test-bot" {
		t.Errorf("expected test-bot, got %s", identity.BotName)
	}
	if identity.OwnerID != "github:456" {
		t.Errorf("expected github:456, got %s", identity.OwnerID)
	}
}

func TestHTTPValidator_InvalidToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(validateResponse{Valid: false})
	}))
	defer server.Close()

	validator := NewHTTPValidator(server.URL, "")
	_, err := validator.Validate(context.Background(), "invalid-token")

	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
}

func TestHTTPValidator_EmptyToken(t *testing.T) {
	validator := NewHTTPValidator("http://localhost:9999", "")
	_, err := validator.Validate(context.Background(), "")

	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken for empty token, got %v", err)
	}
}

func TestHTTPValidator_StatusCodes(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantErr    error
	}{
		{"unauthorized", http.StatusUnauthorized, ErrInvalidToken},
		{"forbidden", http.StatusForbidden, ErrInvalidToken},
		{"rate limited", http.StatusTooManyRequests, ErrUnavailable},
		{"server error", http.StatusInternalServerError, ErrUnavailable},
		{"bad gateway", http.StatusBadGateway, ErrUnavailable},
		{"service unavailable", http.StatusServiceUnavailable, ErrUnavailable},
		{"unexpected", http.StatusTeapot, ErrUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			validator := NewHTTPValidator(server.URL, "")
			_, err := validator.Validate(context.Background(), "token")

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("expected %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestHTTPValidator_Timeout(t *testing.T) {
	// Slow server that takes 2 seconds
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		json.NewEncoder(w).Encode(validateResponse{Valid: true})
	}))
	defer server.Close()

	validator := NewHTTPValidator(server.URL, "")
	_, err := validator.Validate(context.Background(), "token")

	// Should timeout (500ms) and return ErrUnavailable
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("expected ErrUnavailable on timeout, got %v", err)
	}
}

func TestHTTPValidator_AdminSecret(t *testing.T) {
	var receivedSecret string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSecret = r.Header.Get("X-Admin-Secret")
		json.NewEncoder(w).Encode(validateResponse{Valid: true, BotID: "test"})
	}))
	defer server.Close()

	validator := NewHTTPValidator(server.URL, "my-secret")
	validator.Validate(context.Background(), "token")

	if receivedSecret != "my-secret" {
		t.Errorf("expected admin secret 'my-secret', got '%s'", receivedSecret)
	}
}

func TestHTTPValidator_NoAdminSecret(t *testing.T) {
	var receivedSecret string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSecret = r.Header.Get("X-Admin-Secret")
		json.NewEncoder(w).Encode(validateResponse{Valid: true, BotID: "test"})
	}))
	defer server.Close()

	validator := NewHTTPValidator(server.URL, "")
	validator.Validate(context.Background(), "token")

	if receivedSecret != "" {
		t.Errorf("expected no admin secret, got '%s'", receivedSecret)
	}
}

func TestHTTPValidator_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	validator := NewHTTPValidator(server.URL, "")
	_, err := validator.Validate(context.Background(), "token")

	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("expected ErrUnavailable for malformed JSON, got %v", err)
	}
}

func TestHTTPValidator_NetworkError(t *testing.T) {
	// Point to non-existent server
	validator := NewHTTPValidator("http://localhost:1", "")
	_, err := validator.Validate(context.Background(), "token")

	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("expected ErrUnavailable for network error, got %v", err)
	}
}

func TestNoopValidator(t *testing.T) {
	validator := NewNoopValidator()
	identity, err := validator.Validate(context.Background(), "any-token")
	if err != nil {
		t.Fatalf("noop validator should never error: %v", err)
	}
	if identity != nil {
		t.Error("noop validator should return nil identity")
	}
}

func TestNoopValidator_EmptyToken(t *testing.T) {
	validator := NewNoopValidator()
	identity, err := validator.Validate(context.Background(), "")
	if err != nil {
		t.Fatalf("noop validator should never error, even with empty token: %v", err)
	}
	if identity != nil {
		t.Error("noop validator should return nil identity")
	}
}
