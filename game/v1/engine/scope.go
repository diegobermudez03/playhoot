package engine

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
