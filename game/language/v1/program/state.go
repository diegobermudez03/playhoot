package program

// ResourceDeclaration declares an immutable, game-level value shared
// conceptually by every session running the same compiled program version.
//
// Resources are used for data such as board layouts, card definitions,
// available colors, rule configuration, question banks, scoring tables, or
// immutable visual asset references.
//
// A resource's Value is evaluated from source expressions during
// compilation or program loading, not per game session, and a resource is
// never part of a mutable game snapshot. The future engine compiler is
// responsible for validating that Value matches Type, uses only
// expressions that are pure and independent of mutable state, runtime
// users, signals, random values, or workflow-local values, and does not
// form a cyclic dependency with other resources. This package does not
// perform that validation and does not provide any way to mutate a
// resource declaration after construction.
type ResourceDeclaration struct {
	Name  string
	Type  TypeReference
	Value Expression
}

// StateDeclaration describes a mutable state object: a named, typed set of
// fields together with the initializer used when the owning scope is
// created. It is shared by global state, workflow-local state, and
// client-local UI state, each instantiated independently from its own
// StateDeclaration and modified thereafter only by its owning scope's
// operations.
type StateDeclaration struct {
	Fields []StateFieldDeclaration
}

// StateFieldDeclaration declares a single field of a StateDeclaration.
//
// Initializer is evaluated when the owning state scope is created (for
// global state, when a new game session's snapshot is created) and defines
// the field's starting value. It is represented through the Expression
// interface rather than a generic value so that the source model does not
// depend on a runtime value representation; a nil Initializer may appear in
// a partially constructed, invalid source model, but it is not a valid
// source-language initializer and will be diagnosed by the future engine
// compiler.
type StateFieldDeclaration struct {
	Name        string
	Type        TypeReference
	Initializer Expression
}
