// Package loadsessionlimits reads the platform-time deadlines of a Session,
// which its live controller enforces.
package loadsessionlimits

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/usecases/loadsessionlimits/internal/repo"
)

//go:generate mockgen -package=loadsessionlimits -destination=service_mock_test.go . limitsFinder

// limitsFinder reads a Session's deadlines.
type limitsFinder interface {
	FindSessionLimits(ctx context.Context, sessionUUID string) (*internalrepo.Limits, error)
}

// Service is the Session limits use case.
type Service struct {
	limits limitsFinder
}

// New constructs a Service reading from db.
func New(db *gorm.DB) *Service {
	return &Service{limits: internalrepo.New(db)}
}

// LoadSessionLimits returns sessionUUID's deadlines, or
// sessionpkg.ErrSessionNotFound when there is no such Session.
func (s *Service) LoadSessionLimits(ctx context.Context, sessionUUID string) (sessionpkg.SessionLimits, error) {
	if _, err := uuid.Parse(sessionUUID); err != nil {
		return sessionpkg.SessionLimits{}, sessionpkg.ErrSessionNotFound
	}
	found, err := s.limits.FindSessionLimits(ctx, sessionUUID)
	if err != nil {
		return sessionpkg.SessionLimits{}, err
	}
	if found == nil {
		return sessionpkg.SessionLimits{}, sessionpkg.ErrSessionNotFound
	}
	return sessionpkg.SessionLimits{LobbyExpiresAt: found.LobbyExpiresAt.UTC(), ExpiresAt: found.ExpiresAt.UTC()}, nil
}
