package engine

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
