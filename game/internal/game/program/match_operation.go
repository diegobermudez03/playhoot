package program

// MatchOperation performs synchronous operation branching: it inspects
// Value against Cases (see MatchPattern) and executes the Body of the
// first matching case, in place of an equivalent chain of
// UnionVariantMatchPattern-shaped IfOperation checks.
//
// Exactly one case's Body executes. Each selected Body creates a nested
// lexical scope containing that case's pattern bindings and may use any
// existing synchronous operation — including nested IfOperation,
// ForEachOperation, or MatchOperation, drawing deterministic random
// values with DrawRandomOperation, or opening questions, scheduling
// timers, spawning children, and emitting effects through their existing
// operations — but a Body cannot itself choose the transition's workflow
// control; that remains the transition's separate, final phase. Bindings
// introduced by the selected pattern or created inside Body do not escape
// the case.
//
// MatchOperation produces no value and participates in the same atomic
// transition model as every other Operation: if a selected case's Body
// later causes an execution error, no operation from that case, or any
// earlier operation in the transition, commits.
type MatchOperation struct {
	Value Expression
	Cases []MatchOperationCase
}

func (MatchOperation) isOperation() {}

// MatchOperationCase pairs one MatchPattern with the Block executed when
// that pattern is selected. Pattern's lexical bindings, if any, are in
// scope only within this case's Body. This case type has no guard field —
// conditional refinement of a selected case should use a nested
// IfOperation in Body.
type MatchOperationCase struct {
	Pattern MatchPattern
	Body    Block
}
