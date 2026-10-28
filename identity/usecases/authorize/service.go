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

// devTestUsers are fixed account tokens accepted only when the development
// bypass is on, until accounts can sign in locally.
var devTestUsers = map[string]identity.User{
	"test-user-1": {UUID: "aaaaaaaa-aaaa-4aaa-8aaa-000000000001", DisplayName: "Test user 1", Level: identity.LevelAccount},
	"test-user-2": {UUID: "aaaaaaaa-aaaa-4aaa-8aaa-000000000002", DisplayName: "Test user 2", Level: identity.LevelAccount},
}

// Service is Identity's authorization use case.
type Service struct {
	access       accessVerifier
	devTestUsers bool
	now          func() time.Time
}

// New constructs a Service verifying access tokens signed with accessKey.
// devTestUsers also accepts the fixed test account tokens; never in
// production.
func New(accessKey []byte, devTestUsers bool) (*Service, error) {
	signer, err := accesstoken.NewSigner(accessKey)
	if err != nil {
		return nil, err
	}
	return &Service{access: signer, devTestUsers: devTestUsers, now: time.Now}, nil
}

// Authorize returns the caller's User and whether their level reaches what
// operation requires in the catalog; an uncataloged operation is forbidden.
// It reads nothing: the access token carries the User until it expires.
func (s *Service) Authorize(ctx context.Context, token string, operation identity.Operation) (identity.User, identity.AccessDecision, error) {
	caller, ok := s.devTestUser(token)
	if !ok {
		verified, err := s.access.Verify(token, s.now())
		if err != nil {
			return identity.User{}, identity.AccessUnauthenticated, nil
		}
		caller = verified
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

// devTestUser returns the fixed test account of token when the development
// bypass is on.
func (s *Service) devTestUser(token string) (identity.User, bool) {
	if !s.devTestUsers {
		return identity.User{}, false
	}
	user, ok := devTestUsers[token]
	return user, ok
}
