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

// sessionCancelledCauseKind is session_cause_events.cause_kind's value for a
// manual session cancellation whose SESSION_CANCELLED event the authored
// script accepted (requested a Command in reaction to it, or simply did not
// reject it). sessionCancelledEmptyPayload is the cause event's own payload
// for it: session_cause_events.payload is JSONB NOT NULL, and
// platform.SessionCancelled itself carries no fields to encode, so an empty
// JSON object is the payload's entire durable content beyond the row's own
// actor_id.
const sessionCancelledCauseKind = "SESSION_CANCELLED"

var sessionCancelledEmptyPayload = []byte("{}")

// Cancel session outcome labels persisted to session_requests.outcome for a
// deterministic post-claim decline/success that must survive as a
// replayable outcome - the same technique Start's/SubmitPlayerEvent's own
// outcome* constants use.
const (
	cancelSessionOutcomeCancelled              = "CANCELLED"
	cancelSessionOutcomeAlreadyTerminal        = "ALREADY_TERMINAL"
	cancelSessionOutcomeNotRunning             = "NOT_RUNNING"
	cancelSessionOutcomeNotHost                = "NOT_HOST"
	cancelSessionOutcomeRuntimeExecutionFailed = "RUNTIME_EXECUTION_FAILED"
)

// cancelSessionRepoAPI is CancelSession's own narrow persistence contract,
// identical in shape to submitPlayerEventRepoAPI - both steps address the
// Session itself and share the same cause-persistence/terminal-cleanup
// machinery. LockSessionByUUID/ClaimSessionRequest/CompleteSessionRequest
// are this workflow's own locking/idempotency-claim mechanics, not a
// domain-wide protocol.
type cancelSessionRepoAPI interface {
	LockSessionByUUID(ctx context.Context, tx *gorm.DB, sessionUUID string) (*internalrepo.Session, error)
	FindActor(ctx context.Context, tx *gorm.DB, sessionID uint, userUUID string) (*internalrepo.Actor, error)
	ListActiveParticipantsForRoster(ctx context.Context, tx *gorm.DB, sessionID uint) ([]internalrepo.RosterParticipant, error)
	GetRuntimeTurn(ctx context.Context, tx *gorm.DB, turnID uint) (*internalrepo.RuntimeTurn, error)
	GetGameVersionArtifact(ctx context.Context, tx *gorm.DB, definitionUUID string) (*internalrepo.GameVersionArtifact, error)
	CreateRuntimeTurn(ctx context.Context, tx *gorm.DB, sessionID uint, sequence uint64, sourceKind string, sourceTimerObligationID *uint, sourceCauseEventID *uint, actorID *uint, newState json.RawMessage) (uint, error)
	CreateCauseEvent(ctx context.Context, tx *gorm.DB, sessionID uint, runtimeTurnID uint, causeKind string, actorID *uint, payload []byte) (uint, error)
	SetRuntimeTurnCauseEvent(ctx context.Context, tx *gorm.DB, turnID uint, causeEventID uint) error
	SetCurrentTurn(ctx context.Context, tx *gorm.DB, sessionID uint, currentTurnID uint) error
	SetSessionTerminal(ctx context.Context, tx *gorm.DB, sessionID uint, terminalAt time.Time, terminalReason string) error
	CreateRuntimeFailure(ctx context.Context, tx *gorm.DB, sessionID uint, failureKind string, errorCode string, errorMessage string, baseTurnID *uint, attemptedSequence uint64, sourceKind string, sourceTimerObligationID *uint, actorID *uint, diagnosticPayload []byte) error
	CancelAllActiveTimerObligationsForSession(ctx context.Context, tx *gorm.DB, sessionID uint, reason string) error
	CreateTimerObligation(ctx context.Context, tx *gorm.DB, sessionID uint, timer string, data []byte, delayMs int64, createdByTurnID uint) (uint, error)
	CancelActiveTimerObligation(ctx context.Context, tx *gorm.DB, sessionID uint, timer string, closedByTurnID uint) error
	ClaimSessionRequest(ctx context.Context, tx *gorm.DB, input internalrepo.ClaimSessionRequestInput) (requestID uint, existing *internalrepo.Request, err error)
	CompleteSessionRequest(ctx context.Context, tx *gorm.DB, requestID uint, sessionID *uint, outcome string, responsePayload string) error
}

