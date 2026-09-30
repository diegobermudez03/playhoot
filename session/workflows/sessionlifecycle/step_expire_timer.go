package sessionlifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	"github.com/diegobermudez03/playhoot/session"
	"github.com/diegobermudez03/playhoot/session/internal/executor"
	"github.com/diegobermudez03/playhoot/session/workflows/sessionlifecycle/internal/completion"
	"github.com/diegobermudez03/playhoot/session/workflows/sessionlifecycle/internal/platform"
	internalrepo "github.com/diegobermudez03/playhoot/session/workflows/sessionlifecycle/internal/repo"
	"github.com/diegobermudez03/playhoot/session/workflows/sessionlifecycle/internal/timers"
	"github.com/diegobermudez03/playhoot/utils"
	"gorm.io/gorm"
)

// expireTimerRepoAPI is ExpireTimer's own narrow persistence contract.
// LockSessionByID is this workflow's own locking mechanic, not a
// domain-wide protocol.
type expireTimerRepoAPI interface {
	LockSessionByID(ctx context.Context, tx *gorm.DB, sessionID uint) (*internalrepo.Session, error)
	ResolveSessionForTimerObligation(ctx context.Context, timerObligationUUID string) (*uint, error)
	FindTimerObligationByUUID(ctx context.Context, tx *gorm.DB, timerObligationUUID string) (*internalrepo.TimerObligation, error)
	ListActiveParticipantsForRoster(ctx context.Context, tx *gorm.DB, sessionID uint) ([]internalrepo.RosterParticipant, error)
	GetRuntimeTurn(ctx context.Context, tx *gorm.DB, turnID uint) (*internalrepo.RuntimeTurn, error)
	GetGameVersionArtifact(ctx context.Context, tx *gorm.DB, definitionUUID string) (*internalrepo.GameVersionArtifact, error)
	CreateRuntimeTurn(ctx context.Context, tx *gorm.DB, sessionID uint, sequence uint64, sourceKind string, sourceTimerObligationID *uint, sourceCauseEventID *uint, actorID *uint, newState json.RawMessage) (uint, error)
	SetCurrentTurn(ctx context.Context, tx *gorm.DB, sessionID uint, currentTurnID uint) error
	RenewActivityDeadline(ctx context.Context, tx *gorm.DB, sessionID uint, activityExpiresAt time.Time) error
	CloseTimerObligation(ctx context.Context, tx *gorm.DB, timerObligationID uint, closedByTurnID uint) error
	SetSessionTerminal(ctx context.Context, tx *gorm.DB, sessionID uint, terminalAt time.Time, terminalReason string) error
	CreateRuntimeFailure(ctx context.Context, tx *gorm.DB, sessionID uint, failureKind string, errorCode string, errorMessage string, baseTurnID *uint, attemptedSequence uint64, sourceKind string, sourceTimerObligationID *uint, actorID *uint, diagnosticPayload []byte) error
	CancelAllActiveTimerObligationsForSession(ctx context.Context, tx *gorm.DB, sessionID uint, reason string) error
	CreateTimerObligation(ctx context.Context, tx *gorm.DB, sessionID uint, timer string, data []byte, delayMs int64, createdByTurnID uint) (uint, error)
	CancelActiveTimerObligation(ctx context.Context, tx *gorm.DB, sessionID uint, timer string, closedByTurnID uint) error
}

// ExpireTimer submits the previously scheduled timer identified by
// timerObligationUUID as expired, driving whatever reaction the running
// script declared for it. At most one mutation against a given Session
// executes at a time; this call waits its turn and then always reloads
// current authoritative state before applying the expiration, exactly like
// SubmitPlayerEvent already does - a concurrent player event and timer
// expiration for the same Session must never both execute against the same
// starting state.
//
// No idempotency key is required: the obligation's own persisted state is
// the dedup identity. A duplicate/late delivery against an already-resolved
// obligation replays ExpireTimerOutcomeStale without a second execution.
func (m *Manager) ExpireTimer(ctx context.Context, timerObligationUUID session.TimerObligationUUID) (session.ExpireTimerResult, error) {
	defer logging.Step(ctx, "SessionLifecycle.ExpireTimer").Close()
	logging.LogFields(ctx, logging.Field("timer_obligation_uuid", string(timerObligationUUID)))

	sessionID, err := m.expireTimerRepo.ResolveSessionForTimerObligation(ctx, string(timerObligationUUID))
	if err != nil {
		return session.ExpireTimerResult{}, err
	}
	if sessionID == nil {
		return session.ExpireTimerResult{}, session.ErrTimerObligationNotFound
	}

	return utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (session.ExpireTimerResult, error) {
		return m.expireTimerInTx(ctx, tx, *sessionID, timerObligationUUID)
	})
}

