package sessionlifecycle

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/diegobermudez03/playhoot/game/session/internal/sessionlock"
	"github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/clientoutputs"
	"github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/completion"
	"github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/interactions"
	"github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/replay"
	internalrepo "github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/repo"
	"github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/timers"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	"github.com/diegobermudez03/playhoot/utils"
	"gorm.io/gorm"
)

// expireTimerRepoAPI is ExpireTimer's own narrow persistence contract. It
// embeds interactions.CaptureRepo/timers.CaptureRepo (a timer's expiration
// may itself open a question or schedule/cancel another timer) and
// satisfies replay.Repo (GetRuntimeStart/ListRuntimeTurns/
// GetInteractionByID/GetTimerObligationByID) so replay.LoadPriorSignals can
// be called directly with it, exactly like answerInteractionRepoAPI already
// does.
type expireTimerRepoAPI interface {
	interactions.CaptureRepo
	timers.CaptureRepo
	ResolveSessionForTimerObligation(ctx context.Context, timerObligationUUID string) (*uint, error)
	FindTimerObligationByUUID(ctx context.Context, tx *gorm.DB, timerObligationUUID string) (*internalrepo.TimerObligation, error)
	GetRuntimeTurn(ctx context.Context, tx *gorm.DB, turnID uint) (*internalrepo.RuntimeTurn, error)
	GetRuntimeStart(ctx context.Context, tx *gorm.DB, sessionID uint) (*internalrepo.RuntimeStart, error)
	ListRuntimeTurns(ctx context.Context, tx *gorm.DB, sessionID uint) ([]internalrepo.RuntimeTurnRecord, error)
	GetInteractionByID(ctx context.Context, tx *gorm.DB, interactionID uint) (*internalrepo.Interaction, error)
	GetTimerObligationByID(ctx context.Context, tx *gorm.DB, timerObligationID uint) (*internalrepo.TimerObligation, error)
	CreateRuntimeTurn(ctx context.Context, tx *gorm.DB, sessionID uint, sequence uint64, sourceKind string, sourceInteractionID *uint, sourceTimerObligationID *uint, actorID *uint) (uint, error)
	SetCurrentTurn(ctx context.Context, tx *gorm.DB, sessionID uint, currentTurnID uint) error
	CloseTimerObligation(ctx context.Context, tx *gorm.DB, timerObligationID uint, closedByTurnID uint) error
	SetSessionTerminal(ctx context.Context, tx *gorm.DB, sessionID uint, terminalAt time.Time, terminalReason string) error
	CloseAllActiveInteractionsForSession(ctx context.Context, tx *gorm.DB, sessionID uint, reason string) error
	CancelAllActiveTimerObligationsForSession(ctx context.Context, tx *gorm.DB, sessionID uint, reason string) error
}

// ExpireTimer submits the previously scheduled timer identified by
// timerObligationUUID as expired, driving whatever transition the running
// game declared for it - an ordinary timer or one of a keyed timer's
// independent occurrences. At most one mutation against a given Session
// executes at a time; this call waits its turn and then always reloads
// current authoritative state before applying the expiration, exactly like
// AnswerInteraction already does - a concurrent interaction response and
// timer expiration for the same Session must never both execute against the
// same starting state.
//
// No idempotency key is required: the obligation's own persisted state is
// the dedup identity. A duplicate/late delivery against an already-resolved
// obligation replays ExpireTimerOutcomeStale without a second engine effect.
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

	return utils.RunInDBTransaction(ctx, m, func(ctx context.Context, tx *gorm.DB) (session.ExpireTimerResult, error) {
		return m.expireTimerInTx(ctx, tx, *sessionID, timerObligationUUID)
	})
}

