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

// Submit player event outcome labels persisted to session_requests.outcome
// for a deterministic post-claim decline that must survive as a replayable
// outcome - the same technique Start's own outcome* constants use.
const (
	submitPlayerEventOutcomeAccepted               = "ACCEPTED"
	submitPlayerEventOutcomeRejected               = "REJECTED"
	submitPlayerEventOutcomeRuntimeExecutionFailed = "RUNTIME_EXECUTION_FAILED"
)

// submitPlayerEventRepoAPI is SubmitPlayerEvent's own narrow persistence
// contract: a game-defined player action is submitted directly against the
// Session by name+opaque payload, with no platform-level "open interaction"
// concept for a response to answer. Idempotency/dedup is IdempotencyKey-
// based, the same mechanism Start/CancelSession/Create already use.
// LockSessionByUUID/ClaimSessionRequest/CompleteSessionRequest are this
// workflow's own locking/idempotency-claim mechanics, not a domain-wide
// protocol.
type submitPlayerEventRepoAPI interface {
	LockSessionByUUID(ctx context.Context, tx *gorm.DB, sessionUUID string) (*internalrepo.Session, error)
	FindActor(ctx context.Context, tx *gorm.DB, sessionID uint, userUUID string) (*internalrepo.Actor, error)
	ListActiveParticipantsForRoster(ctx context.Context, tx *gorm.DB, sessionID uint) ([]internalrepo.RosterParticipant, error)
	GetRuntimeTurn(ctx context.Context, tx *gorm.DB, turnID uint) (*internalrepo.RuntimeTurn, error)
	GetGameVersionArtifact(ctx context.Context, tx *gorm.DB, definitionUUID string) (*internalrepo.GameVersionArtifact, error)
	CreateRuntimeTurn(ctx context.Context, tx *gorm.DB, sessionID uint, sequence uint64, sourceKind string, sourceTimerObligationID *uint, sourceCauseEventID *uint, actorID *uint, newState json.RawMessage) (uint, error)
	CreateCauseEvent(ctx context.Context, tx *gorm.DB, sessionID uint, runtimeTurnID uint, causeKind string, actorID *uint, payload []byte) (uint, error)
	SetRuntimeTurnCauseEvent(ctx context.Context, tx *gorm.DB, turnID uint, causeEventID uint) error
	SetCurrentTurn(ctx context.Context, tx *gorm.DB, sessionID uint, currentTurnID uint) error
	RenewActivityDeadline(ctx context.Context, tx *gorm.DB, sessionID uint, activityExpiresAt time.Time) error
	SetSessionTerminal(ctx context.Context, tx *gorm.DB, sessionID uint, terminalAt time.Time, terminalReason string) error
	CreateRuntimeFailure(ctx context.Context, tx *gorm.DB, sessionID uint, failureKind string, errorCode string, errorMessage string, baseTurnID *uint, attemptedSequence uint64, sourceKind string, sourceTimerObligationID *uint, actorID *uint, diagnosticPayload []byte) error
	CancelAllActiveTimerObligationsForSession(ctx context.Context, tx *gorm.DB, sessionID uint, reason string) error
	CreateTimerObligation(ctx context.Context, tx *gorm.DB, sessionID uint, timer string, data []byte, delayMs int64, createdByTurnID uint) (uint, error)
	CancelActiveTimerObligation(ctx context.Context, tx *gorm.DB, sessionID uint, timer string, closedByTurnID uint) error
	ClaimSessionRequest(ctx context.Context, tx *gorm.DB, input internalrepo.ClaimSessionRequestInput) (requestID uint, existing *internalrepo.Request, err error)
	CompleteSessionRequest(ctx context.Context, tx *gorm.DB, requestID uint, sessionID *uint, outcome string, responsePayload string) error
}