// expireTimerInTx is ExpireTimer's per-transaction logic, structurally
// mirroring submitPlayerEventInTx: an obligation lookup and a TIMER_EXPIRED
// Event in place of a PLAYER_EVENT.
func (m *Manager) expireTimerInTx(ctx context.Context, tx *gorm.DB, sessionID uint, timerObligationUUID session.TimerObligationUUID) (session.ExpireTimerResult, error) {
	lockedSession, err := m.expireTimerRepo.LockSessionByID(ctx, tx, sessionID)
	if err != nil {
		return session.ExpireTimerResult{}, err
	}
	if lockedSession == nil {
		return session.ExpireTimerResult{}, session.ErrSessionNotFound
	}

	// A materialized inactivity expiration cancels the obligation being
	// expired, so the existing obligation.State != TimerObligationStateActive
	// check below naturally declines this call as stale - no separate branch
	// is needed.
	if _, err := m.activityExpirer.MaterializeIfDue(ctx, tx, lockedSession, time.Now().UTC()); err != nil {
		return session.ExpireTimerResult{}, err
	}

	obligation, err := m.expireTimerRepo.FindTimerObligationByUUID(ctx, tx, string(timerObligationUUID))
	if err != nil {
		return session.ExpireTimerResult{}, err
	}
	if obligation == nil {
		return session.ExpireTimerResult{}, session.ErrTimerObligationNotFound
	}

	if obligation.State != session.TimerObligationStateActive {
		// Already CONSUMED or CANCELLED - an ordinary duplicate/late physical
		// timer delivery, no execution effect.
		return session.ExpireTimerResult{Outcome: session.ExpireTimerOutcomeStale, SessionUUID: session.SessionUUID(lockedSession.UUID)}, nil
	}

	if lockedSession.CurrentTurnID == nil {
		// An ACTIVE obligation only exists because some committed RuntimeTurn
		// created it, so a locked Session that still owns one but reports no
		// current_turn_id at all is a data-integrity condition, not an
		// ordinary decline.
		monitoring.Alert(ctx, "session has an active timer obligation but no current_turn_id")
		return session.ExpireTimerResult{}, fmt.Errorf("session %d has an active timer obligation but no current_turn_id", lockedSession.ID)
	}
	currentTurn, err := m.expireTimerRepo.GetRuntimeTurn(ctx, tx, *lockedSession.CurrentTurnID)
	if err != nil {
		return session.ExpireTimerResult{}, err
	}
	if currentTurn == nil {
		monitoring.Alert(ctx, "session current_turn_id does not resolve to a runtime turn")
		return session.ExpireTimerResult{}, fmt.Errorf("session %d current_turn_id %d does not resolve to a runtime turn", lockedSession.ID, *lockedSession.CurrentTurnID)
	}

	now := time.Now().UTC()

	artifact, err := m.expireTimerRepo.GetGameVersionArtifact(ctx, tx, lockedSession.GameDefinitionUUID)
	if err != nil {
		return session.ExpireTimerResult{}, err
	}
	if artifact == nil {
		monitoring.Alert(ctx, "session pinned game version artifact is missing")
		return session.ExpireTimerResult{}, session.ErrPinnedDefinitionMissing
	}

	roster, err := m.expireTimerRepo.ListActiveParticipantsForRoster(ctx, tx, lockedSession.ID)
	if err != nil {
		return session.ExpireTimerResult{}, err
	}
	known := knownActorsFromRoster(roster)

	event := platform.NewTimerExpired(obligation.Timer, obligation.Data)
	encodedEvent, err := platform.EncodeEvent(event)
	if err != nil {
		return session.ExpireTimerResult{}, fmt.Errorf("encoding timer expired event: %s", err)
	}

	output, err := m.executor.Execute(ctx, executor.ExecutionInput{
		Script:        executor.ResolvedScript{Source: artifact.BackendScript},
		PreviousState: currentTurn.NewState,
		Event:         encodedEvent,
		Context:       executor.ExecutionContext{LogicalTime: now, RandomSeed: drawSeed()},
	})
	if err != nil {
		var rejected *executor.ScriptRejectedError
		if errors.As(err, &rejected) {
			// The script's own stale/cancelled-delivery backstop: no
			// RuntimeTurn, no state mutation, Session stays RUNNING.
			return session.ExpireTimerResult{Outcome: session.ExpireTimerOutcomeRejected, SessionUUID: session.SessionUUID(lockedSession.UUID)}, nil
		}
		errorCode, errorMessage := classifyExecutionError(err)
		return m.terminalizeExpireTimerFatal(ctx, tx, lockedSession, now, session.TerminalReasonRuntimeExecutionFailed,
			RuntimeFailureKindExecution, errorCode, errorMessage, &currentTurn.ID, currentTurn.Sequence+1, obligation.ID)
	}

	commands, err := parseCommands(output.RequestedCommands, known)
	if err != nil {
		return session.ExpireTimerResult{Outcome: session.ExpireTimerOutcomeRejected, SessionUUID: session.SessionUUID(lockedSession.UUID)}, nil
	}

	obligationID := obligation.ID
	turnID, err := m.expireTimerRepo.CreateRuntimeTurn(ctx, tx, lockedSession.ID, currentTurn.Sequence+1, timerExpiredSourceKind, &obligationID, nil, nil, output.NewState)
	if err != nil {
		return session.ExpireTimerResult{}, err
	}
	if err := m.expireTimerRepo.CloseTimerObligation(ctx, tx, obligationID, turnID); err != nil {
		return session.ExpireTimerResult{}, err
	}
	if err := m.expireTimerRepo.SetCurrentTurn(ctx, tx, lockedSession.ID, turnID); err != nil {
		return session.ExpireTimerResult{}, err
	}
	if err := m.expireTimerRepo.RenewActivityDeadline(ctx, tx, lockedSession.ID, now.Add(m.activityTTL)); err != nil {
		return session.ExpireTimerResult{}, err
	}
	if err := timers.Apply(ctx, tx, m.expireTimerRepo, lockedSession.ID, turnID, commands); err != nil {
		return session.ExpireTimerResult{}, err
	}

	terminalReason, terminated := completion.Detect(commands)
	if terminated {
		if err := m.expireTimerRepo.SetSessionTerminal(ctx, tx, lockedSession.ID, now, terminalReason); err != nil {
			return session.ExpireTimerResult{}, err
		}
		if err := m.expireTimerRepo.CancelAllActiveTimerObligationsForSession(ctx, tx, lockedSession.ID, session.TimerObligationClosureReasonSessionTerminated); err != nil {
			return session.ExpireTimerResult{}, err
		}
	}

	return session.ExpireTimerResult{Outcome: session.ExpireTimerOutcomeExpired, SessionUUID: session.SessionUUID(lockedSession.UUID), TerminalReason: terminalReason}, nil
}

