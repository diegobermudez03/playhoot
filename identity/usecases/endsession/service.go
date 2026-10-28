// Package endsession signs a client out: its session can no longer renew.
// Access tokens already issued keep working until they expire: authorizing
// never reads the session.
package endsession

import (
	"context"
	"time"

	"gorm.io/gorm"

	internalrepo "github.com/diegobermudez03/playhoot/identity/usecases/endsession/internal/repo"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
)

//go:generate mockgen -package=endsession -destination=service_mock_test.go . sessionStore

// sessionStore deactivates sessions.
type sessionStore interface {
	DeactivateSessionOf(ctx context.Context, refreshToken string, endedAt time.Time) (bool, error)
}

var (
	outcomeLabel = metrics.NewLabel("outcome", "ended", "no_active_session")

	endsTotal = metrics.NewCounter(
		"identity_session_ends_total",
		"Sign-outs, by whether they ended an active session.",
		outcomeLabel,
	)
)

// Service is Identity's sign-out use case.
type Service struct {
	sessions sessionStore
	now      func() time.Time
}

// New constructs a Service on db.
func New(db *gorm.DB) *Service {
	return &Service{sessions: internalrepo.New(db), now: time.Now}
}

// EndSession ends the session refreshToken belongs to. Signing out twice, or
// with an unknown token, ends nothing and is not an error.
func (s *Service) EndSession(ctx context.Context, refreshToken string) error {
	ended, err := s.sessions.DeactivateSessionOf(ctx, refreshToken, s.now().UTC().Truncate(time.Microsecond))
	if err != nil {
		return err
	}
	if ended {
		endsTotal.Inc(ctx, outcomeLabel.Value("ended"))
	} else {
		endsTotal.Inc(ctx, outcomeLabel.Value("no_active_session"))
	}
	return nil
}
