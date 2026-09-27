package sessionlifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
	"github.com/diegobermudez03/playhoot/game/language/v1/program"
	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/clientoutputs"
	"github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/completion"
	"github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/replay"
	internalrepo "github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	"github.com/diegobermudez03/playhoot/utils"
	"gorm.io/gorm"
)

// Submit user intent outcome labels persisted to session_requests.outcome
// for a deterministic post-claim decline that must survive as a replayable
// outcome - the same technique Start's own outcome* constants use.
const (
	submitUserIntentOutcomeAccepted               = "ACCEPTED"
	submitUserIntentOutcomeRejected               = "REJECTED"
	submitUserIntentOutcomeRuntimeExecutionFailed = "RUNTIME_EXECUTION_FAILED"
)

// submitUserIntentRepoAPI is SubmitUserIntent's own narrow persistence
// contract. LockSessionByUUID/ClaimSessionRequest/CompleteSessionRequest are
// this workflow's own locking/idempotency-claim mechanics, not a domain-wide
// protocol.
type submitUserIntentRepoAPI interface {
	LockSessionByUUID(ctx context.Context, tx *gorm.DB, sessionUUID string) (*internalrepo.Session, error)
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
	RenewActivityDeadline(ctx context.Context, tx *gorm.DB, sessionID uint, activityExpiresAt time.Time) error
	SetSessionTerminal(ctx context.Context, tx *gorm.DB, sessionID uint, terminalAt time.Time, terminalReason string) error
	CreateRuntimeFailure(ctx context.Context, tx *gorm.DB, sessionID uint, failureKind string, errorCode string, errorMessage string, baseTurnID *uint, attemptedSequence uint64, sourceKind string, sourceInteractionID *uint, sourceTimerObligationID *uint, actorID *uint, diagnosticPayload []byte) error
	CloseAllActiveInteractionsForSession(ctx context.Context, tx *gorm.DB, sessionID uint, reason string) error
	CancelAllActiveTimerObligationsForSession(ctx context.Context, tx *gorm.DB, sessionID uint, reason string) error
	ClaimSessionRequest(ctx context.Context, tx *gorm.DB, input internalrepo.ClaimSessionRequestInput) (requestID uint, existing *internalrepo.Request, err error)
	CompleteSessionRequest(ctx context.Context, tx *gorm.DB, requestID uint, sessionID *uint, outcome string, responsePayload string) error
}

// submitUserIntentRequestPayload is SubmitUserIntent's meaningful-field
// idempotency payload. Arguments is stored as the raw JSON string (not
// json.RawMessage/[]byte) specifically so this struct stays comparable with
// == /!=, the same technique interpretExistingStartClaim relies on for
// startRequestPayload.
type submitUserIntentRequestPayload struct {
	SessionUUID string `json:"session_uuid"`
	UserUUID    string `json:"user_uuid"`
	IntentName  string `json:"intent_name"`
	Arguments   string `json:"arguments"`
}

// SubmitUserIntent submits an unsolicited player-initiated intentName
// against a RUNNING sessionUUID, with arguments encoded as plain JSON: a
// JSON object keyed by parameter name matching the intent's declared
// Parameters (or an empty object/omitted for a zero-parameter intent) - the
// same encoding technique AnswerInteraction's own answer parameter uses.
//
// Unlike AnswerInteraction, a submitted intent has no existing durable row
// to dedup against - it is genuinely unsolicited - so idempotencyKey is
// required, following Start/Join/Leave's own idempotency-based dedup
// instead.
func (m *Manager) SubmitUserIntent(ctx context.Context, sessionUUID session.SessionUUID, userUUID session.UserUUID, intentName string, arguments json.RawMessage, idempotencyKey session.IdempotencyKey) (session.SubmitUserIntentResult, error) {
	defer logging.Step(ctx, "SessionLifecycle.SubmitUserIntent").Close()
	logging.LogFields(ctx,
		logging.Field("session_uuid", string(sessionUUID)),
		logging.Field("user_uuid", string(userUUID)),
		logging.Field("intent", intentName),
	)

	if idempotencyKey == "" {
		return session.SubmitUserIntentResult{}, session.ErrIdempotencyKeyRequired
	}

	return utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (session.SubmitUserIntentResult, error) {
		return m.submitUserIntentInTx(ctx, tx, sessionUUID, userUUID, intentName, arguments, idempotencyKey)
	})
}