// terminalizeExpireTimerFatal performs ExpireTimer's fatal path: atomically
// terminalizes the Session, persists the session_runtime_failures diagnostic
// record (sourced from the timer obligation whose expiration was being
// processed), and cancels every currently-ACTIVE timer obligation for it,
// mirroring terminalizeSubmitPlayerEventFatal exactly. No partial
// RuntimeTurn is ever persisted: sessions.current_turn_id remains at the
// last committed Turn.
func (m *Manager) terminalizeExpireTimerFatal(ctx context.Context, tx *gorm.DB, lockedSession *internalrepo.Session, terminalAt time.Time, terminalReason string, failureKind string, errorCode string, errorMessage string, baseTurnID *uint, attemptedSequence uint64, timerObligationID uint) (session.ExpireTimerResult, error) {
	if err := m.materializeRuntimeFailure(ctx, tx, m.expireTimerRepo, lockedSession, terminalAt, terminalReason,
		failureKind, errorCode, errorMessage, baseTurnID, attemptedSequence, timerExpiredSourceKind, &timerObligationID, nil); err != nil {
		return session.ExpireTimerResult{}, err
	}
	return session.ExpireTimerResult{Outcome: session.ExpireTimerOutcomeRuntimeExecutionFailed, SessionUUID: session.SessionUUID(lockedSession.UUID)}, nil
}
