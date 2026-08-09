package engineservice

import (
	"fmt"

	"github.com/diegobermudez03/playhoot/game/v1/engine"
)

// ExecutionErrorCode identifies the category of an ExecutionError.
//
// The set of codes is expected to grow as real execution semantics are
// implemented. Callers should generally match on the sentinel error
// values below via errors.Is rather than switching on Code directly, so
// that a future ExecutionError carrying additional context still
// compares correctly.
type ExecutionErrorCode int

const (
	// ExecutionErrorUnknown is the zero value. It marks an
	// ExecutionError that has not been assigned a more specific code.
	ExecutionErrorUnknown ExecutionErrorCode = iota

	// ExecutionErrorNotImplemented marks a call into execution behavior
	// this package has not implemented yet. It exists only while the
	// engine's real language semantics are being built out and is
	// expected to disappear once execution is complete.
	ExecutionErrorNotImplemented

	// ExecutionErrorUndefinedReference marks a ReferenceExpression whose
	// name is missing from the engine.Scope Evaluate was given. The
	// compiler already guarantees the name was declared somewhere in
	// scope at compile time; this can still happen because a caller
	// assembles the runtime engine.Scope independently, outside the
	// compiler's control.
	ExecutionErrorUndefinedReference

	// ExecutionErrorDivisionByZero marks a BinaryOperatorDivide or
	// BinaryOperatorModulo evaluation whose right operand is zero.
	ExecutionErrorDivisionByZero

	// ExecutionErrorIndexOutOfRange marks an IndexExpression into a list
	// whose evaluated index is negative, non-integer, or beyond the
	// list's length.
	ExecutionErrorIndexOutOfRange

	// ExecutionErrorKeyNotFound marks an IndexExpression into a map
	// whose evaluated key has no matching entry.
	ExecutionErrorKeyNotFound

	// ExecutionErrorNoMatchingCase marks a MatchExpression whose value
	// matched none of its cases. The compiler does not require match
	// cases to be exhaustive; this is the defined, non-panicking result
	// of reaching a value no case covers.
	ExecutionErrorNoMatchingCase
)

// ExecutionError is the error type returned by NewSnapshot and Step.
//
// An ExecutionError never represents a partially applied change: per the
// engine's atomicity contract, whenever NewSnapshot or Step returns a
// non-nil error, the input engine.Snapshot (if any) is unchanged, no
// engine.Commit is produced, and no engine.Output is considered
// published.
//
// ExecutionError deliberately does not represent compile-time semantic
// problems; those are reported as Diagnostics by Compile, not as errors.
type ExecutionError struct {
	Code    ExecutionErrorCode
	Message string
}

func (e *ExecutionError) Error() string {
	return e.Message
}

// Is lets errors.Is(err, ErrExecutionNotImplemented) and similar checks
// match by Code rather than by pointer identity, so a future
// ExecutionError constructed with additional context still satisfies
// errors.Is against the matching sentinel below.
func (e *ExecutionError) Is(target error) bool {
	t, ok := target.(*ExecutionError)
	if !ok {
		return false
	}
	return e.Code == t.Code
}

// newExecutionError builds an *ExecutionError with the given code and a
// formatted message.
func newExecutionError(code ExecutionErrorCode, format string, args ...any) *ExecutionError {
	return &ExecutionError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// ErrExecutionNotImplemented is returned by any execution operation
// whose real semantics have not been implemented yet. It is a temporary
// marker, not a permanent part of the engine's contract.
var ErrExecutionNotImplemented = &ExecutionError{
	Code:    ExecutionErrorNotImplemented,
	Message: "engineservice: execution semantics are not implemented yet",
}

// NewSnapshot creates the initial engine.Snapshot for one new game
// instance of p.
//
// This version does not yet instantiate the root workflow, initialize
// global state from the definition p was compiled from, or perform any
// other real initialization semantics; it always succeeds and returns an
// empty engine.Snapshot.
func NewSnapshot(p engine.Program, input engine.InitializationInput) (engine.Snapshot, error) {
	return engine.Snapshot{}, nil
}

// Step applies exactly one Signal to snapshot and returns the atomic
// result as an engine.Commit.
//
// Step never mutates snapshot in place; on success it returns a new
// engine.Snapshot inside the engine.Commit, leaving snapshot itself
// valid and unchanged. If Step returns a non-nil error, snapshot is
// unchanged, no engine.Commit is produced, and no engine.Output is
// considered published — the step is atomic from the caller's
// perspective, per LOGICAL_CONTRACT.md.
//
// This version does not yet verify that snapshot belongs to p — that
// check returns once engine.Program and engine.Snapshot carry real
// compiled/instance identity — and does not implement the remaining
// execution contract described in engine/README.md's "Execution
// contract" section (transition selection, guard evaluation, operation
// execution, invariant validation, projection recalculation, or output
// creation). It always returns ErrExecutionNotImplemented.
func Step(p engine.Program, snapshot engine.Snapshot, signal engine.Signal) (engine.Commit, error) {
	return engine.Commit{}, ErrExecutionNotImplemented
}
