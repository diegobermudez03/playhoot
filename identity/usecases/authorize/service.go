// Package authorize is Identity's capability for turning a caller's
// credential into a UserUUID and deciding whether that caller may perform a
// coarse operation.
//
// This is a placeholder: it does not read the token at all. It is here so
// the callers of this capability can be built and wired before Identity's
// real behavior exists.
package authorize

import (
	"context"

	"github.com/google/uuid"

	"github.com/diegobermudez03/playhoot/identity"
)

// Service is Identity's authorization use case.
type Service struct{}

// New constructs a Service.
func New() *Service {
	return &Service{}
}

// Authorize answers who the caller is and whether they may perform
// operation.
//
// For now it ignores token and operation, allows everything, and returns a
// fresh random UserUUID on every call, so the same caller is never
// recognized twice.
//
// Eventually the token (a JWT) is validated and resolved to the caller's
// stable UserUUID, and each operation is allowed or forbidden by what that
// User may do. In particular, identity.OperationCreateSession is expected to
// require an actual account: a guest user must be answered with
// identity.AccessForbidden, not allowed.
func (s *Service) Authorize(_ context.Context, _ identity.AccessToken, _ identity.Operation) (identity.UserUUID, identity.AccessDecision, error) {
	return identity.UserUUID(uuid.NewString()), identity.AccessAllowed, nil
}
