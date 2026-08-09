package program

// ChildWorkflowSlotDeclaration declares a statically named, durable
// child-workflow location owned by each instance of the enclosing
// workflow, refering to exactly one statically declared workflow type by
// name.
//
// A child slot may hold at most one child workflow instance at a time and
// remains addressable across all of the parent instance's state
// transitions — spawning a child in one state and handling its completion
// in a state reached later through GotoControl requires no redeclaration
// or transfer of the slot. A child slot belongs to exactly one parent
// workflow instance, is never a user-visible workflow handle stored in
// workflow-local state, cannot be transferred to another parent, and
// disappears when the parent workflow terminates.
//
// A child slot is conceptually, at runtime, empty, running, or
// completed-awaiting-join; this package does not model those runtime
// states, as they are future engine implementation details.
//
// This package does not validate child-slot name uniqueness within a
// workflow or that Workflow refers to an existing workflow declaration; it
// preserves duplicate and invalid declarations so the future compiler can
// report them deterministically.
//
// Use ChildWorkflowSlotDeclaration when one child has a stable,
// statically named role addressed by a fixed slot and the parent handles
// that one child's individual terminal outcome directly (through
// ChildCompletedSignalSource, ChildFailedSignalSource, or
// ChildCancelledSignalSource). When the number of children is determined
// at runtime, every child shares the same workflow type, and the parent
// wants one aggregated terminal signal instead, use
// TaskGroupSlotDeclaration.
type ChildWorkflowSlotDeclaration struct {
	Name     string
	Workflow string
}

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
// A slot is occupied, and therefore an invalid spawn target, whenever it
// holds a running child or a terminal outcome still awaiting join
// (completed, failed, or cancelled — see ChildCompletedSignalSource,
// ChildFailedSignalSource, and ChildCancelledSignalSource). Spawning into
// an occupied slot is an execution error: the future engine must fail the
// entire transition atomically, creating no child and leaving the slot's
// existing contents and every other pending mutation or output unchanged.
// There is no implicit replacement, cancellation, result discard, restart,
// or slot reset.
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
// This operation is parent-driven cancellation of a running child, which
// is distinct from a child cancelling itself with CancelControl (observed
// by the parent through ChildCancelledSignalSource): parent-driven
// cancellation never produces a child-outcome signal, since the parent
// already knows it requested the cancellation, and the parent transition
// simply continues normally afterward.
//
// Conceptually, the operation evaluates Reason, recursively cancels the
// running child and every descendant it owns, closes pending questions and
// cancels pending timers owned by that child tree, and clears the slot. It
// does not emit any child-outcome signal, does not suspend, and does not
// by itself change the parent workflow's state.
//
// Cancelling an already empty slot is an idempotent no-op, which allows
// unconditional cleanup sequences (for example cancelling a child, a
// timer, and closing a question together) without source-level slot-status
// checks. Cancelling a slot that holds any terminal outcome still awaiting
// join — completed, failed, or cancelled (see ChildCompletedSignalSource,
// ChildFailedSignalSource, and ChildCancelledSignalSource) — is an
// execution error: a completed result, a failure error, or a cancellation
// reason must be explicitly consumed through its corresponding signal,
// never silently discarded — the future compiler may detect some such
// invalid paths statically, but the future runtime must enforce the rule
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