func (m *Manager) submitUserIntentInTx(ctx context.Context, tx *gorm.DB, sessionUUID session.SessionUUID, userUUID session.UserUUID, intentName string, arguments json.RawMessage, idempotencyKey session.IdempotencyKey) (session.SubmitUserIntentResult, error) {
	incomingPayload := submitUserIntentRequestPayload{
		SessionUUID: string(sessionUUID),
		UserUUID:    string(userUUID),
		IntentName:  intentName,
		Arguments:   string(arguments),
	}

	lockedSession, err := m.submitUserIntentRepo.LockSessionByUUID(ctx, tx, string(sessionUUID))
	if err != nil {
		return session.SubmitUserIntentResult{}, err
	}
	if lockedSession == nil {
		return session.SubmitUserIntentResult{}, session.ErrSessionNotFound
	}

	// A materialized inactivity expiration leaves the existing
	// lockedSession.Phase != PhaseRunning check below to naturally decline
	// this call via declineSubmitUserIntent - no separate branch is needed.
	if _, err := m.activityExpirer.MaterializeIfDue(ctx, tx, lockedSession, time.Now().UTC()); err != nil {
		return session.SubmitUserIntentResult{}, err
	}

	payloadBytes, err := json.Marshal(incomingPayload)
	if err != nil {
		return session.SubmitUserIntentResult{}, fmt.Errorf("marshaling submit user intent request payload: %s", err)
	}
	requestID, existing, err := m.submitUserIntentRepo.ClaimSessionRequest(ctx, tx, internalrepo.ClaimSessionRequestInput{
		Operation:      operationSubmitUserIntent,
		UserUUID:       string(userUUID),
		IdempotencyKey: string(idempotencyKey),
		SessionID:      &lockedSession.ID,
		RequestPayload: string(payloadBytes),
	})
	if err != nil {
		return session.SubmitUserIntentResult{}, fmt.Errorf("claiming submit user intent request: %s", err)
	}
	if existing != nil {
		return interpretExistingSubmitUserIntentClaim(existing, incomingPayload)
	}

	// Unlike AnswerInteraction, which is transitively protected against a
	// TERMINAL Session because terminalization always closes every ACTIVE
	// interaction it could otherwise target, a submitted intent addresses
	// no existing row - nothing else stops it from reaching AdvanceTurn
	// against an already-TERMINAL Session whose underlying engine instance
	// never itself reached a Completed/Failed/Cancelled run status (the
	// fatal RUNTIME_EXECUTION_FAILED/RUNTIME_STATE_INVALID paths terminalize
	// the Session without the engine instance ever producing a
	// RunCompletedOutput). This explicit phase check is therefore required,
	// not merely defensive, to preserve the already-accepted invariant that
	// a TERMINAL Session is never further mutated.
	if lockedSession.Phase != session.PhaseRunning {
		return m.declineSubmitUserIntent(ctx, tx, requestID, lockedSession)
	}

	// A missing actor is indistinguishable from "not a current Participant"
	// for this purpose, the same reasoning AnswerInteraction/Start already
	// apply to their own actor lookups.
	actor, err := m.submitUserIntentRepo.FindActor(ctx, tx, lockedSession.ID, string(userUUID))
	if err != nil {
		return session.SubmitUserIntentResult{}, err
	}
	if actor == nil {
		return m.declineSubmitUserIntent(ctx, tx, requestID, lockedSession)
	}

	definition, err := m.pinnedGameReader.GetGameDefinition(ctx, lockedSession.GameDefinitionUUID)
	if err != nil {
		return session.SubmitUserIntentResult{}, err
	}
	if definition == nil {
		monitoring.Alert(ctx, "session pinned game definition is missing")
		return session.SubmitUserIntentResult{}, session.ErrPinnedDefinitionMissing
	}

	// Decoded and validated before ever reaching the engine: the engine
	// does not itself validate a SignalKindIntent signal's Fields against
	// the intent's declared Parameters (unlike SignalKindInteractionAnswered,
	// which it does validate), so passing an unvalidated, client-supplied
	// shape through risks a downstream binding panic rather than a clean
	// rejection.
	decodedArguments, err := decodeUserIntentArguments(arguments)
	if err != nil {
		return session.SubmitUserIntentResult{}, fmt.Errorf("decoding user intent arguments: %s", err)
	}
	validatedArguments, ok := validateUserIntentArguments(*definition, intentName, decodedArguments)
	if !ok {
		return m.declineSubmitUserIntent(ctx, tx, requestID, lockedSession)
	}

	if lockedSession.CurrentTurnID == nil {
		monitoring.Alert(ctx, "session has no current_turn_id")
		return session.SubmitUserIntentResult{}, fmt.Errorf("session %d has no current_turn_id", lockedSession.ID)
	}
	currentTurn, err := m.submitUserIntentRepo.GetRuntimeTurn(ctx, tx, *lockedSession.CurrentTurnID)
	if err != nil {
		return session.SubmitUserIntentResult{}, err
	}
	if currentTurn == nil {
		monitoring.Alert(ctx, "session current_turn_id does not resolve to a runtime turn")
		return session.SubmitUserIntentResult{}, fmt.Errorf("session %d current_turn_id %d does not resolve to a runtime turn", lockedSession.ID, *lockedSession.CurrentTurnID)
	}

	now := time.Now().UTC()

	compiledProgram, diagnostics := engineservice.Compile(*definition)
	if diagnostics.HasErrors() {
		// The pinned Definition already compiled successfully at Create/at
		// every prior RuntimeTurn - an unexpected recompile failure now is a
		// data-integrity problem, not a deterministic authored-game failure.
		monitoring.Alert(ctx, fmt.Sprintf(
			"pinned game definition failed to recompile submitting user intent: session_uuid=%s game_definition_uuid=%s",
			lockedSession.UUID, lockedSession.GameDefinitionUUID,
		))
		return m.terminalizeSubmitUserIntentFatal(ctx, tx, requestID, lockedSession, now, session.TerminalReasonRuntimeStateInvalid,
			RuntimeFailureKindStateInvalid, RuntimeFailureErrorCodeDefinitionRecompileFailed, formatDiagnostics(diagnostics), &currentTurn.ID, currentTurn.Sequence+1, actor.ID)
	}

	// Current authoritative Runtime state is never loaded from a persisted
	// Snapshot (none exists); engineservice.AdvanceTurn internally replays
	// every durable cause committed so far, given only the ordered signal
	// log below.
	input, priorSignals, err := replay.LoadPriorSignals(ctx, tx, m.submitUserIntentRepo, lockedSession.ID)
	if err != nil {
		return session.SubmitUserIntentResult{}, fmt.Errorf("reconstructing current runtime state: %s", err)
	}

	actorID := actor.ID
	fields := make(map[string]engine.Value, len(validatedArguments.Fields))
	for _, f := range validatedArguments.Fields {
		fields[f.Name] = f.Value
	}
	signal := engine.Signal{
		Kind:   engine.SignalKindIntent,
		Intent: intentName,
		Actor:  engine.UserID(strconv.FormatUint(uint64(actorID), 10)),
		Fields: fields,
	}

	outputs, err := engineservice.AdvanceTurn(compiledProgram, input, priorSignals, signal, engine.DefaultLimits())
	if err != nil {
		if errors.Is(err, engineservice.ErrReplayDivergence) {
			// Every element of priorSignals already succeeded once - it is
			// durable specifically because it did - so failing to replay it
			// identically is a data-integrity condition, never an ordinary
			// decline.
			monitoring.Alert(ctx, fmt.Sprintf("session %d: %s", lockedSession.ID, err))
			return session.SubmitUserIntentResult{}, fmt.Errorf("reconstructing current runtime state: %s", err)
		}
		// A rejection of the intent itself is an ordinary declined outcome:
		// no RuntimeTurn, no Snapshot mutation, Session stays RUNNING.
		if errors.Is(err, engineservice.ErrSignalRejected) || errors.Is(err, engineservice.ErrInputRejected) {
			return m.declineSubmitUserIntent(ctx, tx, requestID, lockedSession)
		}
		errorCode, errorMessage := classifyExecutionError(err)
		return m.terminalizeSubmitUserIntentFatal(ctx, tx, requestID, lockedSession, now, session.TerminalReasonRuntimeExecutionFailed,
			RuntimeFailureKindExecution, errorCode, errorMessage, &currentTurn.ID, currentTurn.Sequence+1, actor.ID)
	}

	payload, err := replay.EncodeUserIntentPayload(intentName, validatedArguments)
	if err != nil {
		return session.SubmitUserIntentResult{}, fmt.Errorf("encoding user intent payload: %s", err)
	}

	turnID, err := m.submitUserIntentRepo.CreateRuntimeTurn(ctx, tx, lockedSession.ID, currentTurn.Sequence+1, replay.UserIntentSourceKind, nil, nil, nil, &actorID)
	if err != nil {
		return session.SubmitUserIntentResult{}, err
	}
	// session_cause_events.runtime_turn_id is NOT NULL, so the cause event
	// row can only be created after the Turn it belongs to already exists -
	// see CreateRuntimeTurn's own doc comment. The Turn's own pointer back
	// to it is then backfilled.
	causeEventID, err := m.submitUserIntentRepo.CreateCauseEvent(ctx, tx, lockedSession.ID, turnID, "USER_INTENT", &actorID, payload)
	if err != nil {
		return session.SubmitUserIntentResult{}, err
	}
	if err := m.submitUserIntentRepo.SetRuntimeTurnCauseEvent(ctx, tx, turnID, causeEventID); err != nil {
		return session.SubmitUserIntentResult{}, err
	}

	if err := m.interactionsCapturer.Capture(ctx, tx, lockedSession.ID, turnID, outputs); err != nil {
		return session.SubmitUserIntentResult{}, err
	}
	if err := m.timersCapturer.Capture(ctx, tx, lockedSession.ID, turnID, outputs); err != nil {
		return session.SubmitUserIntentResult{}, err
	}
	if err := m.submitUserIntentRepo.SetCurrentTurn(ctx, tx, lockedSession.ID, turnID); err != nil {
		return session.SubmitUserIntentResult{}, err
	}
	if err := m.submitUserIntentRepo.RenewActivityDeadline(ctx, tx, lockedSession.ID, now.Add(m.activityTTL)); err != nil {
		return session.SubmitUserIntentResult{}, err
	}

	terminalReason, terminated := completion.Detect(outputs)
	if terminated {
		if err := m.submitUserIntentRepo.SetSessionTerminal(ctx, tx, lockedSession.ID, now, terminalReason); err != nil {
			return session.SubmitUserIntentResult{}, err
		}
		if err := m.submitUserIntentRepo.CloseAllActiveInteractionsForSession(ctx, tx, lockedSession.ID, session.InteractionClosureReasonSessionTerminated); err != nil {
			return session.SubmitUserIntentResult{}, err
		}
		if err := m.submitUserIntentRepo.CancelAllActiveTimerObligationsForSession(ctx, tx, lockedSession.ID, session.TimerObligationClosureReasonSessionTerminated); err != nil {
			return session.SubmitUserIntentResult{}, err
		}
	}

	mappedOutputs, err := m.mapOutputs(ctx, tx, lockedSession.ID, clientoutputs.ClientFacing(outputs))
	if err != nil {
		return session.SubmitUserIntentResult{}, fmt.Errorf("mapping client-facing outputs: %s", err)
	}

	result := session.SubmitUserIntentResult{Outcome: session.SubmitUserIntentOutcomeAccepted, SessionUUID: session.SessionUUID(lockedSession.UUID), Outputs: mappedOutputs, TerminalReason: terminalReason}
	responseBytes, err := json.Marshal(result)
	if err != nil {
		return session.SubmitUserIntentResult{}, fmt.Errorf("marshaling submit user intent response payload: %s", err)
	}
	if err := m.submitUserIntentRepo.CompleteSessionRequest(ctx, tx, requestID, &lockedSession.ID, submitUserIntentOutcomeAccepted, string(responseBytes)); err != nil {
		return session.SubmitUserIntentResult{}, fmt.Errorf("completing submit user intent request: %s", err)
	}
	return result, nil
}

