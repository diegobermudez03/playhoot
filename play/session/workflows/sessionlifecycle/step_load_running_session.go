package sessionlifecycle

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/sessionstatus"
	"github.com/diegobermudez03/playhoot/play/session/internal/objectstore"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/sessionlifecycle/internal/repo"
)

//go:generate mockgen -package=sessionlifecycle -destination=step_load_running_session_mock_test.go . loadRunningSessionRepo

// loadRunningSessionRepo is LoadRunningSession's own narrow persistence
// contract.
type loadRunningSessionRepo interface {
	FindRunningSession(ctx context.Context, sessionUUID string) (*internalrepo.RunningSessionRecord, error)
	FindParticipants(ctx context.Context, sessionID uint) ([]internalrepo.Participant, error)
}

// LoadRunningSession returns what running sessionUUID again needs: its game's
// script, its roster and its latest checkpoint. A Session in another status
// is sessionpkg.ErrSessionNotRunning.
func (m *Manager) LoadRunningSession(ctx context.Context, sessionUUID string) (sessionpkg.RunningSession, error) {
	defer logging.Step(ctx, "sessionlifecycle.load_running_session").Close()

	if _, err := uuid.Parse(sessionUUID); err != nil {
		return sessionpkg.RunningSession{}, sessionpkg.ErrSessionNotFound
	}
	record, err := m.loadRunningSessionRepo.FindRunningSession(ctx, sessionUUID)
	if err != nil {
		return sessionpkg.RunningSession{}, err
	}
	if record == nil {
		return sessionpkg.RunningSession{}, sessionpkg.ErrSessionNotFound
	}
	if sessionstatus.Status(record.Status) != sessionstatus.Running {
		return sessionpkg.RunningSession{}, sessionpkg.ErrSessionNotRunning
	}

	// A Session only becomes RUNNING together with its roster and initial
	// checkpoint, and only on a version with a script: anything missing is
	// broken data.
	if record.CheckpointRevision == nil || len(record.CheckpointContent) == 0 {
		monitoring.Alert(ctx, "play running session has no recovery checkpoint")
		return sessionpkg.RunningSession{}, fmt.Errorf("running session %s has no checkpoint", sessionUUID)
	}
	participants, err := m.loadRunningSessionRepo.FindParticipants(ctx, record.ID)
	if err != nil {
		return sessionpkg.RunningSession{}, err
	}
	if len(participants) == 0 {
		monitoring.Alert(ctx, "play running session has no participants")
		return sessionpkg.RunningSession{}, fmt.Errorf("running session %s has no participants", sessionUUID)
	}
	if record.ScriptObjectKey == nil || record.ScriptSHA256 == nil || record.ScriptSize == nil {
		monitoring.Alert(ctx, "play session is pinned to a game version without a backend script")
		return sessionpkg.RunningSession{}, fmt.Errorf("game version %s has no backend script", record.GameVersionUUID)
	}
	script, err := m.scripts.Load(ctx, objectstore.Locator{ObjectKey: *record.ScriptObjectKey, SHA256: *record.ScriptSHA256, Size: *record.ScriptSize})
	if err != nil {
		return sessionpkg.RunningSession{}, fmt.Errorf("loading backend script: %w", err)
	}

	roster := make([]sessionpkg.StartParticipant, len(participants))
	for i, participant := range participants {
		roster[i] = sessionpkg.StartParticipant{UserUUID: participant.UserUUID, ParticipantID: participant.ParticipantID}
	}
	return sessionpkg.RunningSession{
		GameVersionUUID: record.GameVersionUUID,
		Script:          script,
		Participants:    roster,
		Checkpoint:      sessionpkg.Checkpoint{ContextRevision: *record.CheckpointRevision, Content: record.CheckpointContent},
	}, nil
}
