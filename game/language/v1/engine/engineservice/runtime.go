package engineservice

import (
	"errors"
	"fmt"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/internal/runtime"
)

// ExecutionErrorCode identifies the category of an ExecutionError. See
// the underlying engine/internal/runtime.ExecutionErrorCode for the
// full, documented set of stable error categories.
type ExecutionErrorCode = runtime.ExecutionErrorCode

const (
	ExecutionErrorUnknown                  = runtime.ExecutionErrorUnknown
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
	ExecutionErrorDuplicateRecipient       = runtime.ExecutionErrorDuplicateRecipient
	ExecutionErrorInvalidQuorum            = runtime.ExecutionErrorInvalidQuorum
	ExecutionErrorAskGroupNotJoined        = runtime.ExecutionErrorAskGroupNotJoined
	ExecutionErrorPresentationSlotOccupied = runtime.ExecutionErrorPresentationSlotOccupied
	ExecutionErrorActiveSlotLimitExceeded  = runtime.ExecutionErrorActiveSlotLimitExceeded
	ExecutionErrorStepChainExceeded        = runtime.ExecutionErrorStepChainExceeded
)

// ExecutionError is the error type returned by StartTurn and AdvanceTurn.
// See the underlying engine/internal/runtime.ExecutionError for its
// documented atomicity guarantee.
type ExecutionError = runtime.ExecutionError

// ErrSignalRejected and ErrInputRejected are the two "stale signal"
// outcomes StartTurn/AdvanceTurn ever return for their own new signal,
// for structurally different reasons — see the underlying
// engine/internal/runtime package's doc comment for the full
// distinction.
var (
	ErrSignalRejected = runtime.ErrSignalRejected
	ErrInputRejected  = runtime.ErrInputRejected
)

// ErrReplayDivergence indicates that replaying an already-committed
// signal — Start's own initialization, or an element of AdvanceTurn's
// priorSignals — did not reproduce its original successful commit. This
// is always a data-integrity condition, never an ordinary decline:
// every element of priorSignals already succeeded once (that is why it
// is durable), so failing to reproduce it deterministically means the
// pinned Definition, the recorded inputs, or the engine itself no
// longer agree with what actually happened. Distinct from an ordinary
// failure of AdvanceTurn's own newSignal, which surfaces exactly as it
// always has (ErrSignalRejected, ErrInputRejected, or another
// *ExecutionError).
var ErrReplayDivergence = errors.New("engineservice: replaying an already-committed signal did not reproduce its original commit")

// StartTurn performs a Session's mandatory first turn: initializes new
// runtime state from start and applies the engine's own synthesized
// first lifecycle signal to quiescence, entirely internally — see
// LOGICAL_CONTRACT.md's "Program + initialization input -> initialize
// -> initial Snapshot" followed immediately by its first Step. The
// caller never constructs or sees the synthesized signal itself (a
// fixed, deterministic value with nothing for a caller to decide), or
// any engine.Snapshot. Returns that turn's Outputs.
func StartTurn(p engine.Program, start engine.InitializationInput, limits engine.Limits) ([]engine.Output, error) {
	snapshot, startSignal, err := runtime.NewSnapshot(p, start)
	if err != nil {
		return nil, err
	}
	_, outputs, err := runtime.DrainSignal(p, snapshot, startSignal, limits)
	if err != nil {
		return nil, err
	}
	return outputs, nil
}

// AdvanceTurn reconstructs current runtime state by internally
// replaying Start's own initialization followed by every signal in
// priorSignals, in order, against a freshly initialized instance
// (discarding their Outputs — already durably recorded when they first
// happened), then drains newSignal to quiescence exactly as one Step
// would. Returns only newSignal's own Outputs. The caller never
// constructs, holds, or reads an engine.Snapshot.
//
// priorSignals is exactly the ordered log of externally-driven signals
// a durable, replay-capable caller already needs to keep for its own
// crash-recovery purposes — nothing new needs to be captured to supply
// it.
//
// If replaying Start's initialization or an element of priorSignals
// fails, the returned error wraps ErrReplayDivergence. A failure of
// newSignal itself surfaces exactly as it always has.
func AdvanceTurn(p engine.Program, start engine.InitializationInput, priorSignals []engine.Signal, newSignal engine.Signal, limits engine.Limits) ([]engine.Output, error) {
	snapshot, startSignal, err := runtime.NewSnapshot(p, start)
	if err != nil {
		return nil, fmt.Errorf("%w: replaying start initialization: %s", ErrReplayDivergence, err)
	}
	current, _, err := runtime.DrainSignal(p, snapshot, startSignal, limits)
	if err != nil {
		return nil, fmt.Errorf("%w: replaying start initialization: %s", ErrReplayDivergence, err)
	}
	for i, signal := range priorSignals {
		current, _, err = runtime.DrainSignal(p, current, signal, limits)
		if err != nil {
			return nil, fmt.Errorf("%w: replaying prior signal %d: %s", ErrReplayDivergence, i, err)
		}
	}
	_, outputs, err := runtime.DrainSignal(p, current, newSignal, limits)
	if err != nil {
		return nil, err
	}
	return outputs, nil
}

// Evaluate evaluates expr against scope within program p, following
// the same pure-expression semantics AdvanceTurn/StartTurn use
// internally to evaluate guards, operations, and workflow control.
// Exposed at the root for callers that need to evaluate an arbitrary
// compiled Expression outside of Turn processing (for example, tooling
// or diagnostics) — ordinary game execution never needs to call this
// directly.
func Evaluate(p engine.Program, expr engine.Expression, scope engine.Scope) (engine.Value, error) {
	return runtime.Evaluate(p, expr, scope)
}