// submitPlayerEventRequestPayload is SUBMIT_PLAYER_EVENT's meaningful-field
// idempotency payload. Payload is stored as the raw JSON string (not
// json.RawMessage/[]byte) specifically so this struct stays comparable with
// == /!=, the same technique interpretExistingStartClaim relies on for
// startRequestPayload.
type submitPlayerEventRequestPayload struct {
	SessionUUID string `json:"session_uuid"`
	UserUUID    string `json:"user_uuid"`
	Name        string `json:"name"`
	Payload     string `json:"payload"`
}

// SubmitPlayerEvent submits a game-defined player action, name+payload,
// against a RUNNING sessionUUID. name/payload are entirely game-defined and
// opaque to Playhoot - any validation of what payload must contain is the
// authored backend script's own job, never Session Runtime's.
//
// A submitted player event has no existing durable row to dedup against -
// it is genuinely unsolicited - so idempotencyKey is required, following
// Start/Join/Leave/CancelSession's own idempotency-based dedup.
func (m *Manager) SubmitPlayerEvent(ctx context.Context, sessionUUID session.SessionUUID, userUUID session.UserUUID, name string, payload json.RawMessage, idempotencyKey session.IdempotencyKey) (session.SubmitPlayerEventResult, error) {
	defer logging.Step(ctx, "SessionLifecycle.SubmitPlayerEvent").Close()
	logging.LogFields(ctx,
		logging.Field("session_uuid", string(sessionUUID)),
		logging.Field("user_uuid", string(userUUID)),
		logging.Field("name", name),
	)

	if idempotencyKey == "" {
		return session.SubmitPlayerEventResult{}, session.ErrIdempotencyKeyRequired
	}

	return utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (session.SubmitPlayerEventResult, error) {
		return m.submitPlayerEventInTx(ctx, tx, sessionUUID, userUUID, name, payload, idempotencyKey)
	})
}

