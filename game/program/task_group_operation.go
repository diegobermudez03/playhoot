package program

// BeginTaskGroupOperation creates an empty task-group builder in the
// statically declared task-group slot named Slot, capturing Completion as
// the group's completion policy.
//
// The operation refers to a static slot, evaluates and captures
// Completion, and changes the slot from empty to building; it spawns no
// child by itself, does not block or suspend, and does not by itself
// change the workflow's state. A semantically valid transition that begins
// a task group must either seal it (SealTaskGroupOperation) or cancel it
// (CancelTaskGroupOperation) before the transition ends — a group must
// never remain in the building state after a transition commits. The
// future compiler should detect violations of this rule where practical,
// and the future runtime must enforce it dynamically; this package
// performs neither.
//
// Beginning a group in a slot that is not empty is an execution error: the
// future engine must fail the entire transition atomically, with no
// implicit replacement, cancellation, result discard, or reset of the
// slot's existing contents. Slot is a static, source-level name, not a
// runtime expression. A nil Completion may exist in a partially
// constructed, invalid source object, but the future compiler must reject
// it.
type BeginTaskGroupOperation struct {
	Slot       string
	Completion TaskGroupCompletionPolicy
}

func (BeginTaskGroupOperation) isOperation() {}

// SpawnTaskGroupChildOperation adds one child-workflow task definition,
// identified by Key, to the task group currently building in Slot, with
// Arguments as that child's parameters.
//
// The child workflow type comes from the referenced
// TaskGroupSlotDeclaration, so this operation does not itself name a
// workflow. Conceptually, the operation evaluates Key and every entry of
// Arguments in declaration order, captures the resulting values, and
// creates one group-owned task entry, preserving spawn order; it does not
// execute any child logic recursively inside the parent transition, does
// not deliver a WorkflowStarted signal to the child before the parent
// transition commits, does not block or suspend, and produces no runtime
// task handle. This operation is typically used inside a ForEachOperation
// to fan out dynamically over runtime data, with no lambda or per-task
// callback expression required.
//
// A task-group child is owned by the task group (itself owned by the
// parent workflow instance), not by an individual
// ChildWorkflowSlotDeclaration: it exposes no runtime child handle and
// never produces a direct ChildCompletedSignalSource, ChildFailedSignalSource,
// or ChildCancelledSignalSource for the parent — its terminal outcome is
// instead intercepted and aggregated by the task group, observable only
// through TaskGroupCompletedSignalSource.
//
// Slot is a static, source-level name, not a runtime expression. The
// future compiler validates that Slot exists, that it references a valid
// workflow, key-type compatibility, and Arguments against that workflow's
// declared parameters (no missing, unknown, or duplicate arguments, with
// compatible types); the future runtime additionally validates that every
// task key is unique within the group — a duplicate key is an execution
// error that rejects the entire transition atomically.
type SpawnTaskGroupChildOperation struct {
	Slot      string
	Key       Expression
	Arguments []CallArgument
}

func (SpawnTaskGroupChildOperation) isOperation() {}

// SealTaskGroupOperation closes task membership for the task group
// currently building in Slot.
//
// After sealing, no additional task may be added, the group becomes
// running, and its tasks become eligible to later receive their own
// WorkflowStarted signal; completion-policy runtime validation occurs at
// this point, and the group may immediately become completed if its
// policy is already satisfied (for example, an empty
// TaskGroupAllTerminalPolicy group, while sealing an empty
// TaskGroupFirstTerminalPolicy group, or a TaskGroupQuorumTerminalPolicy
// whose count exceeds the sealed task count, are future execution errors).
// Sealing does not block, does not suspend, does not execute any child
// transition recursively, and does not by itself change the parent
// workflow's state.
//
// Sealing a slot that is empty, running, or completed-awaiting-join is an
// execution error — sealing an already-sealed group is not idempotent.
// Slot is a static, source-level name, not a runtime expression.
type SealTaskGroupOperation struct {
	Slot string
}

func (SealTaskGroupOperation) isOperation() {}

// FinalizeTaskGroupOperation forces the sealed, running task group in Slot
// to complete using only the authored terminal outcomes recorded so far.
//
// It is intended for explicit deadline or external-stop handling composed
// with a workflow-owned timer slot (see TimerSlotDeclaration): a
// transition handling that timer's expiration finalizes the group instead
// of the group completing on its own. When finalizing a running group,
// every still-running task is recursively cancelled and its key is
// appended to the completion signal's "unfinished" field in original task
// order; existing successful, failed, and self-cancelled outcomes are
// preserved, the group becomes completed-awaiting-join, and a completion
// signal is delivered later — never recursively inside the finalizing
// transition. This operation never synthesizes a successful result, an
// authored failure, or an authored cancellation reason for a task it
// cancels — such tasks appear only in "unfinished".
//
// If Slot is completed-awaiting-join, this operation is an idempotent
// no-op, which allows safe outcome-versus-deadline race handling: whichever
// of a completing policy or a deadline finalization is processed first
// wins, and the other becomes a no-op. If Slot is empty or building, this
// operation is an execution error that the future engine must reject
// atomically.
//
// Slot is a static, source-level name, not a runtime expression. The
// future compiler validates that Slot exists in the current workflow.
type FinalizeTaskGroupOperation struct {
	Slot string
}

func (FinalizeTaskGroupOperation) isOperation() {}

// CancelTaskGroupOperation abandons the task group in Slot without
// producing a TaskGroupCompletedSignalSource signal, with Reason
// describing the cancellation.
//
// It is intended for workflow cleanup, session cancellation, abandoning an
// entire phase, or explicit parent-driven cancellation. When cancelling a
// group in the building or running state, every created or running task
// is recursively cancelled (with Reason as the parent-driven cancellation
// reason for those descendants, following the same semantics as
// CancelChildWorkflowOperation), descendant questions and timers are
// cleaned up, every collected outcome is discarded, and the slot is
// cleared — no completion signal is produced.
//
// Cancelling an empty slot is an idempotent no-op. Cancelling a
// completed-awaiting-join group is an execution error: a completed
// aggregate result must be joined explicitly through
// TaskGroupCompletedSignalSource and must never be silently discarded.
//
// Slot is a static, source-level name, not a runtime expression. Reason
// must eventually compile to the built-in string type; a nil Reason may
// exist only in a partially constructed, invalid source object, and the
// future compiler must reject it.
type CancelTaskGroupOperation struct {
	Slot   string
	Reason Expression
}

func (CancelTaskGroupOperation) isOperation() {}
