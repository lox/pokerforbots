package auth

import "context"

// SimpleIdentity is a simple struct for returning auth identity.
// This avoids circular imports between internal/auth and internal/server.
type SimpleIdentity struct {
	BotID   string
	BotName string
	OwnerID string
}

// Adapter wraps a Validator to return interface{} for compatibility with server.AuthValidator.
type Adapter struct {
	validator Validator
}

// NewAdapter creates an adapter that wraps a Validator.
func NewAdapter(validator Validator) *Adapter {
	return &Adapter{validator: validator}
}

// Validate implements the server.AuthValidator interface.
func (a *Adapter) Validate(ctx context.Context, token string) (interface{}, error) {
	identity, err := a.validator.Validate(ctx, token)
	if err != nil {
		return nil, err
	}
	if identity == nil {
		return nil, nil
	}
	// Convert to SimpleIdentity for easy extraction by caller
	return &SimpleIdentity{
		BotID:   identity.BotID,
		BotName: identity.BotName,
		OwnerID: identity.OwnerID,
	}, nil
}
