package engine

// Function, Question, Projection, and Effect are the engine's named,
// reusable, program-level declarations: each is compiled exactly once
// from its program.Definition counterpart, stored keyed by name in one
// of Program's own catalogs (Functions, Questions, Projections,
// Effects), and referenced by name from wherever a workflow uses it —
// never redeclared or recompiled per use. Function and Projection share
// an identical shape (Name, Parameters, ResultType, Body); what
// distinguishes them is where their Body may look and who is allowed to
// call them — see each type's own doc comment.

// Function is the compiled representation of one user-declared pure
// function, mirroring program.FunctionDeclaration once compiled: every
// parameter's type reference is resolved, and Body is compiled and
// known to statically match ResultType.
//
// Calling a Function evaluates Body in a Scope containing exactly its
// evaluated arguments, bound by parameter name — a function body never
// sees the caller's own scope, matching program.FunctionDeclaration's
// documented restriction that a function may only reference its own
// Parameters, resources, other functions, and built-ins.
type Function struct {
	Name       string
	Parameters []FieldType
	ResultType Type
	Body       Expression
}

// Question is the compiled representation of one
// program.QuestionDeclaration.
//
// Validation is nil when the source declaration had none — meaning only
// ResponseType compatibility applies to a submitted answer. When
// non-nil, the compiler guarantees it is statically bool and compiles
// against a scope containing "resources", "global", the reserved
// bindings "respondent" (user) and "answer" (ResponseType), and each of
// Parameters by name — see program.QuestionDeclaration's documented
// Validation scope.
type Question struct {
	Name         string
	Parameters   []FieldType
	ResponseType Type
	Validation   Expression
}

// Projection is the compiled representation of one
// program.ProjectionDeclaration: a pure, server-authoritative
// transformation from authoritative game state into a typed model for
// one specific viewing user.
//
// A Projection is evaluated against the reserved lexical roots
// "global" and "resources", the implicit immutable "viewer" binding
// (typed user, supplied by whoever mounts the projection — a
// Presentation's Targets or a pending question's recipient), and
// Parameters, in that scope. It never sees "local", a signal's fields,
// or any other workflow- or transition-specific binding — see
// program.ProjectionDeclaration's documented privacy boundary.
type Projection struct {
	Name       string
	Parameters []FieldType
	ResultType Type
	Body       Expression
}

// Effect is the compiled representation of one program.EffectDeclaration:
// a reusable, typed, client-facing presentation event's payload shape.
// Effect declares no view, transport, or rendering behavior — see
// EmitEffectOperation and EmitEffectOutput for how one is produced and
// delivered.
type Effect struct {
	Name       string
	Parameters []FieldType
}
