// Package createguest creates guest Users: people playing without an account,
// who get a bearer credential of their own.
package createguest

import (
	"context"

	"github.com/diegobermudez03/playhoot/identity"
)

// testGuest is the hardcoded guest the authorization placeholder recognizes.
var testGuest = identity.CreateGuestResult{
	Outcome:  identity.GuestCreated,
	UserUUID: "aaaaaaaa-aaaa-4aaa-8aaa-0000000000ff",
	Token:    "test-guest",
}

// Service is Identity's guest creation use case.
type Service struct{}

// New constructs a Service.
func New() *Service {
	return &Service{}
}

// CreateGuest creates a guest User named displayName, idempotently per
// idempotencyKey, and returns their credential. A reused key with another
// name returns identity.ErrIdempotencyConflict.
//
// Placeholder: returns the same test guest for every call, storing nothing.
func (s *Service) CreateGuest(_ context.Context, _ string, _ string) (identity.CreateGuestResult, error) {
	return testGuest, nil
}