// declineSubmitUserIntent completes requestID as an ordinary decline (no
// engine effect, no RuntimeTurn) and reports SubmitUserIntentOutcomeRejected.
func (m *Manager) declineSubmitUserIntent(ctx context.Context, tx *gorm.DB, requestID uint, lockedSession *internalrepo.Session) (session.SubmitUserIntentResult, error) {
	if err := m.submitUserIntentRepo.CompleteSessionRequest(ctx, tx, requestID, &lockedSession.ID, submitUserIntentOutcomeRejected, ""); err != nil {
		return session.SubmitUserIntentResult{}, fmt.Errorf("completing submit user intent request: %s", err)
	}
	return session.SubmitUserIntentResult{Outcome: session.SubmitUserIntentOutcomeRejected, SessionUUID: session.SessionUUID(lockedSession.UUID)}, nil
}

// terminalizeSubmitUserIntentFatal performs SubmitUserIntent's fatal path:
// atomically terminalizes the Session, persists the session_runtime_failures
// diagnostic record, and closes every currently-ACTIVE session_interactions
// row and timer obligation for it, mirroring terminalizeAnswerInteractionFatal
// exactly, and completes the idempotency claim so a retry replays this same
// outcome instead of re-attempting a doomed execution.
func (m *Manager) terminalizeSubmitUserIntentFatal(ctx context.Context, tx *gorm.DB, requestID uint, lockedSession *internalrepo.Session, terminalAt time.Time, terminalReason string, failureKind string, errorCode string, errorMessage string, baseTurnID *uint, attemptedSequence uint64, actorID uint) (session.SubmitUserIntentResult, error) {
	if err := m.materializeRuntimeFailure(ctx, tx, m.submitUserIntentRepo, lockedSession, terminalAt, terminalReason,
		failureKind, errorCode, errorMessage, baseTurnID, attemptedSequence, replay.UserIntentSourceKind, nil, nil, &actorID); err != nil {
		return session.SubmitUserIntentResult{}, err
	}
	if err := m.submitUserIntentRepo.CompleteSessionRequest(ctx, tx, requestID, &lockedSession.ID, submitUserIntentOutcomeRuntimeExecutionFailed, ""); err != nil {
		return session.SubmitUserIntentResult{}, fmt.Errorf("completing submit user intent request: %s", err)
	}
	return session.SubmitUserIntentResult{Outcome: session.SubmitUserIntentOutcomeRuntimeExecutionFailed, SessionUUID: session.SessionUUID(lockedSession.UUID)}, nil
}

