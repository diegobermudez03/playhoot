package sessionlifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/diegobermudez03/playhoot/game/session/internal/idempotency"
	"github.com/diegobermudez03/playhoot/game/session/internal/sessionlock"
	"github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/activity"
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

// sessionCancelledCauseKind is session_cause_events.cause_kind's value for a
// manual session cancellation whose SessionCancelled signal the engine
// accepted (a transition matched). sessionCancelledEmptyPayload is the
// cause event's own payload for it: session_cause_events.payload is JSONB
// NOT NULL, and SessionCancelled itself carries no Fields to encode (see
// replay.buildSessionCancelledSignal), so an empty JSON object is the
// payload's entire durable content beyond the row's own actor_id.
const sessionCancelledCauseKind = "SESSION_CANCELLED"

var sessionCancelledEmptyPayload = []byte("{}")

// Cancel session outcome labels persisted to session_requests.outcome for a
// deterministic post-claim decline/success that must survive as a
// replayable outcome - the same technique Start's/SubmitUserIntent's own
// outcome* constants use.
const (
	cancelSessionOutcomeCancelled              = "CANCELLED"
	cancelSessionOutcomeAlreadyTerminal        = "ALREADY_TERMINAL"
	cancelSessionOutcomeNotRunning             = "NOT_RUNNING"
	cancelSessionOutcomeNotHost                = "NOT_HOST"
	cancelSessionOutcomeRuntimeExecutionFailed = "RUNTIME_EXECUTION_FAILED"
)

// cancelSessionRepoAPI is CancelSession's own narrow persistence contract,
// identical in shape to submitUserIntentRepoAPI - both steps address the
// Session itself (not a separate interaction/obligation row) and share the
// same cause-persistence/capture/terminal-cleanup machinery.
type cancelSessionRepoAPI interface {
	interactions.CaptureRepo
	timers.CaptureRepo
	FindActor(ctx context.Context, tx *gorm.DB, sessionID uint, userUUID string) (*internalrepo.Actor, error)
	GetRuntimeTurn(ctx context.Context, tx *gorm.DB, turnID uint) (*internalrepo.RuntimeTurn, error)
	GetRuntimeStart(ctx context.Context, tx *gorm.DB, sessionID uint) (*internalrepo.RuntimeStart, error)
	ListRuntimeTurns(ctx context.Context, tx *gorm.DB, sessionID uint) ([]internalrepo.RuntimeTurnRecord, error)
	GetInteractionByID(ctx context.Context, tx *gorm.DB, interactionID uint) (*internalrepo.Interaction, error)
	GetTimerObligationByID(ctx context.Context, tx *gorm.DB, timerObligationID uint) (*internalrepo.TimerObligation, error)
	GetCauseEventByID(ctx context.Context, tx *gorm.DB, causeEventID uint) (*internalrepo.CauseEvent, error)
	CreateRuntimeTurn(ctx context.Context, tx *gorm.DB, sessionID uint, sequence uint64, sourceKind string, sourceInteractionID *uint, sourceTimerObligationID *uint, sourceCauseEventID *uint, actorID *uint) (uint, error)
	CreateCauseEvent(ctx context.Context, tx *gorm.DB, sessionID uint, runtimeTurnID uint, causeKind string, actorID *uint, payload []byte) (uint, error)
	SetRuntimeTurnCauseEvent(ctx context.Context, tx *gorm.DB, turnID uint, causeEventID uint) error
	SetCurrentTurn(ctx context.Context, tx *gorm.DB, sessionID uint, currentTurnID uint) error
	SetSessionTerminal(ctx context.Context, tx *gorm.DB, sessionID uint, terminalAt time.Time, terminalReason string) error
	CreateRuntimeFailure(ctx context.Context, tx *gorm.DB, sessionID uint, failureKind string, errorCode string, errorMessage string, baseTurnID *uint, attemptedSequence uint64, sourceKind string, sourceInteractionID *uint, sourceTimerObligationID *uint, actorID *uint, diagnosticPayload []byte) error
	CloseAllActiveInteractionsForSession(ctx context.Context, tx *gorm.DB, sessionID uint, reason string) error
	CancelAllActiveTimerObligationsForSession(ctx context.Context, tx *gorm.DB, sessionID uint, reason string) error
}

