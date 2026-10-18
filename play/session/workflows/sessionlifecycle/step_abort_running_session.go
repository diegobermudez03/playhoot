package sessionlifecycle

import (
	"context"
	"fmt"
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

//go:generate mockgen -package=sessionlifecycle -destination=step_abort_running_session_mock_test.go . abortRunningSessionRepo

// abortRunningSessionRepo is AbortRunningSession's own narrow persistence
// contract.
type abortRunningSessionRepo interface {
	LockSession(ctx context.Context, tx *gorm.DB, sessionUUID string) (*internalrepo.LockedSession, error)
	ChangeSessionStatus(ctx context.Context, tx *gorm.DB, sessionID uint, fromStatus string, toStatus string, endCause *string, changedBy *string, changedAt time.Time) error
}

// abortCauses maps each cause the Runtime may abort for to its end cause.
var abortCauses = map[sessionpkg.AbortCause]endcause.Cause{
	sessionpkg.AbortScriptFailure:             endcause.ScriptFailure,
	sessionpkg.AbortUnrecoverableInfraFailure: endcause.UnrecoverableInfraFailure,
}

var (
	abortCauseLabel = metrics.NewLabel("cause",
		string(sessionpkg.AbortScriptFailure),
		string(sessionpkg.AbortUnrecoverableInfraFailure),
	)

	runningSessionAborts = metrics.NewCounter(
		"play_session_running_aborted_total",
		"Running Sessions aborted because the platform could not continue them, by cause.",
		abortCauseLabel,
	)
)

// AbortRunningSession ends sessionUUID as ABORTED for cause, as the platform.
// A Session that is not RUNNING is left as it is.
func (m *Manager) AbortRunningSession(ctx context.Context, sessionUUID string, cause sessionpkg.AbortCause) error {
	defer logging.Step(ctx, "sessionlifecycle.abort_running_session").Close()

	if _, err := uuid.Parse(sessionUUID); err != nil {
		return sessionpkg.ErrSessionNotFound
	}
	endCause, known := abortCauses[cause]
	if !known {
		return fmt.Errorf("unknown abort cause %q", cause)
	}
	aborted, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (bool, error) {
		return m.abortRunningSessionInTx(ctx, tx, sessionUUID, endCause)
	})
	if err != nil {
		return err
	}
	logging.LogFields(ctx, logging.Field("running_session_aborted", aborted))
	if aborted {
		runningSessionAborts.Inc(ctx, abortCauseLabel.Value(string(cause)))
	}
	return nil
}

// abortRunningSessionInTx reports whether it aborted the Session.
func (m *Manager) abortRunningSessionInTx(ctx context.Context, tx *gorm.DB, sessionUUID string, endCause endcause.Cause) (bool, error) {
	session, err := m.abortRunningSessionRepo.LockSession(ctx, tx, sessionUUID)
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
	cause := string(endCause)
	// The platform aborts, not a user.
	if err := m.abortRunningSessionRepo.ChangeSessionStatus(ctx, tx, session.ID, string(sessionstatus.Running), string(sessionstatus.Aborted), &cause, nil, now); err != nil {
		return false, err
	}
	return true, nil
}
