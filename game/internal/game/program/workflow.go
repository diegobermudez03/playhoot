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

// WorkflowStateDeclaration is a stable checkpoint in the lifecycle of a
// workflow instance.
//
// A state holds no arbitrary long-running code, no implicit thread, no
// implicit on-enter block, and no mutable data of its own independent from
// workflow-local state. A workflow instance remains in its current state
// until a signal activates one of the state's transitions or a matching
// global transition.
type WorkflowStateDeclaration struct {
	Name        string
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
