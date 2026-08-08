package program

// OpenQuestionOperation opens one concrete instance of the question
// associated with the named workflow slot.
//
// Conceptually, the operation evaluates Recipient, evaluates every entry
// of Arguments in declaration order, and opens the question in Slot,
// recording the target user and captured arguments. It does not wait for
// the user, does not suspend the transition, and does not by itself change
// the workflow's state — the enclosing transition must still choose its
// next state through its WorkflowControl. Opening a slot that already
// holds a pending question is an execution error that must fail the
// entire transition atomically, with no partial state changes or outputs
// committed; there is no implicit replacement policy, so an occupied slot
// must be explicitly closed with CloseQuestionOperation before it can be
// reopened.
//
// Slot is a static, source-level name, not a runtime expression. The
// future compiler validates that Slot exists in the current workflow, that
// Recipient has type User, and that Arguments matches the slot's question
// parameters (no missing, unknown, or duplicate arguments, with compatible
// types).
type OpenQuestionOperation struct {
	Slot      string
	Recipient Expression
	Arguments []CallArgument
}

func (OpenQuestionOperation) isOperation() {}

// CloseQuestionOperation closes the pending question instance in the named
// workflow slot without producing a question-answer signal.
//
// It is used for cleanup, workflow cancellation, participant disconnection,
// abandoning an interaction after another condition wins, or closing stale
// client-facing UI before changing phases. Closing an already empty slot
// is an idempotent no-op. After a slot is closed, later client responses
// for the old question instance are rejected, and the slot may be opened
// again; the client-facing question should eventually be removed through a
// declarative output.
//
// Slot is a static, source-level name, not a runtime expression. The
// future compiler validates that Slot exists in the current workflow.
type CloseQuestionOperation struct {
	Slot string
}

func (CloseQuestionOperation) isOperation() {}

// EmitEffectOperation emits one instance of the named EffectDeclaration to
// Recipients with the given Arguments.
//
// Recipients must eventually compile to list<User> — even a single
// recipient must be represented as a one-element list, so the language
// never needs a User-or-list-of-User union. The operation evaluates
// Recipients and every entry of Arguments in declaration order and creates
// a declarative client-facing output; it performs no network I/O, renders
// no UI, does not suspend, and does not mutate authoritative state by
// itself. The emitted effect becomes externally visible only after the
// containing transition commits successfully — if the transition fails or
// violates an invariant, the effect is discarded along with the rest of
// the uncommitted work.
//
// The future compiler validates that Effect exists, that Recipients has
// type list<User>, and that Arguments matches the effect's parameters (no
// missing, unknown, or duplicate arguments, with compatible types).
type EmitEffectOperation struct {
	Effect     string
	Recipients Expression
	Arguments  []CallArgument
}

func (EmitEffectOperation) isOperation() {}
