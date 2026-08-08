package program

// SpawnChildWorkflowOperation creates one child workflow instance in the
// statically declared child slot named Slot, passing Arguments as the
// child's parameters.
//
// The child workflow type comes from the referenced ChildWorkflowSlotDeclaration,
// so this operation does not itself name a workflow. Conceptually, the
// operation evaluates every entry of Arguments in declaration order,
// captures the evaluated values, creates a child instance in the slot with
// the current workflow instance as its parent, initializes the child's
// parameters and local state, places it in its declared initial state, and
// causes a later built-in WorkflowStarted signal to target the child. It
// does not execute any child transition recursively inside the parent
// transition, does not block or suspend the parent, does not by itself
// change the parent's state, and does not produce a runtime handle — the
// parent must use its own WorkflowControl to move into a state that waits
// for or otherwise manages the child.
//
// Spawning into a slot that is not empty (whether running or holding a
// completed child awaiting join) is an execution error: the future engine
// must fail the entire transition atomically, creating no child and
// leaving the slot's existing contents and every other pending mutation
// or output unchanged. There is no implicit replacement, cancellation,
// result discard, restart, or slot reset.
//
// Slot is a static, source-level name, not a runtime expression. The
// future compiler validates that Slot exists, that it references a valid
// workflow, and that Arguments matches that workflow's declared parameters
// (no missing, unknown, or duplicate arguments, with compatible types).
type SpawnChildWorkflowOperation struct {
	Slot      string
	Arguments []CallArgument
}

func (SpawnChildWorkflowOperation) isOperation() {}

// CancelChildWorkflowOperation explicitly cancels the running child
// workflow instance in the named child slot, with Reason describing the
// cancellation.
//
// Conceptually, the operation evaluates Reason, recursively cancels the
// running child and every descendant it owns, closes pending questions and
// cancels pending timers owned by that child tree, and clears the slot. It
// does not emit a ChildCompletedSignalSource signal, does not suspend, and
// does not by itself change the parent workflow's state.
//
// Cancelling an already empty slot is an idempotent no-op, which allows
// unconditional cleanup sequences (for example cancelling a child, a
// timer, and closing a question together) without source-level slot-status
// checks. Cancelling a slot that holds a successfully completed child
// still awaiting join is an execution error: a completed child's result
// must be explicitly consumed through ChildCompletedSignalSource, never
// silently discarded — the future compiler may detect some such invalid
// paths statically, but the future runtime must enforce the rule
// dynamically in every case.
//
// Slot is a static, source-level name, not a runtime expression. Reason
// must be a non-nil expression in any semantically valid transition; a nil
// Reason may exist only in a partially constructed, invalid source object,
// and this package does not introduce a dedicated cancellation-reason
// built-in type.
type CancelChildWorkflowOperation struct {
	Slot   string
	Reason Expression
}

func (CancelChildWorkflowOperation) isOperation() {}
