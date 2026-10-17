// Package checkadminjoin decides whether a user may open an admin connection
// to a Session.
//
// Placeholder: it does not look the Session up.
package checkadminjoin

import (
	"context"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

// Service is the admin-join eligibility use case.
type Service struct{}

// New constructs a Service.
func New() *Service {
	return &Service{}
}

// EligibleToJoinAsAdmin reports whether userUUID may join sessionUUID as
// admin: the Session exists, is in LOBBY or RUNNING, and userUUID created it.
//
// Placeholder: every user is eligible for every Session.
func (s *Service) EligibleToJoinAsAdmin(_ context.Context, _ string, _ string) (sessionpkg.AdminJoinEligibility, error) {
	return sessionpkg.AdminJoinEligible, nil
}
