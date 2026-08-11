package engineservice

import (
	"github.com/diegobermudez03/playhoot/game/v1/engine"
	"github.com/diegobermudez03/playhoot/game/v1/engine/internal/runtime"
)

// ExecutionErrorCode identifies the category of an ExecutionError. See
// the underlying engine/internal/runtime.ExecutionErrorCode for the
// full, documented set of stable error categories.
type ExecutionErrorCode = runtime.ExecutionErrorCode

const (
	ExecutionErrorUnknown                  = runtime.ExecutionErrorUnknown
	ExecutionErrorNotImplemented           = runtime.ExecutionErrorNotImplemented
	ExecutionErrorUndefinedReference       = runtime.ExecutionErrorUndefinedReference
	ExecutionErrorDivisionByZero           = runtime.ExecutionErrorDivisionByZero
	ExecutionErrorIndexOutOfRange          = runtime.ExecutionErrorIndexOutOfRange
	ExecutionErrorKeyNotFound              = runtime.ExecutionErrorKeyNotFound
	ExecutionErrorNoMatchingCase           = runtime.ExecutionErrorNoMatchingCase
	ExecutionErrorInvalidInitialState      = runtime.ExecutionErrorInvalidInitialState
	ExecutionErrorInvariantViolation       = runtime.ExecutionErrorInvariantViolation
	ExecutionErrorSnapshotProgramMismatch  = runtime.ExecutionErrorSnapshotProgramMismatch
	ExecutionErrorSignalRejected           = runtime.ExecutionErrorSignalRejected
	ExecutionErrorBudgetExceeded           = runtime.ExecutionErrorBudgetExceeded
	ExecutionErrorLoopLimitExceeded        = runtime.ExecutionErrorLoopLimitExceeded
	ExecutionErrorInvalidRandomRange       = runtime.ExecutionErrorInvalidRandomRange
	ExecutionErrorEmptyRandomCollection    = runtime.ExecutionErrorEmptyRandomCollection
	ExecutionErrorSlotOccupied             = runtime.ExecutionErrorSlotOccupied
	ExecutionErrorInvalidTimerDelay        = runtime.ExecutionErrorInvalidTimerDelay
	ExecutionErrorInputRejected            = runtime.ExecutionErrorInputRejected
	ExecutionErrorChildOutcomeNotJoined    = runtime.ExecutionErrorChildOutcomeNotJoined
	ExecutionErrorDuplicateRecipient       = runtime.ExecutionErrorDuplicateRecipient
	ExecutionErrorInvalidQuorum            = runtime.ExecutionErrorInvalidQuorum
	ExecutionErrorAskGroupNotJoined        = runtime.ExecutionErrorAskGroupNotJoined
	ExecutionErrorDuplicateTaskKey         = runtime.ExecutionErrorDuplicateTaskKey
	ExecutionErrorTaskGroupNotJoined       = runtime.ExecutionErrorTaskGroupNotJoined
	ExecutionErrorTaskGroupLeftBuilding    = runtime.ExecutionErrorTaskGroupLeftBuilding
	ExecutionErrorPresentationSlotOccupied = runtime.ExecutionErrorPresentationSlotOccupied
	ExecutionErrorWorkflowDepthExceeded    = runtime.ExecutionErrorWorkflowDepthExceeded
	ExecutionErrorActiveSlotLimitExceeded  = runtime.ExecutionErrorActiveSlotLimitExceeded
)

// ExecutionError is the error type returned by NewSnapshot and Step.
// See the underlying engine/internal/runtime.ExecutionError for its
// documented atomicity guarantee.
type ExecutionError = runtime.ExecutionError

// ErrExecutionNotImplemented is returned by any execution operation
// whose real semantics have not been implemented yet.
var ErrExecutionNotImplemented = runtime.ErrExecutionNotImplemented

// ErrSignalRejected and ErrInputRejected are the two "stale signal"
// outcomes Step ever returns, for structurally different reasons — see
// the underlying engine/internal/runtime package's doc comment for the
// full distinction.
var (
	ErrSignalRejected = runtime.ErrSignalRejected
	ErrInputRejected  = runtime.ErrInputRejected
)

// NewSnapshot creates the initial engine.Snapshot for one new game
// instance of p, together with the first Signal a caller should pass to
// Step — see LOGICAL_CONTRACT.md's "Program + initialization input ->
// initialize -> initial Snapshot".
func NewSnapshot(p engine.Program, input engine.InitializationInput) (engine.Snapshot, engine.Signal, error) {
	return runtime.NewSnapshot(p, input)
}

// Step applies exactly one Signal to snapshot and returns the atomic
// result as an engine.Commit — see LOGICAL_CONTRACT.md's "Program +
// Snapshot + Signal -> step -> Commit". Step never mutates snapshot in
// place: on success it returns a new Snapshot inside the Commit; on
// failure snapshot remains valid and unchanged, no Commit is produced,
// and no Output is considered published.
func Step(p engine.Program, snapshot engine.Snapshot, signal engine.Signal, limits engine.Limits) (engine.Commit, error) {
	return runtime.Step(p, snapshot, signal, limits)
}

// Evaluate evaluates expr against scope within program p, following
// the same pure-expression semantics Step uses internally to evaluate
// guards, operations, and workflow control. Exposed at the root for
// callers that need to evaluate an arbitrary compiled Expression
// outside of a Step call (for example, tooling or diagnostics) —
// ordinary game execution never needs to call this directly.
func Evaluate(p engine.Program, expr engine.Expression, scope engine.Scope) (engine.Value, error) {
	return runtime.Evaluate(p, expr, scope)
}