// cancelSessionRequestPayload is CancelSession's meaningful-field
// idempotency payload, mirroring startRequestPayload's shape.
type cancelSessionRequestPayload struct {
	SessionUUID string `json:"session_uuid"`
	UserUUID    string `json:"user_uuid"`
}

// CancelSession lets sessionUUID's own host explicitly end a RUNNING
// Session before it would otherwise reach a terminal state on its own: it
// delivers a SESSION_CANCELLED Event to the authored script and always
// terminalizes the Session once past authorization - whether because the
// script itself reacts to it (requesting SESSION_COMPLETE/SESSION_FAIL, or
// simply not rejecting it) or because it rejects it outright. A host
// cancel is an administrative override, not gameplay input, so - unlike
// SubmitPlayerEvent/ExpireTimer - it is never a silent no-op merely because
// the script rejects the event.
//
// Like SubmitPlayerEvent, a cancellation has no pre-existing target row to
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

	return utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (session.CancelSessionResult, error) {
		return m.cancelSessionInTx(ctx, tx, sessionUUID, userUUID, idempotencyKey)
	})
}

func (m *Manager) cancelSessionInTx(ctx context.Context, tx *gorm.DB, sessionUUID session.SessionUUID, userUUID session.UserUUID, idempotencyKey session.IdempotencyKey) (session.CancelSessionResult, error) {
	incomingPayload := cancelSessionRequestPayload{SessionUUID: string(sessionUUID), UserUUID: string(userUUID)}

	lockedSession, err := m.cancelSessionRepo.LockSessionByUUID(ctx, tx, string(sessionUUID))
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
	if _, err := m.activityExpirer.MaterializeIfDue(ctx, tx, lockedSession, time.Now().UTC()); err != nil {
		return session.CancelSessionResult{}, err
	}

	payloadBytes, err := json.Marshal(incomingPayload)
	if err != nil {
		return session.CancelSessionResult{}, fmt.Errorf("marshaling cancel session request payload: %s", err)
	}
	requestID, existing, err := m.cancelSessionRepo.ClaimSessionRequest(ctx, tx, internalrepo.ClaimSessionRequestInput{
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
	// LOBBY has no running script instance yet to deliver an event to.
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

	artifact, err := m.cancelSessionRepo.GetGameVersionArtifact(ctx, tx, lockedSession.GameDefinitionUUID)
	if err != nil {
		return session.CancelSessionResult{}, err
	}
	if artifact == nil {
		monitoring.Alert(ctx, "session pinned game version artifact is missing")
		return session.CancelSessionResult{}, session.ErrPinnedDefinitionMissing
	}

	roster, err := m.cancelSessionRepo.ListActiveParticipantsForRoster(ctx, tx, lockedSession.ID)
	if err != nil {
		return session.CancelSessionResult{}, err
	}
	known := knownActorsFromRoster(roster)

	event := platform.NewSessionCancelled()
	encodedEvent, err := platform.EncodeEvent(event)
	if err != nil {
		return session.CancelSessionResult{}, fmt.Errorf("encoding session cancelled event: %s", err)
	}

	output, err := m.executor.Execute(ctx, executor.ExecutionInput{
		Script:        executor.ResolvedScript{Source: artifact.BackendScript},
		PreviousState: currentTurn.NewState,
		Event:         encodedEvent,
		Context:       executor.ExecutionContext{LogicalTime: now, RandomSeed: drawSeed(), ActingActor: string(actorRefForActorID(actor.ID))},
	})
	if err != nil {
		var rejected *executor.ScriptRejectedError
		if errors.As(err, &rejected) {
			// No reaction matches SESSION_CANCELLED at all. A host cancel
			// still always ends the Session (the human-approved
			// force-terminal decision) - but nothing here was ever accepted
			// by the script, so no RuntimeTurn/cause event is created:
			// there is no new state to persist, and the prior state remains
			// exactly what it was.
			return m.terminalizeCancelSessionForced(ctx, tx, requestID, lockedSession, now)
		}
		errorCode, errorMessage := classifyExecutionError(err)
		return m.terminalizeCancelSessionFatal(ctx, tx, requestID, lockedSession, now, session.TerminalReasonRuntimeExecutionFailed,
			RuntimeFailureKindExecution, errorCode, errorMessage, &currentTurn.ID, currentTurn.Sequence+1, actor.ID)
	}

	commands, err := parseCommands(output.RequestedCommands, known)
	if err != nil {
		return m.terminalizeCancelSessionForced(ctx, tx, requestID, lockedSession, now)
	}

	// Accepted: the authored script reacted to SESSION_CANCELLED.
	// session_cause_events.runtime_turn_id is NOT NULL, so the cause event
	// row can only be created after the Turn it belongs to already exists -
	// its own pointer back to it is then backfilled, exactly like
	// SubmitPlayerEvent's own cause persistence.
	actorID := actor.ID
	turnID, err := m.cancelSessionRepo.CreateRuntimeTurn(ctx, tx, lockedSession.ID, currentTurn.Sequence+1, sessionCancelledSourceKind, nil, nil, &actorID, output.NewState)
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
	if err := m.cancelSessionRepo.SetCurrentTurn(ctx, tx, lockedSession.ID, turnID); err != nil {
		return session.CancelSessionResult{}, err
	}
	if err := timers.Apply(ctx, tx, m.cancelSessionRepo, lockedSession.ID, turnID, commands); err != nil {
		return session.CancelSessionResult{}, err
	}

	// Force-terminal always: reuse the script's own completion request when
	// it made one (TerminalReasonGame*); otherwise the accepted reaction
	// moved state without ending the run, so this call terminalizes it
	// anyway under the new TerminalReasonSessionCancelledByHost. Either way,
	// a CancelSession call that reaches this point always ends the Session -
	// the terminal-cleanup steps below always run, unlike every other
	// signal-driven capability's conditional "if terminated" branch.
	terminalReason, terminated := completion.Detect(commands)
	if !terminated {
		terminalReason = session.TerminalReasonSessionCancelledByHost
	}
	if err := m.cancelSessionRepo.SetSessionTerminal(ctx, tx, lockedSession.ID, now, terminalReason); err != nil {
		return session.CancelSessionResult{}, err
	}
	if err := m.cancelSessionRepo.CancelAllActiveTimerObligationsForSession(ctx, tx, lockedSession.ID, session.TimerObligationClosureReasonSessionTerminated); err != nil {
		return session.CancelSessionResult{}, err
	}

	result := session.CancelSessionResult{Outcome: session.CancelSessionOutcomeCancelled, SessionUUID: session.SessionUUID(lockedSession.UUID), TerminalReason: terminalReason, Events: collectOutboundEvents(commands)}
	responseBytes, err := json.Marshal(result)
	if err != nil {
		return session.CancelSessionResult{}, fmt.Errorf("marshaling cancel session response payload: %s", err)
	}
	if err := m.cancelSessionRepo.CompleteSessionRequest(ctx, tx, requestID, &lockedSession.ID, cancelSessionOutcomeCancelled, string(responseBytes)); err != nil {
		return session.CancelSessionResult{}, fmt.Errorf("completing cancel session request: %s", err)
	}
	return result, nil
}

// declineCancelSession completes requestID as an ordinary decline (no
// execution effect, no state change) and reports resultOutcome, mirroring
// declineSubmitPlayerEvent's shape.
func (m *Manager) declineCancelSession(ctx context.Context, tx *gorm.DB, requestID uint, lockedSession *internalrepo.Session, outcomeLabel string, resultOutcome session.CancelSessionOutcome) (session.CancelSessionResult, error) {
	if err := m.cancelSessionRepo.CompleteSessionRequest(ctx, tx, requestID, &lockedSession.ID, outcomeLabel, ""); err != nil {
		return session.CancelSessionResult{}, fmt.Errorf("completing cancel session request: %s", err)
	}
	return session.CancelSessionResult{Outcome: resultOutcome, SessionUUID: session.SessionUUID(lockedSession.UUID)}, nil
}

// terminalizeCancelSessionForced performs CancelSession's forced-terminal
// path for a SESSION_CANCELLED event the script rejected outright (no
// RuntimeTurn, current_turn_id unchanged) - see cancelSessionInTx's own
// ScriptRejectedError/parse-failure handling for why no cause is persisted.
// Outcome is still CancelSessionOutcomeCancelled: the Session ends either
// way.
func (m *Manager) terminalizeCancelSessionForced(ctx context.Context, tx *gorm.DB, requestID uint, lockedSession *internalrepo.Session, terminalAt time.Time) (session.CancelSessionResult, error) {
	if err := m.cancelSessionRepo.SetSessionTerminal(ctx, tx, lockedSession.ID, terminalAt, session.TerminalReasonSessionCancelledByHost); err != nil {
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
	if err := m.cancelSessionRepo.CompleteSessionRequest(ctx, tx, requestID, &lockedSession.ID, cancelSessionOutcomeCancelled, string(responseBytes)); err != nil {
		return session.CancelSessionResult{}, fmt.Errorf("completing cancel session request: %s", err)
	}
	return result, nil
}

// terminalizeCancelSessionFatal performs CancelSession's fatal path:
// atomically terminalizes the Session, persists the session_runtime_failures
// diagnostic record, and cancels every currently-ACTIVE timer obligation
// for it, mirroring terminalizeSubmitPlayerEventFatal exactly, and
// completes the idempotency claim so a retry replays this same outcome
// instead of re-attempting a doomed execution. Distinct from
// terminalizeCancelSessionForced above: this is a genuine infrastructure
// failure, not the host's own intentional cancellation, so only this path
// persists a session_runtime_failures row.
func (m *Manager) terminalizeCancelSessionFatal(ctx context.Context, tx *gorm.DB, requestID uint, lockedSession *internalrepo.Session, terminalAt time.Time, terminalReason string, failureKind string, errorCode string, errorMessage string, baseTurnID *uint, attemptedSequence uint64, actorID uint) (session.CancelSessionResult, error) {
	if err := m.materializeRuntimeFailure(ctx, tx, m.cancelSessionRepo, lockedSession, terminalAt, terminalReason,
		failureKind, errorCode, errorMessage, baseTurnID, attemptedSequence, sessionCancelledSourceKind, nil, &actorID); err != nil {
		return session.CancelSessionResult{}, err
	}
	if err := m.cancelSessionRepo.CompleteSessionRequest(ctx, tx, requestID, &lockedSession.ID, cancelSessionOutcomeRuntimeExecutionFailed, ""); err != nil {
		return session.CancelSessionResult{}, fmt.Errorf("completing cancel session request: %s", err)
	}
	return session.CancelSessionResult{Outcome: session.CancelSessionOutcomeRuntimeExecutionFailed, SessionUUID: session.SessionUUID(lockedSession.UUID)}, nil
}

// interpretExistingCancelSessionClaim replays an already-completed
// idempotency claim's original outcome, mirroring
// interpretExistingSubmitPlayerEventClaim exactly.
func interpretExistingCancelSessionClaim(existing *internalrepo.Request, incoming cancelSessionRequestPayload) (session.CancelSessionResult, error) {
	if existing.Status != internalrepo.RequestStatusCompleted {
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
