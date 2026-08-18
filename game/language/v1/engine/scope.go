package engine

// Reserved lexical scope root names, referenced throughout program's and
// engine's own documentation (see, for example, program.InvariantDeclaration's
// documented "global" root, program.ProjectionDeclaration's "global"
// and "resources", and program.ViewDeclaration's "local"). Promoted to
// named constants here — rather than left as inline string literals in
// engineservice — because both engineservice's compiler and its
// runtime, which live in separate internal packages, independently need
// to agree on exactly these names, and engine is the one package both
// already depend on.
const (
	// GlobalScopeRootName is the reserved lexical root bound to a game
	// instance's mutable global state.
	GlobalScopeRootName = "global"

	// LocalScopeRootName is the reserved lexical root bound to a
	// workflow instance's own mutable local state, or, within a
	// View's element tree, to that view's own client-local state.
	LocalScopeRootName = "local"

	// ResourcesScopeRootName is the reserved lexical root bound to a
	// game's immutable, program-level resource values.
	ResourcesScopeRootName = "resources"
)

// Scope is an immutable set of named runtime bindings an Expression
// evaluates against — for example a pure function's evaluated
// arguments, or a list query's per-element item and index bindings.
//
// A Scope is never mutated in place: introducing an additional binding
// (as a function call, match case, or list query iteration does)
// produces a new Scope rather than changing an existing one, so a Scope
// already in use by an enclosing evaluation is never affected by a
// nested one.
type Scope struct {
	Bindings map[string]Value
}

// Lookup returns the Value bound to name, if any.
func (s Scope) Lookup(name string) (Value, bool) {
	v, ok := s.Bindings[name]
	return v, ok
}
