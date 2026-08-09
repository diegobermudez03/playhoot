package program

// PresentationSlotDeclaration declares a statically named location in a
// user's client interface, such as "main", "hud", "primaryInteraction",
// "modal", "overlay", or "sidebar".
//
// A presentation slot is scoped per user: the same slot name for two
// different users never conflicts, but for one initial-language version,
// a given user may have at most one active presentation — workflow-level,
// state-level, or pending-question — occupying a given slot at a time.
// This package adds no priority, replacement, queueing, stacking, or
// multiplicity configuration for slots, and slot names are always static:
// they are never computed from an expression.
//
// This package does not validate slot-name uniqueness; it preserves
// duplicates so the future compiler can report them deterministically.
type PresentationSlotDeclaration struct {
	Name string
}

// PresentationDeclaration declaratively connects a target list of users, a
// presentation slot, a projection (with its arguments), and a view, for
// use as a workflow-level or state-level presentation.
//
// # Identity
//
// Name is the presentation's static, source-level identity within its
// owning scope (a WorkflowDeclaration or a WorkflowStateDeclaration),
// combined conceptually with the owning workflow instance and the target
// user to form a stable presentation-instance identity. That identity is
// what lets a future client preserve the mounted view's local state while
// the presentation stays active; this package exposes no runtime
// presentation-instance identifiers. Name is used for diagnostics, traces,
// debugging, and generated explanations; the future compiler validates its
// uniqueness within scope, and this package preserves duplicates for
// deterministic diagnostics.
//
// # Targets
//
// Targets is an expression that must eventually compile to list<User>,
// even when only one user is targeted — the language has no User-or-list
// union. The future engine evaluates Slot (see PresentationSlotDeclaration)
// independently for every user Slot's expression produces at commit time
// and requires those users to be unique by identity; a target list
// containing the same user more than once is an execution error, never
// silently deduplicated or silently multiply mounted.
//
// # Projection and view
//
// Slot statically names one PresentationSlotDeclaration; the future
// compiler rejects unknown slot names, and slot names are never
// expressions. Projection statically names one ProjectionDeclaration,
// evaluated independently per target user with that user bound as the
// projection's implicit viewer and ProjectionArguments (reusing the
// existing CallArgument model, in declaration order) supplied as its
// arguments; the resulting value becomes the immutable model of the view
// named by View. The future compiler requires the projection's ResultType
// to be assignable to the view's ModelType. The mounted view receives only
// its model and its own local state — never direct access to the
// workflow, global state, resources, or viewer.
type PresentationDeclaration struct {
	Name                string
	Slot                string
	Targets             Expression
	Projection          string
	ProjectionArguments []CallArgument
	View                string
}

// QuestionPresentationDeclaration declaratively connects a pending
// question instance to a presentation slot, projection, and view, for a
// workflow-owned QuestionSlotDeclaration.
//
// # Why it belongs to the question slot, not the question
//
// A QuestionDeclaration is a reusable authoritative request contract that
// different workflows may present differently — for example, the same
// ConfirmAction question might be a modal in one workflow, a sidebar panel
// in another, and have no authored UI at all in an automated test
// workflow. Presentation configuration therefore belongs to the
// workflow-owned QuestionSlotDeclaration.Presentation, not to
// QuestionDeclaration itself.
//
// # Implicit recipient binding and target
//
// Unlike PresentationDeclaration, a question presentation has no Targets
// field: its target is always the pending question's recipient. When its
// projection is evaluated, the projection's implicit viewer is bound to
// that recipient, so only the intended recipient ever receives the
// mounted question view.
//
// # Argument scope
//
// ProjectionArguments (reusing the existing CallArgument model, in
// declaration order) may reference the question's captured parameters by
// name, the implicit "recipient" binding (typed User — the user the
// pending question was opened for), global, resources, user-declared pure
// functions, and engine-provided pure built-ins. They must not reference
// workflow-local state, workflow parameters, transition lexical bindings,
// signal, actor, answer, or respondent: any workflow-specific value the
// question UI needs must instead be captured as a question parameter when
// OpenQuestionOperation executes, keeping a pending question self-contained
// and durable independently of workflow-state changes. This package
// preserves out-of-scope references for future compiler diagnostics; it
// performs no such validation itself.
//
// # Lifecycle
//
// When an OpenQuestionOperation commits successfully for a slot with a
// non-nil Presentation, the view named by View is mounted for the
// recipient, with its model produced by evaluating Projection against the
// committed snapshot, the recipient as viewer, and ProjectionArguments.
// The question presentation survives workflow-state changes, because it
// belongs to the question slot (part of the workflow instance) rather than
// to one state. It is unmounted when an answer is accepted, when
// CloseQuestionOperation closes the question, or when the owning workflow
// completes, fails, or is cancelled. Reopening the same slot later creates
// a new presentation instance with freshly initialized view-local state.
type QuestionPresentationDeclaration struct {
	Slot                string
	Projection          string
	ProjectionArguments []CallArgument
	View                string
}
