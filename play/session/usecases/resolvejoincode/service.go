// Package resolvejoincode finds the Session a join code names.
package resolvejoincode

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/sessionstatus"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/usecases/resolvejoincode/internal/repo"
)

//go:generate mockgen -package=resolvejoincode -destination=service_mock_test.go . joinCodeFinder

var (
	resolveOutcomeLabel = metrics.NewLabel("outcome",
		string(sessionpkg.JoinCodeFound),
		string(sessionpkg.JoinCodeInvalid),
		string(sessionpkg.JoinCodeExpired),
		string(sessionpkg.JoinCodeSessionStarted),
	)

	joinCodesResolved = metrics.NewCounter(
		"play_session_join_codes_resolved_total",
		"Join codes looked up to join a Session, by outcome.",
		resolveOutcomeLabel,
	)
)

// joinCodeFinder reads join code assignments.
type joinCodeFinder interface {
	FindLatestJoinCode(ctx context.Context, code string) (*internalrepo.JoinCode, error)
}

// Service is the join code resolution use case.
type Service struct {
	joinCodes joinCodeFinder
	now       func() time.Time
}

// New constructs a Service reading from db.
func New(db *gorm.DB) *Service {
	return &Service{joinCodes: internalrepo.New(db), now: time.Now}
}

// ResolveJoinCode returns the Session joinCode names when players can join it
// now: it is in its lobby and the code has not expired. Otherwise the outcome
// says why not. Codes are matched regardless of letter case.
func (s *Service) ResolveJoinCode(ctx context.Context, joinCode string) (sessionpkg.JoinCodeResolution, error) {
	found, err := s.joinCodes.FindLatestJoinCode(ctx, strings.ToUpper(joinCode))
	if err != nil {
		return sessionpkg.JoinCodeResolution{}, err
	}
	resolution := s.resolve(found)
	joinCodesResolved.Inc(ctx, resolveOutcomeLabel.Value(string(resolution.Outcome)))
	return resolution, nil
}

func (s *Service) resolve(found *internalrepo.JoinCode) sessionpkg.JoinCodeResolution {
	if found == nil {
		return sessionpkg.JoinCodeResolution{Outcome: sessionpkg.JoinCodeInvalid}
	}
	switch sessionstatus.Status(found.SessionStatus) {
	case sessionstatus.Lobby:
		// Only the lobby is decided by the code's expiry: a code can expire
		// while its Session is still being played.
		if !s.now().Before(found.ExpiresAt) {
			return sessionpkg.JoinCodeResolution{Outcome: sessionpkg.JoinCodeExpired}
		}
		return sessionpkg.JoinCodeResolution{Outcome: sessionpkg.JoinCodeFound, SessionUUID: found.SessionUUID}
	case sessionstatus.Finished, sessionstatus.Cancelled, sessionstatus.Expired, sessionstatus.Aborted:
		return sessionpkg.JoinCodeResolution{Outcome: sessionpkg.JoinCodeInvalid}
	default:
		return sessionpkg.JoinCodeResolution{Outcome: sessionpkg.JoinCodeSessionStarted}
	}
}
