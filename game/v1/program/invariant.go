package program

// InvariantDeclaration is a named, pure boolean condition over
// authoritative global game state that must hold for every committed game
// snapshot, such as "scores are never negative" or "a finished match has a
// winner".
//
// Condition must eventually compile to the built-in bool type; a nil
// Condition may exist in a partially constructed, invalid source object,
// but the future engine compiler must reject it. This package reuses the
// existing Expression model rather than introducing a dedicated
// invariant-expression interface, and does not evaluate or validate
// Condition itself.
//
// # Scope
//
// An invariant's Condition may reference global state through the
// reserved root "global", immutable resources through "resources", other
// user-declared pure functions, and engine-provided pure built-in
// functions. It must not directly reference transition-specific or
// workflow-specific bindings such as local, signal, actor, answer,
// respondent, viewer, workflow parameters, question parameters, or lexical
// bindings created inside a transition. Logic that needs to be reused
// across invariants (or between invariants and other pure contexts) should
// be extracted into a FunctionDeclaration and called with explicit
// arguments. This package preserves out-of-scope references so the future
// compiler can produce deterministic diagnostics; it does not enforce this
// restriction itself.
//
// # Purity and determinism
//
// Given the same global state, the same immutable resources, and the same
// compiled program version, an invariant must always produce the same
// boolean result. It must not mutate state, open or close questions,
// schedule or cancel timers, spawn or cancel child workflows, emit
// effects, read wall-clock time, use operating-system randomness, or
// access network, database, or other external state — the Expression-only
// body naturally rules out operations, but purity of any called functions
// or built-ins is a future compiler concern.
//
// # Evaluation timing
//
// Conceptually, the future engine evaluates every invariant twice: once
// after initial global-state construction, before the initial game
// snapshot is accepted, and once after a transition has executed its
// operations and workflow control against a working candidate snapshot,
// before that candidate becomes the new committed snapshot. Invariants are
// only checked against this final candidate state, never after each
// individual operation inside a transition's block — an invariant may be
// temporarily false partway through a block (for example, between removing
// a token from one collection and appending it to another) as long as it
// holds once the transition's operations have all finished.
//
// # Violation semantics
//
// If any invariant evaluates to false for a candidate snapshot, the future
// engine treats the entire step as a rejected execution error, not an
// authored outcome: the original snapshot remains unchanged, and no
// workflow state change, state mutation, question, timer, child workflow,
// effect, or projection update from that step is committed. In particular,
// an invariant violation must never execute FailControl or CancelControl,
// must never produce a child-failure or child-cancellation signal, and
// must never transition the workflow into another authored state — the
// workflow simply remains at its previous state because the candidate
// transition never commits. This package declares invariants only; it
// implements none of this checking or rejection behavior, and adds no
// repair operations, fallback values, corrective workflow controls,
// violation effects, violation-specific signals, or custom error handlers.
// Recoverable domain validation belongs in workflow guards, question
// validation, explicit conditional operations, or successful domain result
// types instead.
//
// # Authored versus engine structural invariants
//
// InvariantDeclaration represents only authored global invariants:
// game-specific rules over authored global state and immutable resources,
// declared by the game author. It is distinct from engine structural
// invariants (for example, that every child workflow has exactly one
// parent, or that a completed child result is never silently discarded),
// which the future engine enforces internally and which have no
// source-language declaration in this package.
//
// # Ordering
//
// Because invariant conditions are pure, their declaration order has no
// effect on game state; order is preserved in Definition.Invariants purely
// for deterministic diagnostics, traces, debugging, and source fidelity.
// This package does not reject duplicate invariant names — it preserves
// them so the future compiler can report them deterministically.
type InvariantDeclaration struct {
	Name      string
	Condition Expression
}
