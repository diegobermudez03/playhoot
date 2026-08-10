package engine

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
