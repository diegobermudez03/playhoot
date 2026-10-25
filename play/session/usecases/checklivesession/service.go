// Package checklivesession decides whether a Session can have live
// connections at all, whoever asks.
package checklivesession

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/monitoring"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/sessionstatus"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/usecases/checklivesession/internal/repo"
)

//go:generate mockgen -package=checklivesession -destination=service_mock_test.go . sessionFinder

var (
	checkOutcomeLabel = metrics.NewLabel("outcome",
		string(sessionpkg.LiveSessionLive),
		string(sessionpkg.LiveSessionNotFound),
		string(sessionpkg.LiveSessionEnded),
		string(sessionpkg.LiveSessionLobbyExpired),
	)

	liveSessionChecks = metrics.NewCounter(
		"play_session_live_checks_total",
		"Sessions checked for live connections before one is opened, by outcome.",
		checkOutcomeLabel,
	)
)

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

// CheckLiveSession reports whether sessionUUID exists and is in LOBBY (with
// its lobby not yet expired) or RUNNING, and otherwise why not: a Session
// that ended comes with its terminal status and end cause. A sessionUUID that
// is not a UUID names no Session.
func (s *Service) CheckLiveSession(ctx context.Context, sessionUUID string) (sessionpkg.LiveSessionCheck, error) {
	check, err := s.check(ctx, sessionUUID)
	if err != nil {
		return sessionpkg.LiveSessionCheck{}, err
	}
	liveSessionChecks.Inc(ctx, checkOutcomeLabel.Value(string(check.Outcome)))
	return check, nil
}

func (s *Service) check(ctx context.Context, sessionUUID string) (sessionpkg.LiveSessionCheck, error) {
	if _, err := uuid.Parse(sessionUUID); err != nil {
		return sessionpkg.LiveSessionCheck{Outcome: sessionpkg.LiveSessionNotFound}, nil
	}
	session, err := s.sessions.FindSession(ctx, sessionUUID)
	if err != nil {
		return sessionpkg.LiveSessionCheck{}, err
	}
	if session == nil {
		return sessionpkg.LiveSessionCheck{Outcome: sessionpkg.LiveSessionNotFound}, nil
	}
	switch sessionstatus.Status(session.Status) {
	case sessionstatus.Running:
		return sessionpkg.LiveSessionCheck{Outcome: sessionpkg.LiveSessionLive}, nil
	case sessionstatus.Lobby:
		// An overdue lobby has expired even if nothing recorded it yet.
		if !s.now().Before(session.LobbyExpiresAt) {
			return sessionpkg.LiveSessionCheck{Outcome: sessionpkg.LiveSessionLobbyExpired}, nil
		}
		return sessionpkg.LiveSessionCheck{Outcome: sessionpkg.LiveSessionLive}, nil
	case sessionstatus.Finished, sessionstatus.Cancelled, sessionstatus.Expired, sessionstatus.Aborted:
		if session.EndCause == nil || *session.EndCause == "" {
			monitoring.Alert(ctx, "play session ended without an end cause")
			return sessionpkg.LiveSessionCheck{}, fmt.Errorf("session %s is %s without an end cause", sessionUUID, session.Status)
		}
		return sessionpkg.LiveSessionCheck{Outcome: sessionpkg.LiveSessionEnded, Status: session.Status, EndCause: *session.EndCause}, nil
	default:
		monitoring.Alert(ctx, "play session has an unknown status")
		return sessionpkg.LiveSessionCheck{}, fmt.Errorf("session %s has unknown status %q", sessionUUID, session.Status)
	}
}
