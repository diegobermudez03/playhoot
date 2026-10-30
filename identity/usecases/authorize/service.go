// Package authorize resolves a caller's access token to a User and decides
// whether they may perform a coarse operation.
package authorize

import (
	"context"
	"time"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/business/accesstoken"
	"github.com/diegobermudez03/playhoot/identity/internal/business/operationlevel"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
)

// accessVerifier verifies access tokens.
type accessVerifier interface {
	Verify(token string, now time.Time) (identity.User, error)
}

// Service is Identity's authorization use case.
type Service struct {
	access accessVerifier
	now    func() time.Time
}

// New constructs a Service verifying access tokens signed with accessKey.
func New(accessKey []byte) (*Service, error) {
	signer, err := accesstoken.NewSigner(accessKey)
	if err != nil {
		return nil, err
	}
	return &Service{access: signer, now: time.Now}, nil
}

// Authorize returns the caller's User and whether their level reaches what
// operation requires in the catalog; an uncataloged operation is forbidden.
// It reads nothing: the access token carries the User until it expires.
func (s *Service) Authorize(ctx context.Context, token string, operation identity.Operation) (identity.User, identity.AccessDecision, error) {
	caller, err := s.access.Verify(token, s.now())
	if err != nil {
		return identity.User{}, identity.AccessUnauthenticated, nil
	}
	required, known := operationlevel.Required(operation)
	if !known {
		logging.LogFields(ctx, logging.Field("uncataloged_operation", string(operation)))
		monitoring.Alert(ctx, "authorize: operation without a required level in the catalog")
		return caller, identity.AccessForbidden, nil
	}
	if !operationlevel.Satisfies(caller.Level, required) {
		return caller, identity.AccessForbidden, nil
	}
	return caller, identity.AccessAllowed, nil
}
