package engine

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
