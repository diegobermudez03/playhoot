package program

// TimerSlotDeclaration declares a statically named, durable timer location
// owned by each instance of the enclosing workflow.
//
// A slot may hold at most one pending timer at a time and may be reused
// once that timer expires or is cancelled. A slot remains active across
// all of the workflow instance's state transitions — scheduling a timer in
// one state and handling its expiration in a state reached later through
// GotoControl requires no redeclaration or transfer — and disappears only
// when the owning workflow instance terminates. This durability, and the
// fact that pending-timer metadata belongs to the future engine's
// workflow-instance snapshot, is why a timer slot is declared statically
// here rather than represented as ordinary user-defined state: it is never
// a StateFieldDeclaration, a TypeReference, a lexical binding, a
// dynamically generated string, or a handle stored manually in
// workflow-local state.
//
// This package does not validate timer-slot name uniqueness within a
// workflow; it preserves duplicates so the future compiler can report them
// deterministically.
//
// Explicit question timeouts are expressed by composing a question slot, a
// timer slot, an open-question operation, a schedule-timer operation, and
// separate transitions for the question-answered and timer-expired
// signals — this package intentionally has no dedicated timeout field on
// QuestionDeclaration, QuestionSlotDeclaration, or OpenQuestionOperation,
// and no dedicated "question timed out" signal source.
type TimerSlotDeclaration struct {
	Name string
}

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

// KeyedTimerSlotDeclaration declares a statically named, durable timer
// location owned by each instance of the enclosing workflow, generalizing
// TimerSlotDeclaration to hold several independent, simultaneously
// pending timers at once, addressed by an authored key rather than at
// most one pending timer per slot.
//
// Conceptual identity is (slot, key): at most one timer may be pending
// per exact (Name, key) tuple. Different keys under the same slot have
// fully independent timers — scheduling "disconnect_timeout" for key
// "P1" never affects, or is affected by, "disconnect_timeout" for key
// "P2".
//
// This package does not validate timer-slot name uniqueness within a
// workflow; it preserves duplicates so the future compiler can report
// them deterministically.
type KeyedTimerSlotDeclaration struct {
	Name string

	// KeyType is the authored type every key value for this slot must
	// match. It may be any TypeReference usable as a map key (see
	// MapTypeReference) — this package places no additional restriction
	// on it.
	KeyType TypeReference
}

// ScheduleKeyedTimerOperation schedules a timer at the named workflow
// keyed slot's authored Key, to fire after DelayMilliseconds evaluates,
// generalizing ScheduleTimerOperation to a (slot, key) occurrence
// instead of a slot-wide one.
//
// The operation evaluates Key (which must eventually compile to the
// slot's declared KeyType) and DelayMilliseconds, and records a pending
// logical timer at (Slot, Key); it creates a declarative scheduling
// output only after the enclosing transition commits, exactly like
// ScheduleTimerOperation. Scheduling into an already-occupied (slot,
// key) is an execution error: the future engine must fail the entire
// transition atomically, leaving the previous pending timer and every
// other pending mutation or output unchanged — there is no implicit
// replacement, reset, extension, or coalescing. A workflow must
// explicitly cancel an existing timer with CancelKeyedTimerOperation
// before scheduling another one into the same (slot, key). A different
// key under the same Slot is entirely independent and unaffected.
//
// Slot is a static, source-level name, not a runtime expression. Key
// must be statically compatible with the slot's declared KeyType.
// DelayMilliseconds must eventually compile to number; the future engine
// additionally requires the evaluated delay to be a finite, non-negative
// integer number of milliseconds within configured limits.
type ScheduleKeyedTimerOperation struct {
	Slot              string
	Key               Expression
	DelayMilliseconds Expression
}

func (ScheduleKeyedTimerOperation) isOperation() {}

// CancelKeyedTimerOperation cancels the currently pending timer at the
// named workflow keyed slot's authored Key, if one exists, without
// producing a KeyedTimerExpiredSignalSource signal, generalizing
// CancelTimerOperation to a (slot, key) occurrence instead of a
// slot-wide one.
//
// The operation clears the logical timer at (Slot, Key) and creates a
// declarative cancellation output after commit when necessary; it is
// idempotent when (slot, key) is already empty. After cancellation,
// (slot, key) may be scheduled again. A different key under the same
// Slot is entirely unaffected.
//
// Slot is a static, source-level name, not a runtime expression. Key
// must be statically compatible with the slot's declared KeyType.
type CancelKeyedTimerOperation struct {
	Slot string
	Key  Expression
}

func (CancelKeyedTimerOperation) isOperation() {}