// cancelSessionRequestPayload is CancelSession's meaningful-field
// idempotency payload, mirroring startRequestPayload's shape.
type cancelSessionRequestPayload struct {
	SessionUUID string `json:"session_uuid"`
	UserUUID    string `json:"user_uuid"`
}

// CancelSession lets sessionUUID's own host explicitly end a RUNNING
// Session before it would otherwise reach a terminal state on its own: it
// delivers a SessionCancelled signal into the Session's current runtime
// instance and always terminalizes the Session once past authorization -
// whether because the authored game itself reacts to SessionCancelled (an
// authored CancelControl transition, reaching a terminal run status or
// not) or because it has no transition for it at all. A host cancel is an
// administrative override, not gameplay input, so - unlike
// AnswerInteraction/SubmitUserIntent/ExpireTimer - it is never a silent
// no-op merely because the engine rejects the signal.
//
// Like SubmitUserIntent, a cancellation has no pre-existing target row to
// dedup against, so idempotencyKey is required.
func (m *Manager) CancelSession(ctx context.Context, sessionUUID session.SessionUUID, userUUID session.UserUUID, idempotencyKey session.IdempotencyKey) (session.CancelSessionResult, error) {
	defer logging.Step(ctx, "SessionLifecycle.CancelSession").Close()
	logging.LogFields(ctx,
		logging.Field("session_uuid", string(sessionUUID)),
		logging.Field("user_uuid", string(userUUID)),
	)

	if idempotencyKey == "" {
		return session.CancelSessionResult{}, session.ErrIdempotencyKeyRequired
	}

	return utils.RunInDBTransaction(ctx, m, func(ctx context.Context, tx *gorm.DB) (session.CancelSessionResult, error) {
		return m.cancelSessionInTx(ctx, tx, sessionUUID, userUUID, idempotencyKey)
	})
}

