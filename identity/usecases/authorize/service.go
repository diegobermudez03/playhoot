// Package authorize resolves a caller's credential to a User and decides
// whether they may perform a coarse operation.
//
// Placeholder: it only recognizes fixed test tokens and validates nothing.
package authorize

import (
	"context"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/business/operationlevel"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
)

// testUser is what a hardcoded test token stands for.
type testUser struct {
	user  identity.User
	level identity.Level
}

// testTokens maps each accepted test token to a fixed user, so repeated
// requests with the same token are the same caller (which is what makes
// idempotency testable by hand).
var testTokens = map[string]testUser{
	"test-user-1": {user: identity.User{UUID: "aaaaaaaa-aaaa-4aaa-8aaa-000000000001", DisplayName: "Test user 1"}, level: identity.LevelAccount},
	"test-user-2": {user: identity.User{UUID: "aaaaaaaa-aaaa-4aaa-8aaa-000000000002", DisplayName: "Test user 2"}, level: identity.LevelAccount},
	"test-guest":  {user: identity.User{UUID: "aaaaaaaa-aaaa-4aaa-8aaa-0000000000ff", DisplayName: "Guest"}, level: identity.LevelGuest},
}

// Service is Identity's authorization use case.
type Service struct{}

// New constructs a Service.
func New() *Service {
	return &Service{}
}

// Authorize returns the caller's User and whether their level reaches what
// operation requires in the catalog; an uncataloged operation is forbidden.
// Placeholder: only the fixed test tokens are known.
func (s *Service) Authorize(ctx context.Context, token string, operation identity.Operation) (identity.User, identity.AccessDecision, error) {
	known, ok := testTokens[token]
	if !ok {
		return identity.User{}, identity.AccessUnauthenticated, nil
	}
	required, ok := operationlevel.Required(operation)
	if !ok {
		logging.LogFields(ctx, logging.Field("uncataloged_operation", string(operation)))
		monitoring.Alert(ctx, "authorize: operation without a required level in the catalog")
		return known.user, identity.AccessForbidden, nil
	}
	if !operationlevel.Satisfies(known.level, required) {
		return known.user, identity.AccessForbidden, nil
	}
	return known.user, identity.AccessAllowed, nil
}