// interpretExistingSubmitUserIntentClaim replays an already-completed
// idempotency claim's original outcome, mirroring
// interpretExistingStartClaim exactly.
func interpretExistingSubmitUserIntentClaim(existing *internalrepo.Request, incoming submitUserIntentRequestPayload) (session.SubmitUserIntentResult, error) {
	if existing.Status != internalrepo.RequestStatusCompleted {
		return session.SubmitUserIntentResult{}, session.ErrIdempotencyInFlight
	}

	var stored submitUserIntentRequestPayload
	if err := json.Unmarshal([]byte(existing.RequestPayload), &stored); err != nil {
		return session.SubmitUserIntentResult{}, fmt.Errorf("decoding stored submit user intent request payload: %s", err)
	}
	if stored != incoming {
		return session.SubmitUserIntentResult{}, session.ErrIdempotencyConflict
	}

	switch existing.Outcome {
	case submitUserIntentOutcomeRejected:
		return session.SubmitUserIntentResult{Outcome: session.SubmitUserIntentOutcomeRejected}, nil
	case submitUserIntentOutcomeRuntimeExecutionFailed:
		return session.SubmitUserIntentResult{Outcome: session.SubmitUserIntentOutcomeRuntimeExecutionFailed}, nil
	}

	if existing.ResponsePayload == nil {
		return session.SubmitUserIntentResult{}, fmt.Errorf("completed submit user intent idempotency record missing response payload")
	}
	var result session.SubmitUserIntentResult
	if err := json.Unmarshal([]byte(*existing.ResponsePayload), &result); err != nil {
		return session.SubmitUserIntentResult{}, fmt.Errorf("decoding stored submit user intent response payload: %s", err)
	}
	return result, nil
}

