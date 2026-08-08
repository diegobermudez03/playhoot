package program

// MatchControl selects exactly one WorkflowControl outcome by inspecting
// Value against Cases (see MatchPattern), in place of an equivalent chain
// of UnionVariantMatchPattern-shaped ConditionalControl checks.
//
// Only the selected case's Control is evaluated; a selected pattern's
// lexical bindings are visible inside that Control — for example, a
// UnionVariantMatchPattern binding may be referenced from
// CompleteControl.Result, FailControl.Error, CancelControl.Reason, a
// nested ConditionalControl, or a nested MatchControl. MatchControl never
// executes an operation block itself: a transition's synchronous
// operations still run, in full, before its final workflow control is
// evaluated.
type MatchControl struct {
	Value Expression
	Cases []MatchControlCase
}

func (MatchControl) isWorkflowControl() {}

// MatchControlCase pairs one MatchPattern with the WorkflowControl
// selected when that pattern matches. Pattern's lexical bindings, if any,
// are in scope only within this case's Control. This case type has no
// guard field — conditional refinement of a selected case should use a
// nested ConditionalControl in Control.
type MatchControlCase struct {
	Pattern MatchPattern
	Control WorkflowControl
}
