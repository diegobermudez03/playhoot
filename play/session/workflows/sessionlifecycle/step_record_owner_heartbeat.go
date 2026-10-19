package sessionlifecycle

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/sessionstatus"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/sessionlifecycle/internal/repo"
	"github.com/diegobermudez03/playhoot/utils"
)

//go:generate mockgen -package=sessionlifecycle -destination=step_record_owner_heartbeat_mock_test.go . recordOwnerHeartbeatRepo

// recordOwnerHeartbeatRepo is RecordOwnerHeartbeat's own narrow persistence
// contract.
type recordOwnerHeartbeatRepo interface {
	LockSession(ctx context.Context, tx *gorm.DB, sessionUUID string) (*internalrepo.LockedSession, error)
	RenewOwnerLease(ctx context.Context, tx *gorm.DB, sessionID uint, expiresAt time.Time) error
}

var (
	heartbeatOutcomeLabel = metrics.NewLabel("outcome",
		string(sessionpkg.HeartbeatRecorded),
		string(sessionpkg.HeartbeatSessionNotRunning),
	)

	ownerHeartbeats = metrics.NewCounter(
		"play_session_owner_heartbeats_total",
		"Heartbeats of running Sessions' live owners, by outcome.",
		heartbeatOutcomeLabel,
	)
)

// RecordOwnerHeartbeat renews sessionUUID's owner lease for ownerLease from
// now while it is RUNNING; otherwise it reports where the Session is, so the
// owner stops serving it.
func (m *Manager) RecordOwnerHeartbeat(ctx context.Context, sessionUUID string) (sessionpkg.OwnerHeartbeatResult, error) {
	defer logging.Step(ctx, "sessionlifecycle.record_owner_heartbeat").Close()

	if _, err := uuid.Parse(sessionUUID); err != nil {
		return sessionpkg.OwnerHeartbeatResult{}, sessionpkg.ErrSessionNotFound
	}
	result, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (sessionpkg.OwnerHeartbeatResult, error) {
		return m.recordOwnerHeartbeatInTx(ctx, tx, sessionUUID)
	})
	if err != nil {
		return sessionpkg.OwnerHeartbeatResult{}, err
	}
	ownerHeartbeats.Inc(ctx, heartbeatOutcomeLabel.Value(string(result.Outcome)))
	return result, nil
}

// recordOwnerHeartbeatInTx locks the Session, so a heartbeat and a
// reconciliation that ends it never cross.
func (m *Manager) recordOwnerHeartbeatInTx(ctx context.Context, tx *gorm.DB, sessionUUID string) (sessionpkg.OwnerHeartbeatResult, error) {
	session, err := m.recordOwnerHeartbeatRepo.LockSession(ctx, tx, sessionUUID)
	if err != nil {
		return sessionpkg.OwnerHeartbeatResult{}, err
	}
	if session == nil {
		return sessionpkg.OwnerHeartbeatResult{}, sessionpkg.ErrSessionNotFound
	}
	if sessionstatus.Status(session.Status) != sessionstatus.Running {
		result := sessionpkg.OwnerHeartbeatResult{Outcome: sessionpkg.HeartbeatSessionNotRunning, Status: session.Status}
		if session.EndCause != nil {
			result.EndCause = *session.EndCause
		}
		return result, nil
	}
	now := m.now().UTC().Truncate(time.Microsecond)
	if err := m.recordOwnerHeartbeatRepo.RenewOwnerLease(ctx, tx, session.ID, now.Add(m.ownerLease)); err != nil {
		return sessionpkg.OwnerHeartbeatResult{}, err
	}
	return sessionpkg.OwnerHeartbeatResult{Outcome: sessionpkg.HeartbeatRecorded}, nil
}
