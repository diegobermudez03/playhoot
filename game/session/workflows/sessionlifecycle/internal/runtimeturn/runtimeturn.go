// Package runtimeturn is sessionlifecycle's shared RuntimeTurn execution
// mechanism: draining an engine.Program's initial and internal-signal chain
// to quiescence under a fixed Step-count bound. Every sessionlifecycle step
// that produces a RuntimeTurn calls Drain rather than reimplementing this
// loop.
package runtimeturn

import (
	"errors"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
)

// MaxSteps is the Session-level RuntimeTurn Step-chain bound: every actual
// engine.Step call belonging to the same RuntimeTurn counts - the initial
// signal plus every subsequent Step caused by draining a prior Step's
// InternalSignals. A code-level constant, not authored by individual games
// and not a durable per-Session setting.
const MaxSteps = 20

// ErrStepBoundExceeded is Result.Err when reaching quiescence would need
// more than MaxSteps Step calls.
var ErrStepBoundExceeded = errors.New("runtimeturn: exceeded max steps per runtime turn")

// StepTrace is one actual engine.Step call's outcome within a drained
// RuntimeTurn.
//
// Workflow/TransitionName/StateBefore/StateAfter/OperationCount are
// session_runtime_steps.commit_payload's persisted shape - technical
// execution trace only, never authoritative gameplay - so they marshal
// with encoding/json directly. Path and Outputs are not part
// of that persisted shape (both carry engine.Value-typed data requiring
// engineservice.EncodeValue, not plain encoding/json); they exist for a
// caller that needs this Step's declarative Outputs against the instance
// Path that produced them - session_interactions capture, in particular -
// so both are excluded from JSON with json:"-".
type StepTrace struct {
	Workflow       string `json:"workflow"`
	TransitionName string `json:"transition_name"`
	StateBefore    string `json:"state_before"`
	StateAfter     string `json:"state_after"`
	OperationCount int    `json:"operation_count"`

	Path    []engine.PathStep `json:"-"`
	Outputs []engine.Output   `json:"-"`
}

// Result is Drain's outcome.
type Result struct {
	// Snapshot is the Turn's final authoritative Snapshot on success, or
	// the original input Snapshot unchanged when Err is non-nil.
	Snapshot engine.Snapshot

	// Steps traces every actual Step call in order; only meaningful when
	// Err is nil.
	Steps []StepTrace

	// Err is nil on success. On failure it is either the error returned by
	// the failing engineservice.Step call (an *engineservice.ExecutionError,
	// engineservice.ErrSignalRejected, or engineservice.ErrInputRejected),
	// or ErrStepBoundExceeded when reaching quiescence would need more than
	// MaxSteps calls.
	Err error

	// FailedOnInitialSignal is only meaningful when Err is non-nil: true
	// means the very first Step call - initialSignal itself - is what
	// failed; false means a later, engine-internally-generated
	// internal-signal Step call failed instead (or the failure is
	// ErrStepBoundExceeded, never attributable to one single signal). A
	// caller that treats a rejected initial signal as an ordinary declined
	// outcome, rather than fatal, needs this distinction - a rejection of
	// an internal signal the engine itself generated is never an ordinary
	// decline.
	FailedOnInitialSignal bool
}

// Drain executes p's initial Signal chain to quiescence, draining each
// Step's engine.Commit.InternalSignals in FIFO order and counting every
// actual Step call (the initial one plus every internal-signal-caused one)
// toward MaxSteps.
//
// Drain is a pure function over engine/engineservice with no
// persistence/transaction dependency, so it is unit-testable directly
// without a real database connection.
func Drain(p engine.Program, snapshot engine.Snapshot, initialSignal engine.Signal) Result {
	pending := []engine.Signal{initialSignal}
	current := snapshot
	var steps []StepTrace
	for len(pending) > 0 {
		if len(steps) >= MaxSteps {
			return Result{Snapshot: snapshot, Err: ErrStepBoundExceeded}
		}
		signal := pending[0]
		pending = pending[1:]

		commit, err := engineservice.Step(p, current, signal, engine.DefaultLimits())
		if err != nil {
			return Result{Snapshot: snapshot, Err: err, FailedOnInitialSignal: len(steps) == 0}
		}
		current = commit.Snapshot
		steps = append(steps, StepTrace{
			Workflow:       commit.Trace.Workflow,
			TransitionName: commit.Trace.TransitionName,
			StateBefore:    commit.Trace.StateBefore,
			StateAfter:     commit.Trace.StateAfter,
			OperationCount: commit.Trace.OperationCount,
			Path:           commit.Trace.Path,
			Outputs:        commit.Outputs,
		})
		pending = append(pending, commit.InternalSignals...)
	}
	return Result{Snapshot: current, Steps: steps}
}