// expireTimerInTx is ExpireTimer's per-transaction logic, structurally
// mirroring answerInteractionInTx: an obligation lookup and a
// SignalKindTimerExpired/SignalKindKeyedTimerExpired signal in place of an
// interaction lookup and SignalKindInteractionAnswered.
func (m *Manager) expireTimerInTx(ctx context.Context, tx *gorm.DB, sessionID uint, timerObligationUUID session.TimerObligationUUID) (session.ExpireTimerResult, error) {
	lockedSession, err := sessionlock.LockByID(ctx, tx, sessionID)
	if err != nil {
		return session.ExpireTimerResult{}, err
	}
	if lockedSession == nil {
		return session.ExpireTimerResult{}, session.ErrSessionNotFound
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
		// timer delivery, no engine effect.
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

	// The pinned Definition/Version UUID is read directly, never the Game's
	// current version.
	definition, err := m.pinnedGameReader.GetGameDefinition(ctx, lockedSession.GameDefinitionUUID)
	if err != nil {
		return session.ExpireTimerResult{}, err
	}
	if definition == nil {
		monitoring.Alert(ctx, "session pinned game definition is missing")
		return session.ExpireTimerResult{}, session.ErrPinnedDefinitionMissing
	}
	compiledProgram, diagnostics := engineservice.Compile(*definition)
	if diagnostics.HasErrors() {
		monitoring.Alert(ctx, fmt.Sprintf(
			"pinned game definition failed to recompile expiring timer: session_uuid=%s game_definition_uuid=%s",
			lockedSession.UUID, lockedSession.GameDefinitionUUID,
		))
		return m.terminalizeExpireTimerFatal(ctx, tx, lockedSession, now, session.TerminalReasonRuntimeStateInvalid)
	}

	// Current authoritative Runtime state is never loaded from a persisted
	// Snapshot (none exists) - engineservice.AdvanceTurn internally replays
	// every durable cause committed so far, given only the ordered signal log
	// below.
	input, priorSignals, err := replay.LoadPriorSignals(ctx, tx, m.expireTimerRepo, lockedSession.ID)
	if err != nil {
		return session.ExpireTimerResult{}, fmt.Errorf("reconstructing current runtime state: %s", err)
	}

	var signal engine.Signal
	if obligation.EngineKey == nil {
		signal = engine.Signal{Kind: engine.SignalKindTimerExpired, Slot: obligation.EngineSlot}
	} else {
		key, err := engineservice.DecodeValue(obligation.EngineKey)
		if err != nil {
			return session.ExpireTimerResult{}, fmt.Errorf("decoding timer obligation key: %s", err)
		}
		signal = engine.Signal{Kind: engine.SignalKindKeyedTimerExpired, Slot: obligation.EngineSlot, Key: key}
	}

	outputs, err := engineservice.AdvanceTurn(compiledProgram, input, priorSignals, signal, engine.DefaultLimits())
	if err != nil {
		if errors.Is(err, engineservice.ErrReplayDivergence) {
			// Every element of priorSignals already succeeded once - it is
			// durable specifically because it did - so failing to replay it
			// identically is a data-integrity condition, never an ordinary
			// decline.
			monitoring.Alert(ctx, fmt.Sprintf("session %d: %s", lockedSession.ID, err))
			return session.ExpireTimerResult{}, fmt.Errorf("reconstructing current runtime state: %s", err)
		}
		// The engine's own stale/cancelled-delivery backstop
		// (program.TimerSlotDeclaration's documented contract): no RuntimeTurn,
		// no Snapshot mutation, Session stays RUNNING.
		if errors.Is(err, engineservice.ErrSignalRejected) || errors.Is(err, engineservice.ErrInputRejected) {
			return session.ExpireTimerResult{Outcome: session.ExpireTimerOutcomeRejected, SessionUUID: session.SessionUUID(lockedSession.UUID)}, nil
		}
		return m.terminalizeExpireTimerFatal(ctx, tx, lockedSession, now, session.TerminalReasonRuntimeExecutionFailed)
	}

	obligationID := obligation.ID
	turnID, err := m.expireTimerRepo.CreateRuntimeTurn(ctx, tx, lockedSession.ID, currentTurn.Sequence+1, replay.TimerExpiredSourceKind, nil, &obligationID, nil)
	if err != nil {
		return session.ExpireTimerResult{}, err
	}
	if err := m.expireTimerRepo.CloseTimerObligation(ctx, tx, obligationID, turnID); err != nil {
		return session.ExpireTimerResult{}, err
	}
	if err := interactions.Capture(ctx, tx, m.expireTimerRepo, lockedSession.ID, turnID, outputs); err != nil {
		return session.ExpireTimerResult{}, err
	}
	if err := timers.Capture(ctx, tx, m.expireTimerRepo, lockedSession.ID, turnID, outputs); err != nil {
		return session.ExpireTimerResult{}, err
	}
	if err := m.expireTimerRepo.SetCurrentTurn(ctx, tx, lockedSession.ID, turnID); err != nil {
		return session.ExpireTimerResult{}, err
	}

	terminalReason, terminated := completion.Detect(outputs)
	if terminated {
		if err := m.expireTimerRepo.SetSessionTerminal(ctx, tx, lockedSession.ID, now, terminalReason); err != nil {
			return session.ExpireTimerResult{}, err
		}
		if err := m.expireTimerRepo.CloseAllActiveInteractionsForSession(ctx, tx, lockedSession.ID, session.InteractionClosureReasonSessionTerminated); err != nil {
			return session.ExpireTimerResult{}, err
		}
		if err := m.expireTimerRepo.CancelAllActiveTimerObligationsForSession(ctx, tx, lockedSession.ID, session.TimerObligationClosureReasonSessionTerminated); err != nil {
			return session.ExpireTimerResult{}, err
		}
	}

	mappedOutputs, err := m.mapOutputs(ctx, tx, lockedSession.ID, clientoutputs.ClientFacing(outputs))
	if err != nil {
		return session.ExpireTimerResult{}, fmt.Errorf("mapping client-facing outputs: %s", err)
	}

	return session.ExpireTimerResult{Outcome: session.ExpireTimerOutcomeExpired, SessionUUID: session.SessionUUID(lockedSession.UUID), Outputs: mappedOutputs, TerminalReason: terminalReason}, nil
}

// terminalizeExpireTimerFatal performs ExpireTimer's fatal path: atomically
// terminalizes the Session, closes every currently-ACTIVE session_interactions
// row, and cancels every currently-ACTIVE session_timer_obligations row for
// it, mirroring terminalizeAnswerInteractionFatal exactly. No partial
// RuntimeTurn is ever persisted: sessions.current_turn_id remains at the
// last committed Turn.
func (m *Manager) terminalizeExpireTimerFatal(ctx context.Context, tx *gorm.DB, lockedSession *sessionlock.Session, terminalAt time.Time, terminalReason string) (session.ExpireTimerResult, error) {
	if err := m.expireTimerRepo.SetSessionTerminal(ctx, tx, lockedSession.ID, terminalAt, terminalReason); err != nil {
		return session.ExpireTimerResult{}, err
	}
	if err := m.expireTimerRepo.CloseAllActiveInteractionsForSession(ctx, tx, lockedSession.ID, session.InteractionClosureReasonSessionTerminated); err != nil {
		return session.ExpireTimerResult{}, err
	}
	if err := m.expireTimerRepo.CancelAllActiveTimerObligationsForSession(ctx, tx, lockedSession.ID, session.TimerObligationClosureReasonSessionTerminated); err != nil {
		return session.ExpireTimerResult{}, err
	}
	return session.ExpireTimerResult{Outcome: session.ExpireTimerOutcomeRuntimeExecutionFailed, SessionUUID: session.SessionUUID(lockedSession.UUID)}, nil
}