func (m *Manager) cancelSessionInTx(ctx context.Context, tx *gorm.DB, sessionUUID session.SessionUUID, userUUID session.UserUUID, idempotencyKey session.IdempotencyKey) (session.CancelSessionResult, error) {
	incomingPayload := cancelSessionRequestPayload{SessionUUID: string(sessionUUID), UserUUID: string(userUUID)}

	lockedSession, err := sessionlock.LockByUUID(ctx, tx, string(sessionUUID))
	if err != nil {
		return session.CancelSessionResult{}, err
	}
	if lockedSession == nil {
		return session.CancelSessionResult{}, session.ErrSessionNotFound
	}

	// A materialized inactivity expiration leaves the existing
	// lockedSession.Phase == PhaseTerminal check below to naturally decline
	// this call as cancelSessionOutcomeAlreadyTerminal - an accurate,
	// already-idempotent outcome regardless of which terminal reason applies
	// - no separate branch is needed. CancelSession does not itself renew the
	// deadline - it always terminalizes, leaving nothing left to renew.
	if _, err := activity.MaterializeIfDue(ctx, tx, m.cancelSessionRepo, lockedSession, time.Now().UTC()); err != nil {
		return session.CancelSessionResult{}, err
	}

	payloadBytes, err := json.Marshal(incomingPayload)
	if err != nil {
		return session.CancelSessionResult{}, fmt.Errorf("marshaling cancel session request payload: %s", err)
	}
	requestID, existing, err := idempotency.Claim(ctx, tx, idempotency.ClaimInput{
		Operation:      operationCancelSession,
		UserUUID:       string(userUUID),
		IdempotencyKey: string(idempotencyKey),
		SessionID:      &lockedSession.ID,
		RequestPayload: string(payloadBytes),
	})
	if err != nil {
		return session.CancelSessionResult{}, fmt.Errorf("claiming cancel session request: %s", err)
	}
	if existing != nil {
		return interpretExistingCancelSessionClaim(existing, incomingPayload)
	}

	// Phase-based short circuits, evaluated before host authorization,
	// mirroring Start's own ordering: cancelling an already-TERMINAL Session
	// (whatever the reason) is a harmless, idempotent no-op; a Session still
	// LOBBY has no running engine instance yet to deliver a signal to (see
	// this WORK's own Scope note on LOBBY-phase cancellation).
	if lockedSession.Phase == session.PhaseTerminal {
		return m.declineCancelSession(ctx, tx, requestID, lockedSession, cancelSessionOutcomeAlreadyTerminal, session.CancelSessionOutcomeAlreadyTerminal)
	}
	if lockedSession.Phase == session.PhaseLobby {
		return m.declineCancelSession(ctx, tx, requestID, lockedSession, cancelSessionOutcomeNotRunning, session.CancelSessionOutcomeNotRunning)
	}

	// A missing actor is indistinguishable from "not the host" for this
	// purpose, the same reasoning Start's own host-authorization check
	// already applies.
	actor, err := m.cancelSessionRepo.FindActor(ctx, tx, lockedSession.ID, string(userUUID))
	if err != nil {
		return session.CancelSessionResult{}, err
	}
	if actor == nil || lockedSession.HostActorID == nil || actor.ID != *lockedSession.HostActorID {
		return m.declineCancelSession(ctx, tx, requestID, lockedSession, cancelSessionOutcomeNotHost, session.CancelSessionOutcomeNotHost)
	}

	if lockedSession.CurrentTurnID == nil {
		monitoring.Alert(ctx, "session has no current_turn_id")
		return session.CancelSessionResult{}, fmt.Errorf("session %d has no current_turn_id", lockedSession.ID)
	}
	currentTurn, err := m.cancelSessionRepo.GetRuntimeTurn(ctx, tx, *lockedSession.CurrentTurnID)
	if err != nil {
		return session.CancelSessionResult{}, err
	}
	if currentTurn == nil {
		monitoring.Alert(ctx, "session current_turn_id does not resolve to a runtime turn")
		return session.CancelSessionResult{}, fmt.Errorf("session %d current_turn_id %d does not resolve to a runtime turn", lockedSession.ID, *lockedSession.CurrentTurnID)
	}

	now := time.Now().UTC()

	// The pinned Definition/Version UUID is read directly, never the Game's
	// current version.
	definition, err := m.pinnedGameReader.GetGameDefinition(ctx, lockedSession.GameDefinitionUUID)
	if err != nil {
		return session.CancelSessionResult{}, err
	}
	if definition == nil {
		monitoring.Alert(ctx, "session pinned game definition is missing")
		return session.CancelSessionResult{}, session.ErrPinnedDefinitionMissing
	}
	compiledProgram, diagnostics := engineservice.Compile(*definition)
	if diagnostics.HasErrors() {
		monitoring.Alert(ctx, fmt.Sprintf(
			"pinned game definition failed to recompile cancelling session: session_uuid=%s game_definition_uuid=%s",
			lockedSession.UUID, lockedSession.GameDefinitionUUID,
		))
		return m.terminalizeCancelSessionFatal(ctx, tx, requestID, lockedSession, now, session.TerminalReasonRuntimeStateInvalid,
			RuntimeFailureKindStateInvalid, RuntimeFailureErrorCodeDefinitionRecompileFailed, formatDiagnostics(diagnostics), &currentTurn.ID, currentTurn.Sequence+1, actor.ID)
	}

	// Current authoritative Runtime state is never loaded from a persisted
	// Snapshot (none exists) - engineservice.AdvanceTurn internally replays
	// every durable cause committed so far, given only the ordered signal
	// log below.
	input, priorSignals, err := replay.LoadPriorSignals(ctx, tx, m.cancelSessionRepo, lockedSession.ID)
	if err != nil {
		return session.CancelSessionResult{}, fmt.Errorf("reconstructing current runtime state: %s", err)
	}

	signal := engine.Signal{Kind: engine.SignalKindNamed, Name: "SessionCancelled"}

	outputs, err := engineservice.AdvanceTurn(compiledProgram, input, priorSignals, signal, engine.DefaultLimits())
	if err != nil {
		if errors.Is(err, engineservice.ErrReplayDivergence) {
			// Every element of priorSignals already succeeded once - it is
			// durable specifically because it did - so failing to replay it
			// identically is a data-integrity condition, never an ordinary
			// decline.
			monitoring.Alert(ctx, fmt.Sprintf("session %d: %s", lockedSession.ID, err))
			return session.CancelSessionResult{}, fmt.Errorf("reconstructing current runtime state: %s", err)
		}
		if errors.Is(err, engineservice.ErrSignalRejected) || errors.Is(err, engineservice.ErrInputRejected) {
			// No transition matches SessionCancelled at all. A host cancel
			// still always ends the Session (the human-approved
			// force-terminal decision) - but nothing here was ever accepted
			// by the engine, so no RuntimeTurn/cause event is created: doing
			// so would durably record a signal that replay would then have
			// to replay forward and see rejected again, indistinguishable
			// from genuine divergence/corruption (see Approved Design's "Why
			// the rejected path creates no RuntimeTurn"). sessions.
			// terminal_reason alone durably explains this outcome.
			return m.terminalizeCancelSessionForced(ctx, tx, requestID, lockedSession, now)
		}
		errorCode, errorMessage := classifyExecutionError(err)
		return m.terminalizeCancelSessionFatal(ctx, tx, requestID, lockedSession, now, session.TerminalReasonRuntimeExecutionFailed,
			RuntimeFailureKindExecution, errorCode, errorMessage, &currentTurn.ID, currentTurn.Sequence+1, actor.ID)
	}

	// Accepted: the authored game has a transition matching SessionCancelled.
	// session_cause_events.runtime_turn_id is NOT NULL, so the cause event
	// row can only be created after the Turn it belongs to already exists -
	// its own pointer back to it is then backfilled, exactly like
	// SubmitUserIntent's own cause persistence.
	actorID := actor.ID
	turnID, err := m.cancelSessionRepo.CreateRuntimeTurn(ctx, tx, lockedSession.ID, currentTurn.Sequence+1, replay.SessionCancelledSourceKind, nil, nil, nil, &actorID)
	if err != nil {
		return session.CancelSessionResult{}, err
	}
	causeEventID, err := m.cancelSessionRepo.CreateCauseEvent(ctx, tx, lockedSession.ID, turnID, sessionCancelledCauseKind, &actorID, sessionCancelledEmptyPayload)
	if err != nil {
		return session.CancelSessionResult{}, err
	}
	if err := m.cancelSessionRepo.SetRuntimeTurnCauseEvent(ctx, tx, turnID, causeEventID); err != nil {
		return session.CancelSessionResult{}, err
	}

	if err := interactions.Capture(ctx, tx, m.cancelSessionRepo, lockedSession.ID, turnID, outputs); err != nil {
		return session.CancelSessionResult{}, err
	}
	if err := timers.Capture(ctx, tx, m.cancelSessionRepo, lockedSession.ID, turnID, outputs); err != nil {
		return session.CancelSessionResult{}, err
	}
	if err := m.cancelSessionRepo.SetCurrentTurn(ctx, tx, lockedSession.ID, turnID); err != nil {
		return session.CancelSessionResult{}, err
	}

	// Force-terminal always: reuse the game's own completion detection when
	// it found one (TerminalReasonGame*); otherwise the accepted transition
	// moved state without ending the run, so this call terminalizes it
	// anyway under the new TerminalReasonSessionCancelledByHost. Either way,
	// a CancelSession call that reaches this point always ends the Session -
	// the terminal-cleanup steps below always run, unlike every other
	// signal-driven capability's conditional "if terminated" branch.
	terminalReason, terminated := completion.Detect(outputs)
	if !terminated {
		terminalReason = session.TerminalReasonSessionCancelledByHost
	}
	if err := m.cancelSessionRepo.SetSessionTerminal(ctx, tx, lockedSession.ID, now, terminalReason); err != nil {
		return session.CancelSessionResult{}, err
	}
	if err := m.cancelSessionRepo.CloseAllActiveInteractionsForSession(ctx, tx, lockedSession.ID, session.InteractionClosureReasonSessionTerminated); err != nil {
		return session.CancelSessionResult{}, err
	}
	if err := m.cancelSessionRepo.CancelAllActiveTimerObligationsForSession(ctx, tx, lockedSession.ID, session.TimerObligationClosureReasonSessionTerminated); err != nil {
		return session.CancelSessionResult{}, err
	}

	mappedOutputs, err := m.mapOutputs(ctx, tx, lockedSession.ID, clientoutputs.ClientFacing(outputs))
	if err != nil {
		return session.CancelSessionResult{}, fmt.Errorf("mapping client-facing outputs: %s", err)
	}

	result := session.CancelSessionResult{Outcome: session.CancelSessionOutcomeCancelled, SessionUUID: session.SessionUUID(lockedSession.UUID), Outputs: mappedOutputs, TerminalReason: terminalReason}
	responseBytes, err := json.Marshal(result)
	if err != nil {
		return session.CancelSessionResult{}, fmt.Errorf("marshaling cancel session response payload: %s", err)
	}
	if err := idempotency.Complete(ctx, tx, requestID, &lockedSession.ID, cancelSessionOutcomeCancelled, string(responseBytes)); err != nil {
		return session.CancelSessionResult{}, fmt.Errorf("completing cancel session request: %s", err)
	}
	return result, nil
}

