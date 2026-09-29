package sessionlifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/diegobermudez03/playhoot/session"
	internalrepo "github.com/diegobermudez03/playhoot/session/workflows/sessionlifecycle/internal/repo"
	"gorm.io/gorm"
)

// Runtime failure classes recorded on every session_runtime_failures row. A
// JS backend script is never compiled ahead of time, so the only fatal
// RUNTIME-phase failure is an Executor infrastructure failure.
const (
	RuntimeFailureKindExecution = "RUNTIME_EXECUTION"
)

// RuntimeFailureErrorCodeExecutorError is the one stable, queryable
// error_code this package's Executor-driven fatal path ever records.
// *executor.ExecutorError carries a free-form Reason/Cause, not a closed
// error enum the way the retired engineservice.ExecutionErrorCode was, so
// there is no finer-grained catalog to map into - the full detail lives in
// error_message instead.
const RuntimeFailureErrorCodeExecutorError = "executor_error"

// classifyExecutionError extracts a stable error_code plus message from a
// *executor.ExecutorError, for materializeRuntimeFailure's
// RUNTIME_EXECUTION_FAILED branch. Every call site already intercepts
// *executor.ScriptRejectedError as an ordinary decline before this is ever
// called - only an infrastructure-level ExecutorError reaches here.
func classifyExecutionError(err error) (errorCode string, errorMessage string) {
	return RuntimeFailureErrorCodeExecutorError, err.Error()
}

// runtimeFailureDiagnosticPayload is session_runtime_failures.
// diagnostic_payload's content - deliberately a minimal, forward-compatible
// envelope. error_code/error_message/source_kind/etc. already have their own
// columns; the Executor's current Execute contract provides no additional
// in-memory diagnostic content for this payload to carry beyond a schema
// version, so it is not invented here.
type runtimeFailureDiagnosticPayload struct {
	SchemaVersion int `json:"schema_version"`
}

// runtimeFailureRepoAPI is materializeRuntimeFailure's own narrow
// persistence contract - satisfied structurally by every existing step's own
// xxxRepoAPI interface once CreateRuntimeFailure is added to it.
type runtimeFailureRepoAPI interface {
	SetSessionTerminal(ctx context.Context, tx *gorm.DB, sessionID uint, terminalAt time.Time, terminalReason string) error
	CreateRuntimeFailure(ctx context.Context, tx *gorm.DB, sessionID uint, failureKind string, errorCode string, errorMessage string, baseTurnID *uint, attemptedSequence uint64, sourceKind string, sourceTimerObligationID *uint, actorID *uint, diagnosticPayload []byte) error
	CancelAllActiveTimerObligationsForSession(ctx context.Context, tx *gorm.DB, sessionID uint, reason string) error
}

// materializeRuntimeFailure atomically sets the Session TERMINAL, persists
// the session_runtime_failures diagnostic record, and cancels every ACTIVE
// timer obligation, in one place for every RuntimeTurn-producing path's
// fatal branch, all within the caller's already-open transaction. Callers
// remain responsible for their own Result value and any idempotency-claim
// completion (both vary per step and stay local to each).
//
// baseTurnID/attemptedSequence identify the last committed Turn this fatal
// attempt was made against: nil/1 for a pre-first-Turn Start failure, or the
// caller's own already-loaded current Turn's id/Sequence+1 otherwise.
// sourceKind/sourceTimerObligationID/actorID identify the driving cause,
// mirroring the same values each step already passes to its own
// (non-fatal) CreateRuntimeTurn call.
func (m *Manager) materializeRuntimeFailure(
	ctx context.Context,
	tx *gorm.DB,
	repo runtimeFailureRepoAPI,
	lockedSession *internalrepo.Session,
	terminalAt time.Time,
	terminalReason string,
	failureKind string,
	errorCode string,
	errorMessage string,
	baseTurnID *uint,
	attemptedSequence uint64,
	sourceKind string,
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
	if err := repo.CreateRuntimeFailure(ctx, tx, lockedSession.ID, failureKind, errorCode, errorMessage, baseTurnID, attemptedSequence, sourceKind, sourceTimerObligationID, actorID, diagnosticPayload); err != nil {
		return err
	}
	if err := repo.CancelAllActiveTimerObligationsForSession(ctx, tx, lockedSession.ID, session.TimerObligationClosureReasonSessionTerminated); err != nil {
		return err
	}
	return nil
}
