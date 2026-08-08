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
