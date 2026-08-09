package program

// AskGroupSlotDeclaration declares a statically named, durable, workflow-owned
// location for a coordinated multi-user interaction: opening the same
// typed question for several users and later joining one aggregated
// completion result.
//
// Use an ask group when several users independently answer one question
// as part of one logical interaction step (every player selecting a card,
// team members voting, collecting simultaneous secret choices, waiting for
// a quorum of responses). Ask groups are for one-step parallel
// interactions; multi-step parallel processes belong to child workflows
// (see ChildWorkflowSlotDeclaration) and future task groups.
//
// A slot references exactly one QuestionDeclaration by Question — the same
// reusable, typed question contract used by single-recipient question
// slots — and may hold at most one ask-group instance (conceptually empty,
// collecting, or completed-awaiting-join; this package adds no public
// runtime-state enum for that) at a time. A slot remains addressable
// across all of the workflow instance's state transitions, belongs to
// exactly one workflow instance, may be reused once its previous group has
// been joined or cancelled, and is never represented as ordinary
// workflow-local state, a user-visible handle, a TypeReference, or
// transferable between workflows.
//
// Presentation reuses the existing QuestionPresentationDeclaration rather
// than a dedicated ask-group presentation type. For every recipient of the
// group, one concrete pending question instance is created and the
// presentation is mounted independently for that recipient: the
// projection's implicit viewer and the presentation's implicit recipient
// are both that recipient, while the question's captured arguments are
// shared by the whole group and available to every recipient's projection
// by name. A nil Presentation means the ask group is authoritative and
// headless — questions are still created and may still be answered
// through another client or test mechanism, but no authored view is
// mounted automatically; nil is never treated as an implicit default
// presentation.
//
// This package does not validate ask-group slot-name uniqueness within a
// workflow, that Question refers to an existing question declaration, or
// presentation references; it preserves duplicate and invalid declarations
// so the future compiler can report them deterministically.
type AskGroupSlotDeclaration struct {
	Name         string
	Question     string
	Presentation *QuestionPresentationDeclaration
}

// AskGroupCompletionPolicy determines when an ask group's collected
// answers are considered complete.
//
// AskGroupCompletionPolicy is a closed interface. Its marker method is
// unexported so that packages outside program cannot introduce unsupported
// variants; the future compiler can safely exhaust all cases with a type
// switch. A policy is evaluated once when its ask group is opened and does
// not change afterward.
type AskGroupCompletionPolicy interface {
	isAskGroupCompletionPolicy()
}

// AskGroupAllResponsesPolicy completes an ask group once every unique
// recipient has submitted one accepted answer.
//
// For an empty recipient list, the group is considered complete
// immediately after its opening transition commits; even so, completion is
// still delivered later through AskGroupCompletedSignalSource rather than
// recursively activating a workflow transition inside the opening
// transition.
type AskGroupAllResponsesPolicy struct{}

func (AskGroupAllResponsesPolicy) isAskGroupCompletionPolicy() {}

// AskGroupFirstResponsePolicy completes an ask group as soon as the first
// accepted answer is processed.
//
// Once complete, every other recipient's pending question closes,
// unanswered recipients are reported as missing, and any later answer is
// rejected. Opening a first-response group with no recipients is a future
// execution error.
type AskGroupFirstResponsePolicy struct{}

func (AskGroupFirstResponsePolicy) isAskGroupCompletionPolicy() {}

// AskGroupQuorumPolicy completes an ask group once Count unique recipients
// have submitted accepted answers.
//
// Count is evaluated once when the group is opened; it must eventually
// compile to number and evaluate to a positive integer not exceeding the
// unique recipient count — an invalid runtime quorum fails the entire
// opening transition atomically. Once quorum is reached, every other
// recipient's pending question closes and unanswered recipients are
// reported as missing. This package adds no percentage-based, majority,
// or weighted quorum variant, and no way to change a policy after opening
// — a majority can be expressed by computing the desired count before
// opening the group.
type AskGroupQuorumPolicy struct {
	Count Expression
}

func (AskGroupQuorumPolicy) isAskGroupCompletionPolicy() {}

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
