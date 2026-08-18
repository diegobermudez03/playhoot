package program

// ProjectionDeclaration defines a reusable, pure, server-authoritative
// transformation from authoritative game state into a typed model
// intended for one specific viewing user.
//
// A future engine invocation conceptually evaluates a projection against a
// viewer, a set of arguments, and a committed snapshot to produce one
// value of ResultType; this package only represents the declaration, not
// projection invocation, subscription, diffing, caching, or execution.
//
// # Implicit viewer binding
//
// Every projection body has an implicit immutable lexical binding,
// "viewer" (typed User), representing the user the model is being
// produced for. It is not part of Parameters and cannot be supplied or
// overridden by a caller. The future compiler validates that viewer
// resolves to the built-in User type and that no explicit parameter is
// also named "viewer"; this package does not perform that validation.
//
// # Parameters and result type
//
// Parameters are explicit, immutable, named inputs supplied when a future
// presentation or question UI uses the projection; they reuse the
// existing FieldDeclaration and preserve declaration order. ResultType is
// mandatory: a projection with no meaningful domain result must declare
// BuiltinTypeReference{Type: BuiltinTypeUnit} and a Body of
// UnitLiteralExpression{}, rather than a nil ResultType. A nil ResultType
// or Body may exist in a partially constructed, invalid source object, but
// the future compiler must reject it.
//
// # Body scope
//
// A projection Body may directly reference global state through "global",
// immutable resources through "resources", the implicit "viewer" binding,
// its declared Parameters, other user-declared pure functions, and
// engine-provided pure built-in functions. It must not directly reference
// transition- or workflow-specific bindings such as local, signal, actor,
// answer, respondent, workflow parameters, question parameters, or
// transition lexical bindings — data of that kind must instead be passed
// explicitly as a projection parameter, keeping the projection reusable
// independently of any particular workflow instance. This package
// preserves out-of-scope references and unresolved calls so the future
// compiler can produce deterministic diagnostics; it performs no such
// validation itself.
//
// A projection body must use CallExpression to invoke user-declared
// functions or engine built-ins, exactly like any other pure expression
// context; a projection is never itself invoked through CallExpression,
// and projections must not call other projections in this version of the
// language — shared logic between projections should be factored into a
// FunctionDeclaration instead.
//
// # Privacy boundary
//
// A projection is the language's mechanism for information-visibility
// control: authoritative global state may hold data that must never reach
// every user (private cards, hidden roles, unrevealed answers, secret
// decisions, internal scoring detail, anti-cheat state), and a projection
// is responsible for producing only the subset of that data visible to
// its viewer. A future client is expected to receive projection results,
// never unrestricted access to global state. This package does not add
// field-level visibility annotations — visibility is expressed entirely
// through projection logic.
//
// # Purity, determinism, and relationship to effects
//
// A projection body is pure: it must not mutate global or workflow-local
// state, open or close questions, schedule or cancel timers, spawn or
// cancel child workflows, emit effects, change workflow control, perform
// external I/O, read wall-clock time, or use operating-system randomness.
// Given the same compiled program version, snapshot, viewer, and
// arguments, a projection must always return the same result, and only a
// successfully committed snapshot may ever be projected — a rejected
// transition never produces a new projection result. A projection
// represents current visible state (for example scores, positions, or
// hands); transient presentation events such as animations belong to
// EffectDeclaration instead, and a client that misses an effect should
// still be able to recover current state from a projection result. This
// package does not modify QuestionDeclaration or EffectDeclaration.
//
// # Relationship to pure functions
//
// Unlike a FunctionDeclaration, which has only explicit parameters, may
// read resources but never global state directly, and has no implicit
// viewer, a projection may read global directly and always has the
// implicit viewer binding; a projection produces client-visible data,
// while a function is a general-purpose reusable computation usable from
// server logic, invariants, validators, and projections alike.
//
// # Ordering
//
// ProjectionDeclaration occupies its own logical declaration namespace.
// This package preserves declaration and parameter order, duplicate
// projection names, names overlapping with functions or other
// declarations, and unresolved function calls — it does not silently
// merge or reject any of them; the future compiler owns those decisions.
type ProjectionDeclaration struct {
	Name       string
	Parameters []FieldDeclaration
	ResultType TypeReference
	Body       Expression
}
