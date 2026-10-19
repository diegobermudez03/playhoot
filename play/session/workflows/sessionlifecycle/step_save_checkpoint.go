package sessionlifecycle

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/sessionstatus"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/sessionlifecycle/internal/repo"
	"github.com/diegobermudez03/playhoot/utils"
)

//go:generate mockgen -package=sessionlifecycle -destination=step_save_checkpoint_mock_test.go . saveCheckpointRepo

// saveCheckpointRepo is SaveCheckpoint's own narrow persistence contract.
type saveCheckpointRepo interface {
	LockSession(ctx context.Context, tx *gorm.DB, sessionUUID string) (*internalrepo.LockedSession, error)
	SaveCheckpoint(ctx context.Context, tx *gorm.DB, sessionID uint, contextRevision int64, content []byte, savedAt time.Time) error
}

// SaveCheckpoint makes checkpoint sessionUUID's latest recovery checkpoint
// while it is RUNNING; one of a higher context revision already stored is
// kept instead.
func (m *Manager) SaveCheckpoint(ctx context.Context, sessionUUID string, checkpoint sessionpkg.Checkpoint) (sessionpkg.SaveCheckpointOutcome, error) {
	defer logging.Step(ctx, "sessionlifecycle.save_checkpoint").Close()

	if _, err := uuid.Parse(sessionUUID); err != nil {
		return "", sessionpkg.ErrSessionNotFound
	}
	return utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (sessionpkg.SaveCheckpointOutcome, error) {
		return m.saveCheckpointInTx(ctx, tx, sessionUUID, checkpoint)
	})
}

// saveCheckpointInTx locks the Session so a checkpoint is never stored for
// one that ended meanwhile.
func (m *Manager) saveCheckpointInTx(ctx context.Context, tx *gorm.DB, sessionUUID string, checkpoint sessionpkg.Checkpoint) (sessionpkg.SaveCheckpointOutcome, error) {
	session, err := m.saveCheckpointRepo.LockSession(ctx, tx, sessionUUID)
	if err != nil {
		return "", err
	}
	if session == nil {
		return "", sessionpkg.ErrSessionNotFound
	}
	if sessionstatus.Status(session.Status) != sessionstatus.Running {
		return sessionpkg.CheckpointSessionNotRunning, nil
	}
	now := m.now().UTC().Truncate(time.Microsecond)
	if err := m.saveCheckpointRepo.SaveCheckpoint(ctx, tx, session.ID, checkpoint.ContextRevision, checkpoint.Content, now); err != nil {
		return "", err
	}
	return sessionpkg.CheckpointSaved, nil
}
