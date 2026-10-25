// Package checkplayerjoin decides whether a Session accepts new players.
package checkplayerjoin

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/monitoring"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/sessionstatus"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/usecases/checkplayerjoin/internal/repo"
)

//go:generate mockgen -package=checkplayerjoin -destination=service_mock_test.go . sessionFinder

// sessionFinder reads the Session facts the decision needs.
type sessionFinder interface {
	FindSession(ctx context.Context, sessionUUID string) (*internalrepo.Session, error)
}

// Service is the player-join eligibility use case.
type Service struct {
	sessions sessionFinder
	now      func() time.Time
}

// New constructs a Service reading from db.
func New(db *gorm.DB) *Service {
	return &Service{sessions: internalrepo.New(db), now: time.Now}
}

// EligibleToJoinAsPlayer reports whether sessionUUID accepts new players: it
// exists and is in a LOBBY whose expiry has not passed. Any user may join.
func (s *Service) EligibleToJoinAsPlayer(ctx context.Context, sessionUUID string) (sessionpkg.PlayerJoinEligibility, error) {
	if _, err := uuid.Parse(sessionUUID); err != nil {
		return sessionpkg.PlayerJoinSessionNotFound, nil
	}
	session, err := s.sessions.FindSession(ctx, sessionUUID)
	if err != nil {
		return "", err
	}
	if session == nil {
		return sessionpkg.PlayerJoinSessionNotFound, nil
	}

	switch sessionstatus.Status(session.Status) {
	case sessionstatus.Lobby:
		// An overdue lobby has expired even if nothing recorded it yet.
		if !s.now().Before(session.LobbyExpiresAt) {
			return sessionpkg.PlayerJoinSessionNotJoinable, nil
		}
		return sessionpkg.PlayerJoinEligible, nil
	case sessionstatus.Running:
		return sessionpkg.PlayerJoinSessionStarted, nil
	case sessionstatus.Finished, sessionstatus.Cancelled, sessionstatus.Expired, sessionstatus.Aborted:
		return sessionpkg.PlayerJoinSessionNotJoinable, nil
	default:
		monitoring.Alert(ctx, "play session has an unknown status")
		return "", fmt.Errorf("session %s has unknown status %q", sessionUUID, session.Status)
	}
}
