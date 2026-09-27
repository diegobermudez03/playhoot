package sessionlifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/diegobermudez03/playhoot/game/session/internal/sessionlock"
	"gorm.io/gorm"
)

// Runtime failure classes (GAME-ADR-0017's failure_kind), recorded on every
// session_runtime_failures row.
const (
	RuntimeFailureKindExecution    = "RUNTIME_EXECUTION"
	RuntimeFailureKindStateInvalid = "RUNTIME_STATE_INVALID"
)

// Stable, Session-Runtime-owned error_code catalog. Deliberately decoupled
// from engineservice.ExecutionErrorCode's own int values (mirrors WORK-0029's
// Value/Output decoupling precedent): a future engine error-code
// renumbering must never silently change what an already-persisted
// session_runtime_failures.error_code value means.
const (
	RuntimeFailureErrorCodeUnknown                  = "unknown"
	RuntimeFailureErrorCodeUndefinedReference       = "undefined_reference"
	RuntimeFailureErrorCodeDivisionByZero           = "division_by_zero"
	RuntimeFailureErrorCodeIndexOutOfRange          = "index_out_of_range"
	RuntimeFailureErrorCodeKeyNotFound              = "key_not_found"
	RuntimeFailureErrorCodeNoMatchingCase           = "no_matching_case"
	RuntimeFailureErrorCodeInvalidInitialState      = "invalid_initial_state"
	RuntimeFailureErrorCodeInvariantViolation       = "invariant_violation"
	RuntimeFailureErrorCodeSnapshotProgramMismatch  = "snapshot_program_mismatch"
	RuntimeFailureErrorCodeBudgetExceeded           = "budget_exceeded"
	RuntimeFailureErrorCodeLoopLimitExceeded        = "loop_limit_exceeded"
	RuntimeFailureErrorCodeInvalidRandomRange       = "invalid_random_range"
	RuntimeFailureErrorCodeEmptyRandomCollection    = "empty_random_collection"
	RuntimeFailureErrorCodeSlotOccupied             = "slot_occupied"
	RuntimeFailureErrorCodeInvalidTimerDelay        = "invalid_timer_delay"
	RuntimeFailureErrorCodeDuplicateRecipient       = "duplicate_recipient"
	RuntimeFailureErrorCodeInvalidQuorum            = "invalid_quorum"
	RuntimeFailureErrorCodeAskGroupNotJoined        = "ask_group_not_joined"
	RuntimeFailureErrorCodePresentationSlotOccupied = "presentation_slot_occupied"
	RuntimeFailureErrorCodeActiveSlotLimitExceeded  = "active_slot_limit_exceeded"
	// RuntimeFailureErrorCodeStepLimitExceeded is GAME-ADR-0019's own named
	// example ("a stable queryable error code equivalent to
	// runtime_turn_step_limit_exceeded").
	RuntimeFailureErrorCodeStepLimitExceeded = "runtime_turn_step_limit_exceeded"

	// RuntimeFailureErrorCodeDefinitionRecompileFailed marks the
	// RUNTIME_STATE_INVALID case: the pinned Definition unexpectedly fails to
	// recompile. This never comes from an engineservice.ExecutionError - the
	// engine is never even reached for this failure.
	RuntimeFailureErrorCodeDefinitionRecompileFailed = "definition_recompile_failed"

	// RuntimeFailureErrorCodeUnrecognized is a defensive fallback for a
	// future engineservice.ExecutionErrorCode this mapping has not been
	// updated for - distinct from RuntimeFailureErrorCodeUnknown, which is
	// the engine's own declared zero-value code, not an unmapped one.
	RuntimeFailureErrorCodeUnrecognized = "unrecognized_execution_error"
)