func (m *Manager) submitPlayerEventInTx(ctx context.Context, tx *gorm.DB, sessionUUID session.SessionUUID, userUUID session.UserUUID, name string, payload json.RawMessage, idempotencyKey session.IdempotencyKey) (session.SubmitPlayerEventResult, error) {
	incomingPayload := submitPlayerEventRequestPayload{
		SessionUUID: string(sessionUUID),
		UserUUID:    string(userUUID),
		Name:        name,
		Payload:     string(payload),
	}

	lockedSession, err := m.submitPlayerEventRepo.LockSessionByUUID(ctx, tx, string(sessionUUID))
	if err != nil {
		return session.SubmitPlayerEventResult{}, err
	}
	if lockedSession == nil {
		return session.SubmitPlayerEventResult{}, session.ErrSessionNotFound
	}

	// A materialized inactivity expiration leaves the existing
	// lockedSession.Phase != PhaseRunning check below to naturally decline
	// this call via declineSubmitPlayerEvent - no separate branch is needed.
	if _, err := m.activityExpirer.MaterializeIfDue(ctx, tx, lockedSession, time.Now().UTC()); err != nil {
		return session.SubmitPlayerEventResult{}, err
	}

	payloadBytes, err := json.Marshal(incomingPayload)
	if err != nil {
		return session.SubmitPlayerEventResult{}, fmt.Errorf("marshaling submit player event request payload: %s", err)
	}
	requestID, existing, err := m.submitPlayerEventRepo.ClaimSessionRequest(ctx, tx, internalrepo.ClaimSessionRequestInput{
		Operation:      operationSubmitPlayerEvent,
		UserUUID:       string(userUUID),
		IdempotencyKey: string(idempotencyKey),
		SessionID:      &lockedSession.ID,
		RequestPayload: string(payloadBytes),
	})
	if err != nil {
		return session.SubmitPlayerEventResult{}, fmt.Errorf("claiming submit player event request: %s", err)
	}
	if existing != nil {
		return interpretExistingSubmitPlayerEventClaim(existing, incomingPayload)
	}

	// A TERMINAL Session is never further mutated - a submitted event
	// addresses no existing row the way an interaction/obligation UUID
	// would, so nothing else stops it from reaching Execute against an
	// already-TERMINAL Session. This explicit phase check is therefore
	// required, mirroring SubmitUserIntent's own reasoning exactly.
	if lockedSession.Phase != session.PhaseRunning {
		return m.declineSubmitPlayerEvent(ctx, tx, requestID, lockedSession)
	}

	// A missing actor is indistinguishable from "not a current Participant"
	// for this purpose, the same reasoning every other RUNNING-phase step
	// already applies to its own actor lookup.
	actor, err := m.submitPlayerEventRepo.FindActor(ctx, tx, lockedSession.ID, string(userUUID))
	if err != nil {
		return session.SubmitPlayerEventResult{}, err
	}
	if actor == nil {
		return m.declineSubmitPlayerEvent(ctx, tx, requestID, lockedSession)
	}

	artifact, err := m.submitPlayerEventRepo.GetGameVersionArtifact(ctx, tx, lockedSession.GameDefinitionUUID)
	if err != nil {
		return session.SubmitPlayerEventResult{}, err
	}
	if artifact == nil {
		monitoring.Alert(ctx, "session pinned game version artifact is missing")
		return session.SubmitPlayerEventResult{}, session.ErrPinnedDefinitionMissing
	}

	if lockedSession.CurrentTurnID == nil {
		monitoring.Alert(ctx, "session has no current_turn_id")
		return session.SubmitPlayerEventResult{}, fmt.Errorf("session %d has no current_turn_id", lockedSession.ID)
	}
	currentTurn, err := m.submitPlayerEventRepo.GetRuntimeTurn(ctx, tx, *lockedSession.CurrentTurnID)
	if err != nil {
		return session.SubmitPlayerEventResult{}, err
	}
	if currentTurn == nil {
		monitoring.Alert(ctx, "session current_turn_id does not resolve to a runtime turn")
		return session.SubmitPlayerEventResult{}, fmt.Errorf("session %d current_turn_id %d does not resolve to a runtime turn", lockedSession.ID, *lockedSession.CurrentTurnID)
	}

	roster, err := m.submitPlayerEventRepo.ListActiveParticipantsForRoster(ctx, tx, lockedSession.ID)
	if err != nil {
		return session.SubmitPlayerEventResult{}, err
	}
	known := knownActorsFromRoster(roster)

	now := time.Now().UTC()
	actorRef := actorRefForActorID(actor.ID)
	event := platform.NewPlayerEvent(actorRef, name, payload)
	encodedEvent, err := platform.EncodeEvent(event)
	if err != nil {
		return session.SubmitPlayerEventResult{}, fmt.Errorf("encoding player event: %s", err)
	}

	output, err := m.executor.Execute(ctx, executor.ExecutionInput{
		Script:        executor.ResolvedScript{Source: artifact.BackendScript},
		PreviousState: currentTurn.NewState,
		Event:         encodedEvent,
		Context:       executor.ExecutionContext{LogicalTime: now, RandomSeed: drawSeed(), ActingActor: string(actorRef)},
	})
	if err != nil {
		var rejected *executor.ScriptRejectedError
		if errors.As(err, &rejected) {
			// A rejection of the event itself is an ordinary declined
			// outcome: no RuntimeTurn, no state mutation, Session stays
			// RUNNING.
			return m.declineSubmitPlayerEvent(ctx, tx, requestID, lockedSession)
		}
		errorCode, errorMessage := classifyExecutionError(err)
		return m.terminalizeSubmitPlayerEventFatal(ctx, tx, requestID, lockedSession, now, session.TerminalReasonRuntimeExecutionFailed,
			RuntimeFailureKindExecution, errorCode, errorMessage, &currentTurn.ID, currentTurn.Sequence+1, actor.ID)
	}

	commands, err := parseCommands(output.RequestedCommands, known)
	if err != nil {
		// A parse/validation failure is itself treated as an ordinary
		// decline: the authored script's own defect, not an infrastructure
		// failure.
		return m.declineSubmitPlayerEvent(ctx, tx, requestID, lockedSession)
	}

	actorID := actor.ID
	turnID, err := m.submitPlayerEventRepo.CreateRuntimeTurn(ctx, tx, lockedSession.ID, currentTurn.Sequence+1, playerEventSourceKind, nil, nil, &actorID, output.NewState)
	if err != nil {
		return session.SubmitPlayerEventResult{}, err
	}
	// session_cause_events.runtime_turn_id is NOT NULL, so the cause event
	// row can only be created after the Turn it belongs to already exists -
	// see CreateRuntimeTurn's own doc comment. The Turn's own pointer back
	// to it is then backfilled.
	causeEventID, err := m.submitPlayerEventRepo.CreateCauseEvent(ctx, tx, lockedSession.ID, turnID, "PLAYER_EVENT", &actorID, encodedEvent)
	if err != nil {
		return session.SubmitPlayerEventResult{}, err
	}
	if err := m.submitPlayerEventRepo.SetRuntimeTurnCauseEvent(ctx, tx, turnID, causeEventID); err != nil {
		return session.SubmitPlayerEventResult{}, err
	}
	if err := m.submitPlayerEventRepo.SetCurrentTurn(ctx, tx, lockedSession.ID, turnID); err != nil {
		return session.SubmitPlayerEventResult{}, err
	}
	if err := m.submitPlayerEventRepo.RenewActivityDeadline(ctx, tx, lockedSession.ID, now.Add(m.activityTTL)); err != nil {
		return session.SubmitPlayerEventResult{}, err
	}
	if err := timers.Apply(ctx, tx, m.submitPlayerEventRepo, lockedSession.ID, turnID, commands); err != nil {
		return session.SubmitPlayerEventResult{}, err
	}

	terminalReason, terminated := completion.Detect(commands)
	if terminated {
		if err := m.submitPlayerEventRepo.SetSessionTerminal(ctx, tx, lockedSession.ID, now, terminalReason); err != nil {
			return session.SubmitPlayerEventResult{}, err
		}
		if err := m.submitPlayerEventRepo.CancelAllActiveTimerObligationsForSession(ctx, tx, lockedSession.ID, session.TimerObligationClosureReasonSessionTerminated); err != nil {
			return session.SubmitPlayerEventResult{}, err
		}
	}

	result := session.SubmitPlayerEventResult{Outcome: session.SubmitPlayerEventOutcomeAccepted, SessionUUID: session.SessionUUID(lockedSession.UUID), TerminalReason: terminalReason}
	responseBytes, err := json.Marshal(result)
	if err != nil {
		return session.SubmitPlayerEventResult{}, fmt.Errorf("marshaling submit player event response payload: %s", err)
	}
	if err := m.submitPlayerEventRepo.CompleteSessionRequest(ctx, tx, requestID, &lockedSession.ID, submitPlayerEventOutcomeAccepted, string(responseBytes)); err != nil {
		return session.SubmitPlayerEventResult{}, fmt.Errorf("completing submit player event request: %s", err)
	}
	return result, nil
}