// declineCancelSession completes requestID as an ordinary decline (no
// engine effect, no state change) and reports resultOutcome, mirroring
// declineSubmitUserIntent's shape.
func (m *Manager) declineCancelSession(ctx context.Context, tx *gorm.DB, requestID uint, lockedSession *sessionlock.Session, outcomeLabel string, resultOutcome session.CancelSessionOutcome) (session.CancelSessionResult, error) {
	if err := idempotency.Complete(ctx, tx, requestID, &lockedSession.ID, outcomeLabel, ""); err != nil {
		return session.CancelSessionResult{}, fmt.Errorf("completing cancel session request: %s", err)
	}
	return session.CancelSessionResult{Outcome: resultOutcome, SessionUUID: session.SessionUUID(lockedSession.UUID)}, nil
}

// terminalizeCancelSessionForced performs CancelSession's forced-terminal
// path for a SessionCancelled signal the engine rejected outright (no
// RuntimeTurn, current_turn_id unchanged) - see cancelSessionInTx's own
// ErrSignalRejected/ErrInputRejected handling for why no cause is
// persisted. Outcome is still CancelSessionOutcomeCancelled: the Session
// ends either way.
func (m *Manager) terminalizeCancelSessionForced(ctx context.Context, tx *gorm.DB, requestID uint, lockedSession *sessionlock.Session, terminalAt time.Time) (session.CancelSessionResult, error) {
	if err := m.cancelSessionRepo.SetSessionTerminal(ctx, tx, lockedSession.ID, terminalAt, session.TerminalReasonSessionCancelledByHost); err != nil {
		return session.CancelSessionResult{}, err
	}
	if err := m.cancelSessionRepo.CloseAllActiveInteractionsForSession(ctx, tx, lockedSession.ID, session.InteractionClosureReasonSessionTerminated); err != nil {
		return session.CancelSessionResult{}, err
	}
	if err := m.cancelSessionRepo.CancelAllActiveTimerObligationsForSession(ctx, tx, lockedSession.ID, session.TimerObligationClosureReasonSessionTerminated); err != nil {
		return session.CancelSessionResult{}, err
	}
	result := session.CancelSessionResult{Outcome: session.CancelSessionOutcomeCancelled, SessionUUID: session.SessionUUID(lockedSession.UUID), TerminalReason: session.TerminalReasonSessionCancelledByHost}
	responseBytes, err := json.Marshal(result)
	if err != nil {
		return session.CancelSessionResult{}, fmt.Errorf("marshaling cancel session response payload: %s", err)
	}
	if err := idempotency.Complete(ctx, tx, requestID, &lockedSession.ID, cancelSessionOutcomeCancelled, string(responseBytes)); err != nil {
		return session.CancelSessionResult{}, fmt.Errorf("completing cancel session request: %s", err)
	}
	return result, nil
}

