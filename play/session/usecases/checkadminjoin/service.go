// Package checkadminjoin decides whether a user may open an admin connection
// to a Session.
package checkadminjoin

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/sessionstatus"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/usecases/checkadminjoin/internal/repo"
)

//go:generate mockgen -package=checkadminjoin -destination=service_mock_test.go . sessionFinder

// sessionFinder reads the Session facts the decision needs.
type sessionFinder interface {
	FindSession(ctx context.Context, sessionUUID string) (*internalrepo.Session, error)
}

// Service is the admin-join eligibility use case.
type Service struct {
	sessions sessionFinder
	now      func() time.Time
}

// New constructs a Service reading from db.
func New(db *gorm.DB) *Service {
	return &Service{sessions: internalrepo.New(db), now: time.Now}
}

// EligibleToJoinAsAdmin reports whether userUUID may join sessionUUID as
// admin: the Session exists, userUUID created it, and it is in LOBBY (with its
// lobby not yet expired) or RUNNING.
func (s *Service) EligibleToJoinAsAdmin(ctx context.Context, sessionUUID string, userUUID string) (sessionpkg.AdminJoinEligibility, error) {
	if _, err := uuid.Parse(sessionUUID); err != nil {
		return sessionpkg.AdminJoinSessionNotFound, nil
	}
	session, err := s.sessions.FindSession(ctx, sessionUUID)
	if err != nil {
		return "", err
	}
	if session == nil {
		return sessionpkg.AdminJoinSessionNotFound, nil
	}
	if !sameUUID(session.CreatedByUserUUID, userUUID) {
		return sessionpkg.AdminJoinNotCreator, nil
	}

	switch sessionstatus.Status(session.Status) {
	case sessionstatus.Running:
		return sessionpkg.AdminJoinEligible, nil
	case sessionstatus.Lobby:
		// An overdue lobby has expired even if nothing recorded it yet.
		if !s.now().Before(session.LobbyExpiresAt) {
			return sessionpkg.AdminJoinSessionNotJoinable, nil
		}
		return sessionpkg.AdminJoinEligible, nil
	default:
		return sessionpkg.AdminJoinSessionNotJoinable, nil
	}
}

// sameUUID compares two UUIDs regardless of their textual form.
func sameUUID(a string, b string) bool {
	parsedA, errA := uuid.Parse(a)
	parsedB, errB := uuid.Parse(b)
	return errA == nil && errB == nil && parsedA == parsedB
}