// declineSubmitPlayerEvent completes requestID as an ordinary decline (no
// execution effect, no RuntimeTurn) and reports SubmitPlayerEventOutcomeRejected.
func (m *Manager) declineSubmitPlayerEvent(ctx context.Context, tx *gorm.DB, requestID uint, lockedSession *internalrepo.Session) (session.SubmitPlayerEventResult, error) {
	if err := m.submitPlayerEventRepo.CompleteSessionRequest(ctx, tx, requestID, &lockedSession.ID, submitPlayerEventOutcomeRejected, ""); err != nil {
		return session.SubmitPlayerEventResult{}, fmt.Errorf("completing submit player event request: %s", err)
	}
	return session.SubmitPlayerEventResult{Outcome: session.SubmitPlayerEventOutcomeRejected, SessionUUID: session.SessionUUID(lockedSession.UUID)}, nil
}

// terminalizeSubmitPlayerEventFatal performs SubmitPlayerEvent's fatal path:
// atomically terminalizes the Session, persists the session_runtime_failures
// diagnostic record, and cancels every currently-ACTIVE timer obligation for
// it, and completes the idempotency claim so a retry replays this same
// outcome instead of re-attempting a doomed execution.
func (m *Manager) terminalizeSubmitPlayerEventFatal(ctx context.Context, tx *gorm.DB, requestID uint, lockedSession *internalrepo.Session, terminalAt time.Time, terminalReason string, failureKind string, errorCode string, errorMessage string, baseTurnID *uint, attemptedSequence uint64, actorID uint) (session.SubmitPlayerEventResult, error) {
	if err := m.materializeRuntimeFailure(ctx, tx, m.submitPlayerEventRepo, lockedSession, terminalAt, terminalReason,
		failureKind, errorCode, errorMessage, baseTurnID, attemptedSequence, playerEventSourceKind, nil, &actorID); err != nil {
		return session.SubmitPlayerEventResult{}, err
	}
	if err := m.submitPlayerEventRepo.CompleteSessionRequest(ctx, tx, requestID, &lockedSession.ID, submitPlayerEventOutcomeRuntimeExecutionFailed, ""); err != nil {
		return session.SubmitPlayerEventResult{}, fmt.Errorf("completing submit player event request: %s", err)
	}
	return session.SubmitPlayerEventResult{Outcome: session.SubmitPlayerEventOutcomeRuntimeExecutionFailed, SessionUUID: session.SessionUUID(lockedSession.UUID)}, nil
}