// mapExecutionErrorCode maps the Game Language engine's own
// engineservice.ExecutionErrorCode into Session Runtime's stable,
// engine-decoupled error_code catalog above.
// ExecutionErrorSignalRejected/ExecutionErrorInputRejected are deliberately
// not mapped: both are class-A expected rejections, already intercepted
// upstream (via errors.Is(err, engineservice.ErrSignalRejected/
// ErrInputRejected)) before any fatal path is reached, so this function never
// needs to classify them.
func mapExecutionErrorCode(code engineservice.ExecutionErrorCode) string {
	switch code {
	case engineservice.ExecutionErrorUnknown:
		return RuntimeFailureErrorCodeUnknown
	case engineservice.ExecutionErrorUndefinedReference:
		return RuntimeFailureErrorCodeUndefinedReference
	case engineservice.ExecutionErrorDivisionByZero:
		return RuntimeFailureErrorCodeDivisionByZero
	case engineservice.ExecutionErrorIndexOutOfRange:
		return RuntimeFailureErrorCodeIndexOutOfRange
	case engineservice.ExecutionErrorKeyNotFound:
		return RuntimeFailureErrorCodeKeyNotFound
	case engineservice.ExecutionErrorNoMatchingCase:
		return RuntimeFailureErrorCodeNoMatchingCase
	case engineservice.ExecutionErrorInvalidInitialState:
		return RuntimeFailureErrorCodeInvalidInitialState
	case engineservice.ExecutionErrorInvariantViolation:
		return RuntimeFailureErrorCodeInvariantViolation
	case engineservice.ExecutionErrorSnapshotProgramMismatch:
		return RuntimeFailureErrorCodeSnapshotProgramMismatch
	case engineservice.ExecutionErrorBudgetExceeded:
		return RuntimeFailureErrorCodeBudgetExceeded
	case engineservice.ExecutionErrorLoopLimitExceeded:
		return RuntimeFailureErrorCodeLoopLimitExceeded
	case engineservice.ExecutionErrorInvalidRandomRange:
		return RuntimeFailureErrorCodeInvalidRandomRange
	case engineservice.ExecutionErrorEmptyRandomCollection:
		return RuntimeFailureErrorCodeEmptyRandomCollection
	case engineservice.ExecutionErrorSlotOccupied:
		return RuntimeFailureErrorCodeSlotOccupied
	case engineservice.ExecutionErrorInvalidTimerDelay:
		return RuntimeFailureErrorCodeInvalidTimerDelay
	case engineservice.ExecutionErrorDuplicateRecipient:
		return RuntimeFailureErrorCodeDuplicateRecipient
	case engineservice.ExecutionErrorInvalidQuorum:
		return RuntimeFailureErrorCodeInvalidQuorum
	case engineservice.ExecutionErrorAskGroupNotJoined:
		return RuntimeFailureErrorCodeAskGroupNotJoined
	case engineservice.ExecutionErrorPresentationSlotOccupied:
		return RuntimeFailureErrorCodePresentationSlotOccupied
	case engineservice.ExecutionErrorActiveSlotLimitExceeded:
		return RuntimeFailureErrorCodeActiveSlotLimitExceeded
	case engineservice.ExecutionErrorStepChainExceeded:
		return RuntimeFailureErrorCodeStepLimitExceeded
	default:
		return RuntimeFailureErrorCodeUnrecognized
	}
}

// classifyExecutionError extracts a stable, engine-decoupled error_code plus
// message from a non-rejection, non-replay-divergence AdvanceTurn/StartTurn
// failure, for materializeRuntimeFailure's RUNTIME_EXECUTION_FAILED branch.
// Every *engineservice.ExecutionError this package can receive here is
// mapped by mapExecutionErrorCode; any other error shape (defensive -
// AdvanceTurn/StartTurn's only other documented failure shapes are already
// handled by each call site's own ErrReplayDivergence/ErrSignalRejected/
// ErrInputRejected checks before this is ever called) reports
// RuntimeFailureErrorCodeUnrecognized with the error's own message.
func classifyExecutionError(err error) (errorCode string, errorMessage string) {
	var execErr *engineservice.ExecutionError
	if errors.As(err, &execErr) {
		return mapExecutionErrorCode(execErr.Code), execErr.Message
	}
	return RuntimeFailureErrorCodeUnrecognized, err.Error()
}

// formatDiagnostics joins a failed recompile's engineservice.Diagnostics
// into one operator/debug-only error_message string, for the
// RUNTIME_STATE_INVALID branch (which never produces an
// engineservice.ExecutionError - the engine is never even reached).
func formatDiagnostics(diagnostics engineservice.Diagnostics) string {
	parts := make([]string, len(diagnostics))
	for i, d := range diagnostics {
		parts[i] = d.String()
	}
	return strings.Join(parts, "; ")
}