// terminalizeCancelSessionFatal performs CancelSession's fatal path:
// atomically terminalizes the Session, persists the session_runtime_failures
// diagnostic record, and closes every currently-ACTIVE session_interactions
// row and timer obligation for it, mirroring terminalizeSubmitUserIntentFatal
// exactly, and completes the idempotency claim so a retry replays this same
// outcome instead of re-attempting a doomed execution. Distinct from
// terminalizeCancelSessionForced below: this is a genuine runtime failure
// (GAME-ADR-0017 class B/C), not the host's own intentional cancellation, so
// only this path persists a session_runtime_failures row.
func (m *Manager) terminalizeCancelSessionFatal(ctx context.Context, tx *gorm.DB, requestID uint, lockedSession *sessionlock.Session, terminalAt time.Time, terminalReason string, failureKind string, errorCode string, errorMessage string, baseTurnID *uint, attemptedSequence uint64, actorID uint) (session.CancelSessionResult, error) {
	if err := m.materializeRuntimeFailure(ctx, tx, m.cancelSessionRepo, lockedSession, terminalAt, terminalReason,
		failureKind, errorCode, errorMessage, baseTurnID, attemptedSequence, replay.SessionCancelledSourceKind, nil, nil, &actorID); err != nil {
		return session.CancelSessionResult{}, err
	}
	if err := idempotency.Complete(ctx, tx, requestID, &lockedSession.ID, cancelSessionOutcomeRuntimeExecutionFailed, ""); err != nil {
		return session.CancelSessionResult{}, fmt.Errorf("completing cancel session request: %s", err)
	}
	return session.CancelSessionResult{Outcome: session.CancelSessionOutcomeRuntimeExecutionFailed, SessionUUID: session.SessionUUID(lockedSession.UUID)}, nil
}

