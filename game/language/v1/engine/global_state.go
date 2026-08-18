package engine

// StateField is the compiled representation of one field of
// program.Definition.GlobalState, mirroring program.StateFieldDeclaration
// once compiled: Type is resolved, and Initializer is compiled and known
// to statically match Type.
//
// A StateField is evaluated once per new game instance — see
// engineservice.NewSnapshot — not once per compiled Program, since each
// instance owns an independent copy of global state.
type StateField struct {
	Name        string
	Type        Type
	Initializer Expression
}

// Invariant is the compiled representation of one
// program.InvariantDeclaration, mirroring it once compiled: Condition is
// compiled and known to statically be bool.
//
// engineservice.NewSnapshot evaluates every Invariant against a game
// instance's initial global state before accepting it; a future step
// evaluation does the same against every committed transition's
// candidate state. A false or erroring Invariant rejects the state that
// produced it — see program.InvariantDeclaration's "Violation semantics".
type Invariant struct {
	Name      string
	Condition Expression
}