// runtimeFailureDiagnosticPayload is session_runtime_failures.
// diagnostic_payload's content - deliberately a minimal, forward-compatible
// envelope. error_code/error_message/source_kind/etc. already have their own
// columns; the engine's current AdvanceTurn/StartTurn contract provides no
// additional in-memory diagnostic content (no per-Step index, no partial
// Step traces, no Snapshot - GAME-ADR-0024 forbids reintroducing one under
// any name) for this payload to carry beyond a schema version, so it is not
// invented here.
type runtimeFailureDiagnosticPayload struct {
	SchemaVersion int `json:"schema_version"`
}

// runtimeFailureRepoAPI is materializeRuntimeFailure's own narrow
// persistence contract - satisfied structurally by every existing step's own
// xxxRepoAPI interface once CreateRuntimeFailure is added to it.
type runtimeFailureRepoAPI interface {
	SetSessionTerminal(ctx context.Context, tx *gorm.DB, sessionID uint, terminalAt time.Time, terminalReason string) error
	CreateRuntimeFailure(ctx context.Context, tx *gorm.DB, sessionID uint, failureKind string, errorCode string, errorMessage string, baseTurnID *uint, attemptedSequence uint64, sourceKind string, sourceInteractionID *uint, sourceTimerObligationID *uint, actorID *uint, diagnosticPayload []byte) error
	CloseAllActiveInteractionsForSession(ctx context.Context, tx *gorm.DB, sessionID uint, reason string) error
	CancelAllActiveTimerObligationsForSession(ctx context.Context, tx *gorm.DB, sessionID uint, reason string) error
}

// materializeRuntimeFailure implements GAME-ADR-0017's atomic fatal
// materialization together with GAME-ADR-0019's terminal-cleanup invariant,
// in one place, for every RuntimeTurn-producing path's fatal branch: sets
// the Session TERMINAL, persists the session_runtime_failures diagnostic
// record, closes every ACTIVE interaction, and cancels every ACTIVE timer
// obligation - all within the caller's already-open transaction. Callers
// remain responsible for their own Result value and any idempotency-claim
// completion (both vary per step and stay local to each).
//
// baseTurnID/attemptedSequence identify the last committed Turn this fatal
// attempt was made against: nil/1 for a pre-first-Turn Start failure, or the
// caller's own already-loaded current Turn's id/Sequence+1 otherwise.
// sourceKind/sourceInteractionID/sourceTimerObligationID/actorID identify the
// driving cause, mirroring the same values each step already passes to its
// own (non-fatal) CreateRuntimeTurn call.
func (m *Manager) materializeRuntimeFailure(
	ctx context.Context,
	tx *gorm.DB,
	repo runtimeFailureRepoAPI,
	lockedSession *sessionlock.Session,
	terminalAt time.Time,
	terminalReason string,
	failureKind string,
	errorCode string,
	errorMessage string,
	baseTurnID *uint,
	attemptedSequence uint64,
	sourceKind string,
	sourceInteractionID *uint,
	sourceTimerObligationID *uint,
	actorID *uint,
) error {
	if err := repo.SetSessionTerminal(ctx, tx, lockedSession.ID, terminalAt, terminalReason); err != nil {
		return err
	}
	diagnosticPayload, err := json.Marshal(runtimeFailureDiagnosticPayload{SchemaVersion: 1})
	if err != nil {
		return fmt.Errorf("encoding runtime failure diagnostic payload: %s", err)
	}
	if err := repo.CreateRuntimeFailure(ctx, tx, lockedSession.ID, failureKind, errorCode, errorMessage, baseTurnID, attemptedSequence, sourceKind, sourceInteractionID, sourceTimerObligationID, actorID, diagnosticPayload); err != nil {
		return err
	}
	if err := repo.CloseAllActiveInteractionsForSession(ctx, tx, lockedSession.ID, session.InteractionClosureReasonSessionTerminated); err != nil {
		return err
	}
	if err := repo.CancelAllActiveTimerObligationsForSession(ctx, tx, lockedSession.ID, session.TimerObligationClosureReasonSessionTerminated); err != nil {
		return err
	}
	return nil
}
