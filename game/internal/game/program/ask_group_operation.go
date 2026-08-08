package program

// OpenAskGroupOperation opens one ask-group instance in the statically
// declared workflow-owned slot named Slot, sending the question associated
// with that slot to every user produced by Recipients.
//
// Conceptually, the operation evaluates Recipients (which must compile to
// list<User>, with every recipient identity required to be unique),
// evaluates every entry of Arguments once, in declaration order, as the
// question's shared arguments for the whole group, and evaluates and
// captures Completion. It then creates one pending question instance per
// recipient, mounting that recipient's question presentation (if any)
// independently, and creates a declarative output for each after the
// transition commits. It does not wait, does not suspend the transition,
// does not by itself change the workflow's state, and produces no runtime
// handle — the transition must explicitly move to a state that awaits
// AskGroupCompletedSignalSource through its own WorkflowControl.
//
// # Shared arguments
//
// Arguments are evaluated exactly once for the entire group, not
// separately per recipient — there are no per-recipient argument lambdas
// or callbacks. Per-recipient behavior must instead come from the
// question's authoritative Validation (implicit respondent), the
// question presentation's implicit recipient, a projection's implicit
// viewer, a captured value keyed by User, or authoritative global state.
//
// # Occupied-slot behavior
//
// Opening into a slot that is not empty (collecting or
// completed-awaiting-join) is an execution error: the future engine must
// reject the entire transition atomically, opening no questions, mounting
// no presentations, and leaving the slot's existing contents and every
// other pending mutation or output unchanged. There is no implicit
// replacement, reset, cancellation, result discard, or merge.
//
// Slot is a static, source-level name, not a runtime expression. The
// future compiler validates that Slot and the question it references
// exist, the shape of Completion, and Arguments against the question's
// parameters (no missing, unknown, or duplicate arguments, with compatible
// types); the future runtime additionally validates recipient uniqueness,
// dynamic quorum validity, and slot occupancy.
type OpenAskGroupOperation struct {
	Slot       string
	Recipients Expression
	Arguments  []CallArgument
	Completion AskGroupCompletionPolicy
}

func (OpenAskGroupOperation) isOperation() {}

// FinalizeAskGroupOperation forces a currently collecting ask group in the
// named slot to complete using only the accepted responses received so
// far, without waiting for its completion policy to be otherwise
// satisfied.
//
// It is intended for explicit deadline handling composed with a
// workflow-owned timer slot (see TimerSlotDeclaration): a transition
// handling that timer's expiration finalizes the group instead of the
// group completing on its own. When finalizing a collecting group, every
// still-pending question closes, unanswered recipients become missing,
// previously accepted responses are preserved, the group becomes
// completed-awaiting-join, and a completion signal is delivered later —
// never recursively inside the finalizing transition. This package adds
// no automatic default answer for a missing recipient.
//
// If Slot is completed-awaiting-join, this operation is an idempotent
// no-op, which allows safe answer-versus-deadline race handling: whichever
// of an accepted answer or a deadline finalization is processed first
// wins, and the other becomes a no-op. If Slot is empty, this operation is
// an execution error that the future engine must reject atomically.
//
// Slot is a static, source-level name, not a runtime expression. The
// future compiler validates that Slot exists in the current workflow.
type FinalizeAskGroupOperation struct {
	Slot string
}

func (FinalizeAskGroupOperation) isOperation() {}

// CancelAskGroupOperation abandons a currently collecting ask group in the
// named slot without producing an AskGroupCompletedSignalSource signal.
//
// It is intended for workflow cleanup, participant or session
// cancellation, abandoning a phase, preparing for parent termination, or
// superseding the interaction through explicit game logic. When
// cancelling a collecting group, every pending question closes, every
// accepted response so far is discarded, and the slot is cleared — no
// completion signal is produced.
//
// Cancelling an empty slot is an idempotent no-op. Cancelling a
// completed-awaiting-join slot is an execution error: a completed group's
// result must be joined explicitly through AskGroupCompletedSignalSource
// and must never be silently discarded. This package adds no cancellation
// reason or cancellation signal for ask groups.
//
// Slot is a static, source-level name, not a runtime expression. The
// future compiler validates that Slot exists in the current workflow.
type CancelAskGroupOperation struct {
	Slot string
}

func (CancelAskGroupOperation) isOperation() {}
