package engine

// TaskGroupCompletionPolicy is the compiled representation of one
// program.TaskGroupCompletionPolicy, evaluated when a BeginTaskGroupOperation
// executes — see engine.TaskGroupCompletionKind for its resolved,
// durable runtime form once a group is begun.
//
// TaskGroupCompletionPolicy is a closed interface, mirroring program's
// own closed-interface pattern.
type TaskGroupCompletionPolicy interface {
	isTaskGroupCompletionPolicy()
}

// TaskGroupAllTerminalPolicy completes a task group once every sealed
// task has reached one authored terminal outcome.
type TaskGroupAllTerminalPolicy struct{}

func (TaskGroupAllTerminalPolicy) isTaskGroupCompletionPolicy() {}

// TaskGroupFirstTerminalPolicy completes a task group as soon as the
// first task reaches any authored terminal outcome.
type TaskGroupFirstTerminalPolicy struct{}

func (TaskGroupFirstTerminalPolicy) isTaskGroupCompletionPolicy() {}

// TaskGroupQuorumTerminalPolicy completes a task group once Count tasks
// have reached an authored terminal outcome. Count is evaluated once
// when the group begins — see program.TaskGroupQuorumTerminalPolicy.
type TaskGroupQuorumTerminalPolicy struct {
	Count Expression
}

func (TaskGroupQuorumTerminalPolicy) isTaskGroupCompletionPolicy() {}

// BeginTaskGroupOperation changes the named task-group slot Slot from
// empty to building, capturing Completion for the group's whole
// lifetime. It spawns no task by itself. Beginning an already occupied
// slot — building, running, or a terminal outcome still awaiting join —
// is an execution error that fails the transition atomically. See
// program.BeginTaskGroupOperation.
type BeginTaskGroupOperation struct {
	Slot       string
	Completion TaskGroupCompletionPolicy
}

func (BeginTaskGroupOperation) isOperation() {}

// SpawnTaskGroupChildOperation adds one task to the currently building
// task group in the named slot Slot, under the task key produced by
// Key, passing Arguments as the new child's parameters. The compiler
// guarantees Arguments matches the slot's declared workflow's declared
// parameters exactly. Spawning into a slot that is not currently
// building, or under a Key already used by another task in the same
// group, is an execution error that fails the transition atomically.
// This does not itself apply the new task's WorkflowStarted transition
// — see program.SpawnTaskGroupChildOperation.
type SpawnTaskGroupChildOperation struct {
	Slot      string
	Key       Expression
	Arguments []CallArgument
}

func (SpawnTaskGroupChildOperation) isOperation() {}

// SealTaskGroupOperation closes membership on the currently building
// task group in the named slot Slot: no further task may be added, and
// the group becomes running. Sealing validates the group's completion
// policy against its final task count — an AskGroupFirstResponsePolicy-style
// zero-task first-terminal group, or a quorum exceeding the final task
// count, is an execution error. A trivially satisfied policy (an
// all-terminal group with zero tasks) completes the group immediately,
// within this same operation. Sealing a slot that is empty, running, or
// completed-awaiting-join is an execution error — unlike
// FinalizeTaskGroupOperation, sealing an already-sealed group is never
// an idempotent no-op. See program.SealTaskGroupOperation.
type SealTaskGroupOperation struct {
	Slot string
}

func (SealTaskGroupOperation) isOperation() {}

// FinalizeTaskGroupOperation forces the currently running task group in
// the named slot Slot to complete using only the authored terminal
// outcomes recorded so far — every task that has not yet reached one is
// structurally cancelled without producing an authored outcome, and
// reported as unfinished. Finalizing a slot already completed-awaiting-join
// is an idempotent no-op — the documented resolution for an
// answer-versus-deadline race, see program.FinalizeTaskGroupOperation.
// Finalizing an empty or still-building slot is an execution error.
type FinalizeTaskGroupOperation struct {
	Slot string
}

func (FinalizeTaskGroupOperation) isOperation() {}

// CancelTaskGroupOperation abandons the task group in the named slot
// Slot — building or running — without producing a
// TaskGroupCompletedSignalSource signal: every created or running task
// is discarded and the slot is cleared. Cancelling an already empty
// slot is an idempotent no-op. Cancelling a slot that holds a terminal
// outcome still awaiting join is an execution error — that outcome must
// be joined through TaskGroupCompletedSignalSource first, never
// silently discarded. See program.CancelTaskGroupOperation.
type CancelTaskGroupOperation struct {
	Slot   string
	Reason Expression
}

func (CancelTaskGroupOperation) isOperation() {}
