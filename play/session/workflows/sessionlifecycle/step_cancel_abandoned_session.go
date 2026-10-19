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

//go:generate mockgen -package=sessionlifecycle -destination=step_cancel_abandoned_session_mock_test.go . cancelAbandonedSessionRepo

// cancelAbandonedSessionRepo is CancelAbandonedSession's own narrow
// persistence contract.
type cancelAbandonedSessionRepo interface {
	LockSession(ctx context.Context, tx *gorm.DB, sessionUUID string) (*internalrepo.LockedSession, error)
	ChangeSessionStatus(ctx context.Context, tx *gorm.DB, sessionID uint, fromStatus string, toStatus string, endCause *string, changedBy *string, changedAt time.Time) error
}

var abandonedSessionsCancelled = metrics.NewCounter(
	"play_session_abandoned_cancelled_total",
	"Running Sessions cancelled because none of their participants could play anymore.",
)

// CancelAbandonedSession ends sessionUUID as CANCELLED with ALL_PLAYERS_LEFT,
// as the platform, once none of its participants can play. A Session that is
// not RUNNING is left as it is.
func (m *Manager) CancelAbandonedSession(ctx context.Context, sessionUUID string) error {
	defer logging.Step(ctx, "sessionlifecycle.cancel_abandoned_session").Close()

	if _, err := uuid.Parse(sessionUUID); err != nil {
		return sessionpkg.ErrSessionNotFound
	}
	cancelled, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (bool, error) {
		return m.cancelAbandonedSessionInTx(ctx, tx, sessionUUID)
	})
	if err != nil {
		return err
	}
	logging.LogFields(ctx, logging.Field("abandoned_session_cancelled", cancelled))
	if cancelled {
		abandonedSessionsCancelled.Inc(ctx)
	}
	return nil
}

// cancelAbandonedSessionInTx reports whether it cancelled the Session.
func (m *Manager) cancelAbandonedSessionInTx(ctx context.Context, tx *gorm.DB, sessionUUID string) (bool, error) {
	session, err := m.cancelAbandonedSessionRepo.LockSession(ctx, tx, sessionUUID)
	if err != nil {
		return false, err
	}
	if session == nil {
		return false, sessionpkg.ErrSessionNotFound
	}
	if sessionstatus.Status(session.Status) != sessionstatus.Running {
		return false, nil
	}
	now := m.now().UTC().Truncate(time.Microsecond)
	cause := string(endcause.AllPlayersLeft)
	// The platform cancels, not a user.
	if err := m.cancelAbandonedSessionRepo.ChangeSessionStatus(ctx, tx, session.ID, string(sessionstatus.Running), string(sessionstatus.Cancelled), &cause, nil, now); err != nil {
		return false, err
	}
	return true, nil
}