// decodeUserIntentArguments decodes arguments as an engine.Value, treating
// an empty/omitted payload as a nameless empty RecordValue - the shape a
// zero-parameter intent's arguments take.
func decodeUserIntentArguments(arguments json.RawMessage) (engine.Value, error) {
	if len(arguments) == 0 {
		return engine.RecordValue{}, nil
	}
	return engineservice.DecodeValue(arguments)
}

// validateUserIntentArguments validates that intentName names a declared
// program.UserIntentDeclaration and that decodedArguments is a RecordValue
// carrying every one of its declared Parameters, defensively at this
// system boundary, since the engine itself does not (see
// decodeUserIntentArguments's caller for why that matters). Known
// limitation: a Parameter whose declared type is a named type
// (record/union/enum/newtype, or a list/map/optional wrapping one) is
// checked for field presence only, not deep type conformance, since
// resolving a program.TypeReference to its compiled engine.Type requires
// compiler-internal named-type resolution this package does not have
// access to.
func validateUserIntentArguments(definition program.Definition, intentName string, decodedArguments engine.Value) (engine.RecordValue, bool) {
	var declaration program.UserIntentDeclaration
	found := false
	for _, d := range definition.UserIntents {
		if d.Name == intentName {
			declaration = d
			found = true
			break
		}
	}
	if !found {
		return engine.RecordValue{}, false
	}

	record, ok := decodedArguments.(engine.RecordValue)
	if !ok {
		return engine.RecordValue{}, false
	}

	for _, p := range declaration.Parameters {
		field, ok := record.FieldByName(p.Name)
		if !ok {
			return engine.RecordValue{}, false
		}
		if builtinType, ok := builtinEngineType(p.Type); ok {
			if !field.Value.Validate(builtinType) {
				return engine.RecordValue{}, false
			}
		}
	}
	return record, true
}

// builtinEngineType maps a program.BuiltinTypeReference to its compiled
// engine.Type, for the subset of parameter types this package can validate
// without compiler-internal named-type resolution. Returns false for any
// other program.TypeReference variant (NamedTypeReference, and any
// List/Map/OptionalTypeReference, even one that ultimately wraps only
// builtin types) - see this function's caller for what that means.
func builtinEngineType(ref program.TypeReference) (engine.Type, bool) {
	builtin, ok := ref.(program.BuiltinTypeReference)
	if !ok {
		return nil, false
	}
	switch builtin.Type {
	case program.BuiltinTypeBool:
		return engine.BoolType{}, true
	case program.BuiltinTypeNumber:
		return engine.NumberType{}, true
	case program.BuiltinTypeString:
		return engine.StringType{}, true
	case program.BuiltinTypeUser:
		return engine.UserType{}, true
	default:
		return nil, false
	}
}
