package engine

// Trace explains one executed transition: which instance and
// transition were selected, its guard's result, how many operations
// ran, what state change or terminal outcome its control produced,
// and which declarative outputs it caused.
//
// A Trace is produced only as part of a Commit and describes exactly
// one Step call — a rejected or failed Step never produces one, since
// no Commit exists to carry it; the returned *ExecutionError's Message
// and Code are the explanation for those. It is intended to support
// debugging, explanation, and replay verification (see
// LOGICAL_CONTRACT.md's determinism guarantees), not to drive further
// execution — nothing in engineservice ever reads a Trace back in.
//
// Trace deliberately duplicates a little of what Commit already
// carries at the top level (Outputs, in particular) so that a Trace
// value is self-contained: something that only has a Trace — a log
// line, a stored debugging record — can still fully explain what
// happened without needing the rest of the Commit alongside it.
type Trace struct {
	// Path addresses which workflow instance this Step call targeted —
	// the same value as the consumed Signal's own Path.
	Path []PathStep

	// Workflow names the compiled Workflow the targeted instance runs.
	Workflow string

	// TransitionName is the Name of the Transition selected — see
	// Transition.Name. Empty when no ordinary transition was selected
	// (currently, only a SignalKindAskGroupAnswered Step, which never
	// selects one at all — see program.AskGroupCompletedSignalSource's
	// documented "never produces a signal per individual answer").
	TransitionName string

	// GuardEvaluated reports whether the selected transition declared a
	// Guard at all. GuardResult is only meaningful when this is true —
	// a transition never commits with a false Guard, so GuardResult is
	// always true whenever a Trace exists to report it.
	GuardEvaluated bool
	GuardResult    bool

	// StateBefore and StateAfter are the targeted instance's State
	// immediately before and after this Step call. They are equal
	// unless the selected transition's control was a GotoControl.
	StateBefore string
	StateAfter  string

	// Outcome is nil unless this Step call's control produced a
	// terminal outcome for the targeted instance — see WorkflowOutcome.
	Outcome *WorkflowOutcome

	// OperationCount is how many synchronous operations this Step call
	// executed, toward Limits.MaxOperations.
	OperationCount int

	// Outputs is every declarative Output this Step call produced —
	// the same slice as the owning Commit's own Outputs field.
	Outputs []Output
}
