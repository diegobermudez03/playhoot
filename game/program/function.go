package program

// FunctionDeclaration defines a reusable, deterministic, pure,
// value-producing computation.
//
// A function receives immutable Parameters, evaluates exactly one
// expression Body, and returns that expression's result as ResultType. It
// has no operation block: it cannot mutate state, suspend, open a
// question, schedule a timer, spawn a workflow, emit an effect, or change
// workflow control, and it performs no external I/O.
//
// Given the same parameter values, the same immutable resources, and the
// same compiled program version, a function must always produce the same
// result. It must not depend on wall-clock time, operating-system
// randomness, network state, database state, current workflow state, or
// any mutable state (global or workflow-local) — randomness and other
// non-deterministic values must instead be modeled through explicit
// engine-controlled operations or passed in explicitly as parameters.
//
// Body may reference Parameters, immutable resources through the reserved
// root "resources", other user-declared functions, and engine-provided
// pure built-in functions; it must not reference mutable or
// execution-specific roots such as global, local, signal, answer,
// respondent, actor, or viewer. If a function needs mutable game data, the
// caller must pass it explicitly as a parameter (for example
// HasWon(participant: id, tokens: global.tokens) rather than an implicit
// read of global.tokens), keeping the function reusable across guards,
// operations, question validation, projections, and invariants. This
// package does not enforce that scope restriction.
//
// Functions are declared only at the root Definition level: there are no
// nested function declarations, closures, captured lexical variables,
// anonymous functions, or lambda expressions. Every dependency must come
// from a declared parameter, immutable resources, or another statically
// named function.
//
// The initial language does not support recursion — direct, indirect, or
// cyclic function-call graphs are all disallowed — but this package still
// preserves such definitions so the future compiler can report
// deterministic diagnostics; it performs no recursion detection itself.
// User-declared functions share one logical function namespace; this
// package preserves duplicate function names and calls to undeclared
// functions rather than rejecting or resolving them.
//
// ResultType is mandatory: a function with no meaningful domain result
// must declare BuiltinTypeReference{Type: BuiltinTypeUnit} and a Body of
// UnitLiteralExpression{}, rather than a nil ResultType. A nil ResultType
// or Body may exist in a partially constructed, invalid source object, but
// the future compiler must reject it.
type FunctionDeclaration struct {
	Name string

	// Parameters are immutable lexical bindings available throughout
	// Body, declared with the existing FieldDeclaration and passed by
	// callers through named CallExpression arguments rather than
	// position. Parameter order is preserved for source fidelity,
	// diagnostics, and documentation.
	Parameters []FieldDeclaration

	ResultType TypeReference
	Body       Expression
}
