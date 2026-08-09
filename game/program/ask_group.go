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
