// Package checklivesession decides whether a Session can have live
// connections at all, whoever asks.
package checklivesession

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/play/session/internal/business/sessionstatus"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/usecases/checklivesession/internal/repo"
)

//go:generate mockgen -package=checklivesession -destination=service_mock_test.go . sessionFinder

// sessionFinder reads the Session facts the decision needs.
type sessionFinder interface {
	FindSession(ctx context.Context, sessionUUID string) (*internalrepo.Session, error)
}

// Service is the live-Session check use case.
type Service struct {
	sessions sessionFinder
	now      func() time.Time
}

// New constructs a Service reading from db.
func New(db *gorm.DB) *Service {
	return &Service{sessions: internalrepo.New(db), now: time.Now}
}

// IsSessionLive reports whether sessionUUID exists and is in LOBBY (with its
// lobby not yet expired) or RUNNING.
func (s *Service) IsSessionLive(ctx context.Context, sessionUUID string) (bool, error) {
	if _, err := uuid.Parse(sessionUUID); err != nil {
		return false, nil
	}
	session, err := s.sessions.FindSession(ctx, sessionUUID)
	if err != nil {
		return false, err
	}
	if session == nil {
		return false, nil
	}
	switch sessionstatus.Status(session.Status) {
	case sessionstatus.Running:
		return true, nil
	case sessionstatus.Lobby:
		// An overdue lobby has expired even if nothing recorded it yet.
		return s.now().Before(session.LobbyExpiresAt), nil
	default:
		return false, nil
	}
}
