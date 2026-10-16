// Package authorize resolves a caller's credential to a UserUUID and decides
// whether they may perform a coarse operation.
//
// Placeholder: it only recognizes fixed test tokens and validates nothing.
package authorize

import (
	"context"

	"github.com/diegobermudez03/playhoot/identity"
)

// testUser is what a hardcoded test token stands for.
type testUser struct {
	userUUID string
	guest    bool
}

// testTokens maps each accepted test token to a fixed user, so repeated
// requests with the same token are the same caller (which is what makes
// idempotency testable by hand).
var testTokens = map[string]testUser{
	"test-user-1": {userUUID: "aaaaaaaa-aaaa-4aaa-8aaa-000000000001"},
	"test-user-2": {userUUID: "aaaaaaaa-aaaa-4aaa-8aaa-000000000002"},
	"test-guest":  {userUUID: "aaaaaaaa-aaaa-4aaa-8aaa-0000000000ff", guest: true},
}

// accountOnlyOperations are the operations a guest-only identity may never
// perform.
var accountOnlyOperations = map[identity.Operation]bool{
	identity.OperationCreateSession:      true,
	identity.OperationJoinSessionAsAdmin: true,
}

// Service is Identity's authorization use case.
type Service struct{}

// New constructs a Service.
func New() *Service {
	return &Service{}
}

// Authorize returns the caller's UserUUID and whether they may perform
// operation. Account-only operations are forbidden to guests.
//
// Placeholder: only the fixed test tokens are known; any other is
// unauthenticated.
func (s *Service) Authorize(_ context.Context, token string, operation identity.Operation) (string, identity.AccessDecision, error) {
	user, ok := testTokens[token]
	if !ok {
		return "", identity.AccessUnauthenticated, nil
	}
	if user.guest && accountOnlyOperations[operation] {
		return user.userUUID, identity.AccessForbidden, nil
	}
	return user.userUUID, identity.AccessAllowed, nil
}