// interpretExistingSubmitPlayerEventClaim replays an already-completed
// idempotency claim's original outcome, mirroring
// interpretExistingStartClaim exactly.
func interpretExistingSubmitPlayerEventClaim(existing *internalrepo.Request, incoming submitPlayerEventRequestPayload) (session.SubmitPlayerEventResult, error) {
	if existing.Status != internalrepo.RequestStatusCompleted {
		return session.SubmitPlayerEventResult{}, session.ErrIdempotencyInFlight
	}

	var stored submitPlayerEventRequestPayload
	if err := json.Unmarshal([]byte(existing.RequestPayload), &stored); err != nil {
		return session.SubmitPlayerEventResult{}, fmt.Errorf("decoding stored submit player event request payload: %s", err)
	}
	if stored != incoming {
		return session.SubmitPlayerEventResult{}, session.ErrIdempotencyConflict
	}

	switch existing.Outcome {
	case submitPlayerEventOutcomeRejected:
		return session.SubmitPlayerEventResult{Outcome: session.SubmitPlayerEventOutcomeRejected}, nil
	case submitPlayerEventOutcomeRuntimeExecutionFailed:
		return session.SubmitPlayerEventResult{Outcome: session.SubmitPlayerEventOutcomeRuntimeExecutionFailed}, nil
	}

	if existing.ResponsePayload == nil {
		return session.SubmitPlayerEventResult{}, fmt.Errorf("completed submit player event idempotency record missing response payload")
	}
	var result session.SubmitPlayerEventResult
	if err := json.Unmarshal([]byte(*existing.ResponsePayload), &result); err != nil {
		return session.SubmitPlayerEventResult{}, fmt.Errorf("decoding stored submit player event response payload: %s", err)
	}
	return result, nil
}

// parseCommands parses every element of raw via platform.ParseCommand,
// known validating actor references - shared by every RUNNING-phase step
// that calls Execute.
func parseCommands(raw []json.RawMessage, known platform.KnownActors) ([]platform.Command, error) {
	commands := make([]platform.Command, len(raw))
	for i, r := range raw {
		command, err := platform.ParseCommand(r, known)
		if err != nil {
			return nil, err
		}
		commands[i] = command
	}
	return commands, nil
}
