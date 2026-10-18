package sessionlifecycle

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/endcause"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/sessionstatus"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/sessionlifecycle/internal/repo"
	"github.com/diegobermudez03/playhoot/utils"
)

//go:generate mockgen -package=sessionlifecycle -destination=step_abort_start_mock_test.go . abortStartRepo

// abortStartRepo is AbortStart's own narrow persistence contract.
type abortStartRepo interface {
	LockSession(ctx context.Context, tx *gorm.DB, sessionUUID string) (*internalrepo.LockedSession, error)
	ChangeSessionStatus(ctx context.Context, tx *gorm.DB, sessionID uint, fromStatus string, toStatus string, endCause *string, changedBy *string, changedAt time.Time) error
	ReleaseJoinCodes(ctx context.Context, tx *gorm.DB, sessionID uint, releasedAt time.Time) error
}

var sessionStartAborts = metrics.NewCounter(
	"play_session_start_aborted_total",
	"Sessions aborted because their game failed to start.",
)

// AbortStart ends sessionUUID as ABORTED by a game start failure and releases
// its join code. A Session no longer in its lobby is left as it is.
func (m *Manager) AbortStart(ctx context.Context, sessionUUID string) error {
	defer logging.Step(ctx, "sessionlifecycle.abort_start").Close()

	if _, err := uuid.Parse(sessionUUID); err != nil {
		return sessionpkg.ErrSessionNotFound
	}
	aborted, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (bool, error) {
		return m.abortStartInTx(ctx, tx, sessionUUID)
	})
	if err != nil {
		return err
	}
	logging.LogFields(ctx, logging.Field("start_aborted", aborted))
	if aborted {
		sessionStartAborts.Inc(ctx)
	}
	return nil
}

// abortStartInTx reports whether it aborted the Session.
func (m *Manager) abortStartInTx(ctx context.Context, tx *gorm.DB, sessionUUID string) (bool, error) {
	session, err := m.abortStartRepo.LockSession(ctx, tx, sessionUUID)
	if err != nil {
		return false, err
	}
	if session == nil {
		return false, sessionpkg.ErrSessionNotFound
	}
	if sessionstatus.Status(session.Status) != sessionstatus.Lobby {
		return false, nil
	}
	now := m.now().UTC().Truncate(time.Microsecond)
	cause := string(endcause.GameStartFailure)
	// The platform aborts, not a user.
	if err := m.abortStartRepo.ChangeSessionStatus(ctx, tx, session.ID, string(sessionstatus.Lobby), string(sessionstatus.Aborted), &cause, nil, now); err != nil {
		return false, err
	}
	if err := m.abortStartRepo.ReleaseJoinCodes(ctx, tx, session.ID, now); err != nil {
		return false, err
	}
	return true, nil
}
