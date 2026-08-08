package program

// WorkflowDeclaration defines one reusable, deterministic, signal-driven
// finite-state process.
//
// A workflow instance changes state only in response to signals: there is
// no implicit state-entry code, no implicit background execution, and no
// automatic transition chain without an intervening signal. Every state
// change is the result of exactly one WorkflowControl produced by a
// matched transition.
type WorkflowDeclaration struct {
	Name string

	// Parameters are immutable lexical values supplied when a workflow
	// instance is created. The future compiler validates parameter-name
	// uniqueness, parameter types, and argument compatibility at workflow
	// start.
	Parameters []FieldDeclaration

	// ResultType is the type produced by CompleteControl when the
	// workflow instance completes successfully. It is mandatory: a
	// workflow with no meaningful result must declare
	// BuiltinTypeReference{Type: BuiltinTypeUnit} rather than leaving
	// ResultType nil.
	ResultType TypeReference

	// LocalState declares the mutable state owned by each workflow
	// instance. Every instance receives an independent copy initialized
	// from LocalState's field initializers. Workflow-local state belongs
	// to the workflow instance, is part of the future game snapshot, is
	// distinct from global state, and survives across the instance's
	// transitions; it disappears once the workflow completes, fails, or
	// is cancelled, except for its final result and trace.
	LocalState StateDeclaration

	// QuestionSlots declares the statically named question slots owned by
	// each instance of this workflow. See QuestionSlotDeclaration for its
	// semantics.
	QuestionSlots []QuestionSlotDeclaration

	// TimerSlots declares the statically named timer slots owned by each
	// instance of this workflow. See TimerSlotDeclaration for its
	// semantics.
	TimerSlots []TimerSlotDeclaration

	// ChildSlots declares the statically named child-workflow slots owned
	// by each instance of this workflow. See ChildWorkflowSlotDeclaration
	// for its semantics.
	ChildSlots []ChildWorkflowSlotDeclaration

	// Presentations declares this workflow's workflow-level presentations,
	// each active for the entire lifetime of a workflow instance —
	// created when the instance is created, remaining active across every
	// state transition, and unmounted only when the instance completes,
	// fails, or is cancelled. See PresentationDeclaration for its
	// semantics. A child workflow instance owns its own workflow-level
	// presentations independently, active only while that child instance
	// exists.
	Presentations []PresentationDeclaration

	// InitialState names the state a new workflow instance begins in.
	// The future compiler validates that the named state exists.
	InitialState string

	// GlobalTransitions are workflow-level fallback transitions that may
	// apply from any current state. When a signal is being resolved, a
	// state-local transition for that signal (see
	// WorkflowStateDeclaration.Transitions) takes priority over a global
	// transition for the same signal; a global transition only applies
	// when the current state has none for that signal. Global transitions
	// are intended for cross-cutting concerns such as cancellation,
	// parent cancellation, session shutdown, participant unavailability,
	// or workflow-wide deadlines. This package does not implement that
	// resolution order or reject signals with no matching transition.
	GlobalTransitions []TransitionDeclaration

	// States are the workflow's declared states, in declaration order.
	States []WorkflowStateDeclaration
}

// QuestionSlotDeclaration declares a statically named, durable interaction
// location owned by each instance of the enclosing workflow.
//
// A slot is associated with exactly one QuestionDeclaration named by
// Question and may hold at most one pending question instance at a time;
// it may be reopened once its previous question is answered or closed. A
// slot remains addressable across all of the workflow instance's state
// transitions and disappears only when the workflow instance terminates.
// This durability is why a question slot is declared statically here
// rather than represented as ordinary user-defined state: it is never a
// StateFieldDeclaration, a runtime handle stored in local state, a
// TypeReference, or a lexical binding, and its pending question instance
// is tracked by the future engine's workflow snapshot metadata, not by the
// source model.
//
// This package does not validate slot-name uniqueness within a workflow or
// that Question refers to an existing question declaration; it preserves
// duplicates so the future compiler can report them deterministically.
type QuestionSlotDeclaration struct {
	Name     string
	Question string

	// Presentation optionally connects this slot's pending question
	// instance to a presentation slot, projection, and view. A nil
	// Presentation means opening the question still creates an
	// authoritative pending question with no automatically mounted
	// authored client view — it may still be answered through another
	// supported client or testing mechanism; nil is never treated as an
	// implicit default presentation. See QuestionPresentationDeclaration
	// for its semantics.
	Presentation *QuestionPresentationDeclaration
}

// WorkflowStateDeclaration is a stable checkpoint in the lifecycle of a
// workflow instance.
//
// A state holds no arbitrary long-running code, no implicit thread, no
// implicit on-enter block, and no mutable data of its own independent from
// workflow-local state. A workflow instance remains in its current state
// until a signal activates one of the state's transitions or a matching
// global transition.
type WorkflowStateDeclaration struct {
	Name string

	// Presentations declares this state's state-level presentations, each
	// active only while the workflow instance remains in this state. A
	// presentation instance is created when the workflow enters the
	// state and unmounted when the workflow leaves it (through any
	// WorkflowControl) or terminates; re-entering the same state later
	// creates a new presentation instance with freshly initialized
	// view-local state. See PresentationDeclaration for its semantics.
	// Declaring presentations here is declarative state, not implicit
	// entry or exit code — this package adds no OnEnter, OnExit, mount,
	// or unmount operations.
	Presentations []PresentationDeclaration

	Transitions []TransitionDeclaration
}

// TransitionDeclaration declares how a workflow instance in a given state
// responds to one matched signal.
//
// A transition is conceptually executed in this order: (1) match the
// incoming signal against Signal; (2) bind the signal's fields per
// Signal.Bindings; (3) evaluate Guard, if any; (4) execute Operations in
// declaration order; (5) evaluate and apply exactly one WorkflowControl
// result from Control. A future engine executes this sequence atomically:
// if any step fails, no partial mutation made by Operations is committed,
// no Control result is applied, and the workflow instance's prior snapshot
// is unchanged.
//
// Each state, and separately each set of global transitions, may declare
// at most one transition per signal name for this version of the
// language; branching behavior for a single signal must be expressed with
// IfOperation inside Operations and ConditionalControl inside Control
// rather than with multiple same-signal transitions. This package does not
// reject duplicate same-signal transitions — it preserves them so the
// future compiler can diagnose them deterministically.
type TransitionDeclaration struct {
	// Name is a source-level transition identifier used for diagnostics,
	// traces, debugging, and generated explanations. It does not affect
	// signal matching.
	Name string

	// Signal identifies the signal handled by this transition and the
	// lexical bindings extracted from its payload.
	Signal SignalPattern

	// Guard is an optional precondition evaluated after signal binding
	// and before Operations executes. A nil Guard means the transition is
	// unconditional once its signal matches; there is no special "true"
	// sentinel for this case. A non-nil Guard must eventually compile to
	// a boolean value. If Guard evaluates to false, the signal is
	// rejected for this transition. Guard evaluation must not mutate
	// state.
	Guard Expression

	// Operations is the synchronous operation block executed, in
	// declaration order, after Guard passes. It may create immutable
	// lexical bindings, mutate authorized global or workflow-local state,
	// and execute finite conditionals and loops, but it cannot itself
	// choose the workflow's next state.
	Operations Block

	// Control is evaluated after Operations finishes and produces the
	// transition's single workflow-control outcome. A nil Control may
	// exist in a partially constructed, invalid source object, but every
	// semantically valid transition must have exactly one; the future
	// compiler is responsible for rejecting a nil Control.
	Control WorkflowControl
}
