package program

// ChildFailedSignalSource matches the signal produced when the child
// workflow in the named child slot owned by the current workflow (see
// ChildWorkflowSlotDeclaration) terminates through an authored FailControl.
//
// The signal schema exposes exactly one field, "error", typed as the
// built-in string. It never exposes child or parent workflow instance IDs,
// the slot name as a runtime value, child-local state, stack traces,
// engine errors, or other internal execution metadata — an authored child
// failure is a business-level outcome, not an engine execution error (see
// FailControl for the distinction).
//
// Handling this signal is how a parent joins a failed child slot: when the
// child fails, the slot becomes failed-awaiting-join and its error is held
// durably until the parent's transition for this signal commits
// successfully, at which point the slot is cleared and may be reused. If
// that transition fails, the slot remains failed-awaiting-join and the
// error is not discarded; once joined and cleared, a duplicate or stale
// failure delivery must not activate another transition.
type ChildFailedSignalSource struct {
	Slot string
}

func (ChildFailedSignalSource) isSignalSource() {}

// ChildCancelledSignalSource matches the signal produced when the child
// workflow in the named child slot owned by the current workflow (see
// ChildWorkflowSlotDeclaration) terminates itself through an authored
// CancelControl.
//
// The signal schema exposes exactly one field, "reason", typed as the
// built-in string. It never exposes internal child or runtime metadata.
// This signal is distinct from parent-driven cancellation performed with
// CancelChildWorkflowOperation, which never produces a signal because the
// parent already knows it requested the cancellation.
//
// Handling this signal is how a parent joins a self-cancelled child slot:
// when the child cancels itself, the slot becomes cancelled-awaiting-join
// and its reason is held durably until the parent's transition for this
// signal commits successfully, at which point the slot is cleared and may
// be reused. If that transition fails, the slot remains
// cancelled-awaiting-join and the reason is not discarded; once joined and
// cleared, a duplicate or stale cancellation delivery must not activate
// another transition.
type ChildCancelledSignalSource struct {
	Slot string
}

func (ChildCancelledSignalSource) isSignalSource() {}