// interpretExistingCancelSessionClaim replays an already-completed
// idempotency claim's original outcome, mirroring
// interpretExistingSubmitUserIntentClaim exactly.
func interpretExistingCancelSessionClaim(existing *idempotency.Request, incoming cancelSessionRequestPayload) (session.CancelSessionResult, error) {
	if existing.Status != idempotency.StatusCompleted {
		return session.CancelSessionResult{}, session.ErrIdempotencyInFlight
	}

	var stored cancelSessionRequestPayload
	if err := json.Unmarshal([]byte(existing.RequestPayload), &stored); err != nil {
		return session.CancelSessionResult{}, fmt.Errorf("decoding stored cancel session request payload: %s", err)
	}
	if stored != incoming {
		return session.CancelSessionResult{}, session.ErrIdempotencyConflict
	}

	switch existing.Outcome {
	case cancelSessionOutcomeAlreadyTerminal:
		return session.CancelSessionResult{Outcome: session.CancelSessionOutcomeAlreadyTerminal}, nil
	case cancelSessionOutcomeNotRunning:
		return session.CancelSessionResult{Outcome: session.CancelSessionOutcomeNotRunning}, nil
	case cancelSessionOutcomeNotHost:
		return session.CancelSessionResult{Outcome: session.CancelSessionOutcomeNotHost}, nil
	case cancelSessionOutcomeRuntimeExecutionFailed:
		return session.CancelSessionResult{Outcome: session.CancelSessionOutcomeRuntimeExecutionFailed}, nil
	}

	if existing.ResponsePayload == nil {
		return session.CancelSessionResult{}, fmt.Errorf("completed cancel session idempotency record missing response payload")
	}
	var result session.CancelSessionResult
	if err := json.Unmarshal([]byte(*existing.ResponsePayload), &result); err != nil {
		return session.CancelSessionResult{}, fmt.Errorf("decoding stored cancel session response payload: %s", err)
	}
	return result, nil
}
