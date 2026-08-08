package program

// MatchExpression is a pure, value-producing expression that inspects
// Value against Cases (see MatchPattern) and evaluates to the Result of
// the first matching case.
//
// Every case's Result must eventually be assignable to one common result
// type. A MatchExpression may be used anywhere an Expression is otherwise
// legal — resource expressions, state initializers, pure function bodies,
// invariant conditions, projection bodies, question validators, transition
// guards, operation values, presentation expressions, view expressions, UI
// action values, and workflow controls — without weakening whatever scope
// restrictions already apply in that context (for example, a
// MatchExpression inside a pure function body still cannot reference
// global). The future compiler validates the matched expression's type,
// pattern compatibility, case exhaustiveness and reachability,
// result-type compatibility across cases, and lexical binding scope; this
// package performs none of that validation.
type MatchExpression struct {
	Value Expression
	Cases []MatchExpressionCase
}

func (MatchExpression) isExpression() {}

// MatchExpressionCase pairs one MatchPattern with the Result expression
// evaluated when that pattern is selected. Pattern's lexical bindings, if
// any, are in scope only within this case's Result. This case type has no
// guard field — conditional refinement of a selected case should use a
// nested ConditionalExpression in Result.
type MatchExpressionCase struct {
	Pattern MatchPattern
	Result  Expression
}
