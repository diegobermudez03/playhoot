package program

// ScheduleTimerOperation schedules a timer in a statically declared timer
// slot owned by the current workflow instance, to fire after
// DelayMilliseconds evaluates.
//
// The operation evaluates DelayMilliseconds and records a pending logical
// timer in Slot; it creates a declarative scheduling output only after the
// enclosing transition commits. It does not block, does not suspend the
// transition, does not immediately activate another transition, and does
// not read a clock or perform real scheduling itself — expiration can only
// arrive later as a TimerExpiredSignalSource signal, even when
// DelayMilliseconds evaluates to zero, keeping the language strictly
// signal-driven.
//
// Scheduling into an already occupied slot is an execution error: the
// future engine must fail the entire transition atomically, leaving the
// previous pending timer, state, and any other outputs unchanged. There is
// no implicit replacement, reset, extension, or coalescing — a workflow
// must explicitly cancel an existing timer with CancelTimerOperation
// before scheduling another one into the same slot.
//
// Slot is a static, source-level name, not a runtime expression.
// DelayMilliseconds must eventually compile to number; the future engine
// additionally requires the evaluated delay to be a finite, non-negative
// integer number of milliseconds within configured limits. This package
// does not introduce a dedicated Duration built-in type.
type ScheduleTimerOperation struct {
	Slot              string
	DelayMilliseconds Expression
}

func (ScheduleTimerOperation) isOperation() {}

// CancelTimerOperation cancels the currently pending timer in the named
// workflow timer slot, if one exists, without producing a
// TimerExpiredSignalSource signal.
//
// The operation clears the logical timer from Slot and creates a
// declarative cancellation output after commit when necessary; it does not
// suspend and is idempotent when the slot is already empty. After
// cancellation the slot may be scheduled again, and any later external
// delivery of the cancelled timer's expiration must not activate a
// workflow transition — duplicate external cancellation delivery is
// likewise harmless.
//
// When a workflow instance completes, fails, or is cancelled, every timer
// it owns is logically cancelled and every timer slot is cleared as part
// of that termination, independently of any explicit CancelTimerOperation.
//
// Slot is a static, source-level name, not a runtime expression. The
// future compiler validates that Slot exists in the current workflow.
type CancelTimerOperation struct {
	Slot string
}

func (CancelTimerOperation) isOperation() {}
