package program

// TaskGroupSlotDeclaration declares a statically named, durable,
// workflow-owned structured-concurrency location containing a dynamically
// sized collection of child workflow instances of the same declared
// workflow type, each addressed by a typed task key.
//
// Use a task group (rather than ChildWorkflowSlotDeclaration) when the
// number of children depends on runtime data, every child uses the same
// multi-step workflow type, each child needs its own local state,
// questions, timers, or presentations, and the parent wants one aggregated
// terminal signal instead of individually handling each child. Use a task
// group (rather than AskGroupSlotDeclaration) when children are full
// multi-step workflows rather than a single typed question with one
// response per user.
//
// Every task in the group runs the workflow named by Workflow — task
// groups are homogeneous, and this package does not support mixing
// workflow types within one slot. KeyType is the type used to address
// tasks and aggregate their outcomes; the future compiler requires it to
// be valid as a map key (for example a TeamId, ParticipantId, User, or
// string). A slot may hold at most one task-group instance at a time
// (conceptually empty, building, running, or completed-awaiting-join —
// this package adds no public runtime-state enum for that), remains
// addressable across all of the parent instance's state transitions,
// belongs to exactly one parent workflow instance, may be reused once its
// previous group is joined or cancelled, and is never represented as
// ordinary workflow-local state, a user-visible handle, a TypeReference,
// or transferable between parent workflows.
//
// This package does not validate task-group slot-name uniqueness within a
// workflow, that Workflow or KeyType are valid, or key-type map-key
// validity; it preserves duplicate and invalid declarations so the future
// compiler can report them deterministically.
type TaskGroupSlotDeclaration struct {
	Name     string
	Workflow string
	KeyType  TypeReference
}

// TaskGroupCompletionPolicy determines when a task group's collected task
// outcomes are considered complete.
//
// A child workflow reaches an authored terminal outcome by executing
// CompleteControl (success), FailControl (failure), or CancelControl
// (self-cancellation); an engine execution error is never an authored
// terminal outcome and never counts toward a completion policy.
//
// TaskGroupCompletionPolicy is a closed interface. Its marker method is
// unexported so that packages outside program cannot introduce unsupported
// variants; the future compiler can safely exhaust all cases with a type
// switch. A policy is evaluated once when its task group begins (see
// BeginTaskGroupOperation) and does not change afterward.
type TaskGroupCompletionPolicy interface {
	isTaskGroupCompletionPolicy()
}

// TaskGroupAllTerminalPolicy completes a task group once every task in the
// sealed group has reached one authored terminal outcome (successful,
// failed, or self-cancelled, in any combination) — the parent inspects
// the aggregated outcome maps to decide what those outcomes mean for the
// game.
//
// For an empty sealed group, the group is considered complete immediately
// after the sealing transition commits; even so, completion is still
// delivered later through TaskGroupCompletedSignalSource rather than
// recursively activating a parent transition inside that transition.
type TaskGroupAllTerminalPolicy struct{}

func (TaskGroupAllTerminalPolicy) isTaskGroupCompletionPolicy() {}

// TaskGroupFirstTerminalPolicy completes a task group as soon as the first
// task reaches any authored terminal outcome (successful, failed, or
// self-cancelled).
//
// Once that first outcome is recorded, every other still-running task is
// structurally cancelled and reported in the completion signal's
// "unfinished" field — cancelling for this reason never produces an
// authored failed or cancelled outcome for those tasks. Sealing a
// first-terminal group with zero tasks is a future execution error.
type TaskGroupFirstTerminalPolicy struct{}

func (TaskGroupFirstTerminalPolicy) isTaskGroupCompletionPolicy() {}

// TaskGroupQuorumTerminalPolicy completes a task group once Count tasks
// have reached authored terminal outcomes (successful, failed, or
// self-cancelled, in any combination).
//
// Count is evaluated once when the group begins; it must eventually
// compile to number and evaluate to a positive integer not exceeding the
// group's final sealed task count — an invalid runtime quorum fails the
// operation that begins the group atomically. Once quorum is reached,
// every other still-running task is structurally cancelled and reported
// as unfinished. This package adds no success-only, weighted, or
// percentage-based quorum variant, and no way to change a policy after the
// group begins — a majority can be expressed by computing the desired
// count before the group begins.
type TaskGroupQuorumTerminalPolicy struct {
	Count Expression
}

func (TaskGroupQuorumTerminalPolicy) isTaskGroupCompletionPolicy() {}
