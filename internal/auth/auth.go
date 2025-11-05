// Package auth provides optional external authentication for bot connections.
package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

var (
	// ErrInvalidToken indicates the token is definitively invalid.
	ErrInvalidToken = errors.New("auth: invalid token")

	// ErrUnavailable indicates the auth service is unreachable or unavailable.
	// Callers may choose to fail open (allow) or fail closed (reject).
	ErrUnavailable = errors.New("auth: unavailable")
)

// Identity represents an authenticated bot's identity.
type Identity struct {
	BotID   string `json:"bot_id"`
	BotName string `json:"bot_name"`
	OwnerID string `json:"owner_id"`
}

// Validator validates authentication tokens.
type Validator interface {
	// Validate checks if a token is valid and returns the bot identity.
	// Returns:
	//   - (*Identity, nil) if token is valid
	//   - (nil, ErrInvalidToken) if token is definitively invalid
	//   - (nil, ErrUnavailable) if auth service is unavailable
	//   - (nil, nil) if auth is disabled (NoopValidator only)
	Validate(ctx context.Context, token string) (*Identity, error)
}

// HTTPValidator validates tokens via HTTP callback to external service.
type HTTPValidator struct {
	url         string
	client      *http.Client
	adminSecret string
}

// NewHTTPValidator creates a validator that calls an external HTTP endpoint.
func NewHTTPValidator(url string, adminSecret string) *HTTPValidator {
	return &HTTPValidator{
		url:         url,
		adminSecret: adminSecret,
		client: &http.Client{
			Timeout: 500 * time.Millisecond, // Align with context timeout
		},
	}
}

type validateRequest struct {
	Token string `json:"token"`
}

type validateResponse struct {
	Valid   bool   `json:"valid"`
	BotID   string `json:"bot_id,omitempty"`
	BotName string `json:"bot_name,omitempty"`
	OwnerID string `json:"owner_id,omitempty"`
	Error   string `json:"error,omitempty"`
}

func (v *HTTPValidator) Validate(ctx context.Context, token string) (*Identity, error) {
	// Empty token is invalid when auth is enabled
	if token == "" {
		return nil, ErrInvalidToken
	}

	// Apply timeout via context
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	reqBody, err := json.Marshal(validateRequest{Token: token})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if v.adminSecret != "" {
		req.Header.Set("X-Admin-Secret", v.adminSecret)
	}

	resp, err := v.client.Do(req)
	if err != nil {
		// Network errors, timeouts, etc. = unavailable
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()

	// Handle HTTP status codes
	switch resp.StatusCode {
	case http.StatusOK:
		// Success - decode response
	case http.StatusUnauthorized, http.StatusForbidden:
		// Definitive rejection
		return nil, ErrInvalidToken
	case http.StatusTooManyRequests, http.StatusInternalServerError,
		http.StatusBadGateway, http.StatusServiceUnavailable:
		// Service issues
		return nil, fmt.Errorf("%w: status %d", ErrUnavailable, resp.StatusCode)
	default:
		// Treat unexpected status as unavailable
		return nil, fmt.Errorf("%w: unexpected status %d", ErrUnavailable, resp.StatusCode)
	}

	// Limit response body to 1MB to avoid pathological responses
	limitedReader := io.LimitReader(resp.Body, 1<<20)

	var authResp validateResponse
	if err := json.NewDecoder(limitedReader).Decode(&authResp); err != nil {
		return nil, fmt.Errorf("%w: decode error: %v", ErrUnavailable, err)
	}

	if !authResp.Valid {
		return nil, ErrInvalidToken
	}

	return &Identity{
		BotID:   authResp.BotID,
		BotName: authResp.BotName,
		OwnerID: authResp.OwnerID,
	}, nil
}

// NoopValidator allows all connections without validation (dev mode).
type NoopValidator struct{}

// NewNoopValidator creates a validator that allows all connections.
func NewNoopValidator() *NoopValidator {
	return &NoopValidator{}
}

func (v *NoopValidator) Validate(ctx context.Context, token string) (*Identity, error) {
	// No authentication - allow all connections
	return nil, nil
}
