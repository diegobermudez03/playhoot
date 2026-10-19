package sessionlifecycle

import (
	"context"
	"fmt"
	"slices"
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

//go:generate mockgen -package=sessionlifecycle -destination=step_end_session_mock_test.go . endSessionRepo

// endSessionRepo is EndSession's own narrow persistence contract.
type endSessionRepo interface {
	LockSession(ctx context.Context, tx *gorm.DB, sessionUUID string) (*internalrepo.LockedSession, error)
	ChangeSessionStatus(ctx context.Context, tx *gorm.DB, sessionID uint, fromStatus string, toStatus string, endCause *string, changedBy *string, changedAt time.Time) error
	ReleaseJoinCodes(ctx context.Context, tx *gorm.DB, sessionID uint, releasedAt time.Time) error
}

var (
	endCauseLabel = metrics.NewLabel("cause",
		string(sessionpkg.EndGameFinished),
		string(sessionpkg.EndHostCancelled),
		string(sessionpkg.EndAllPlayersLeft),
		string(sessionpkg.EndPlatformTerminated),
		string(sessionpkg.EndLobbyExpired),
		string(sessionpkg.EndInactivityTimeout),
		string(sessionpkg.EndMaxLifetime),
		string(sessionpkg.EndGameStartFailure),
		string(sessionpkg.EndScriptFailure),
		string(sessionpkg.EndUnrecoverableInfraFailure),
		string(sessionpkg.EndOwnerLost),
	)

	sessionsEnded = metrics.NewCounter(
		"play_session_ended_total",
		"Live Sessions moved to a terminal status, by end cause.",
		endCauseLabel,
	)
)

// EndSession moves sessionUUID to the terminal status end.Cause leads to,
// recording who asked, and releases its join codes. A Session in a status the
// cause does not apply to is left as it is and reported as it is.
func (m *Manager) EndSession(ctx context.Context, sessionUUID string, end sessionpkg.SessionEnd) (sessionpkg.EndSessionResult, error) {
	defer logging.Step(ctx, "sessionlifecycle.end_session").Close()

	if _, err := uuid.Parse(sessionUUID); err != nil {
		return sessionpkg.EndSessionResult{}, sessionpkg.ErrSessionNotFound
	}
	ending, known := endcause.EndingOf(endcause.Cause(end.Cause))
	if !known {
		return sessionpkg.EndSessionResult{}, fmt.Errorf("unknown end cause %q", end.Cause)
	}
	if end.ChangedBy != "" {
		if _, err := uuid.Parse(end.ChangedBy); err != nil {
			return sessionpkg.EndSessionResult{}, fmt.Errorf("invalid user %q ending the session", end.ChangedBy)
		}
	}
	result, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (sessionpkg.EndSessionResult, error) {
		return m.endSessionInTx(ctx, tx, sessionUUID, end, ending)
	})
	if err != nil {
		return sessionpkg.EndSessionResult{}, err
	}
	logging.LogFields(ctx, logging.Field("end_cause", string(end.Cause)), logging.Field("end_session_outcome", string(result.Outcome)))
	if result.Outcome == sessionpkg.SessionEndedNow {
		sessionsEnded.Inc(ctx, endCauseLabel.Value(string(end.Cause)))
	}
	return result, nil
}

func (m *Manager) endSessionInTx(ctx context.Context, tx *gorm.DB, sessionUUID string, end sessionpkg.SessionEnd, ending endcause.Ending) (sessionpkg.EndSessionResult, error) {
	session, err := m.endSessionRepo.LockSession(ctx, tx, sessionUUID)
	if err != nil {
		return sessionpkg.EndSessionResult{}, err
	}
	if session == nil {
		return sessionpkg.EndSessionResult{}, sessionpkg.ErrSessionNotFound
	}
	if !slices.Contains(ending.From, sessionstatus.Status(session.Status)) {
		result := sessionpkg.EndSessionResult{Outcome: sessionpkg.SessionNotEnded, Status: session.Status}
		if session.EndCause != nil {
			result.EndCause = *session.EndCause
		}
		return result, nil
	}
	now := m.now().UTC().Truncate(time.Microsecond)
	cause := string(end.Cause)
	// Nil records the platform as the one who ended it.
	var changedBy *string
	if end.ChangedBy != "" {
		changedBy = &end.ChangedBy
	}
	if err := m.endSessionRepo.ChangeSessionStatus(ctx, tx, session.ID, session.Status, string(ending.Status), &cause, changedBy, now); err != nil {
		return sessionpkg.EndSessionResult{}, err
	}
	// A started Session released its codes at Start; this covers a lobby.
	if err := m.endSessionRepo.ReleaseJoinCodes(ctx, tx, session.ID, now); err != nil {
		return sessionpkg.EndSessionResult{}, err
	}
	return sessionpkg.EndSessionResult{Outcome: sessionpkg.SessionEndedNow, Status: string(ending.Status), EndCause: cause}, nil
}
