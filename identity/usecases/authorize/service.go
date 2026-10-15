// Package authorize is Identity's capability for turning a caller's
// credential into a UserUUID and deciding whether that caller may perform a
// coarse operation.
//
// This is a placeholder: it does not validate tokens. It recognizes a fixed
// set of test tokens so the callers of this capability can be built, wired
// and tested by hand before Identity's real behavior exists.
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

// Service is Identity's authorization use case.
type Service struct{}

// New constructs a Service.
func New() *Service {
	return &Service{}
}

// Authorize answers who the caller is and whether they may perform
// operation.
//
// For now it only knows the hardcoded test tokens: any other token is
// unauthenticated, the guest test token is forbidden from creating Sessions,
// and every other operation is allowed.
//
// Eventually the token (a JWT) is validated and resolved to the caller's
// stable UserUUID, and each operation is allowed or forbidden by what that
// User may do. In particular, identity.OperationCreateSession is expected to
// require an actual account: a guest user must be answered with
// identity.AccessForbidden, not allowed.
func (s *Service) Authorize(_ context.Context, token string, operation identity.Operation) (string, identity.AccessDecision, error) {
	user, ok := testTokens[token]
	if !ok {
		return "", identity.AccessUnauthenticated, nil
	}
	if user.guest && operation == identity.OperationCreateSession {
		return user.userUUID, identity.AccessForbidden, nil
	}
	return user.userUUID, identity.AccessAllowed, nil
}
