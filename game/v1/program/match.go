package program

// MatchPattern is a source-language pattern shared by MatchExpression,
// MatchOperation, and MatchControl for inspecting an enum, tagged-union,
// or optional value.
//
// Authored pattern matching in this version of the language is only for
// enum, tagged-union, and optional values; the future compiler must reject
// matching directly on bool, number, string, unit, list, map, record, a
// new type, or User unless the value is wrapped in a supported enum,
// union, or optional type. Primitive branching remains expressible
// through ConditionalExpression, IfOperation, ConditionalControl, and
// equality/ordering expressions. This package adds no literal patterns,
// nested patterns, list/map/record destructuring, or case guards in this
// step.
//
// Every match form evaluates its matched value exactly once, inspects
// cases in source order, selects the first matching case, introduces that
// pattern's immutable lexical bindings, and evaluates only that case —
// there is no fallthrough into a later case. Case order is therefore
// semantically significant and is preserved through slices, never maps.
//
// A semantically valid match must be exhaustive: every enum value or every
// union variant or both optional cases must be covered, or a reachable
// WildcardMatchPattern must exist. This package performs no
// exhaustiveness, reachability, or duplicate-case validation — it
// preserves missing cases, duplicate cases, unreachable cases, mixed
// pattern families, multiple wildcards, and empty case lists so the future
// compiler can report deterministic diagnostics.
//
// MatchPattern is a closed interface. Its marker method is unexported so
// that packages outside program cannot introduce unsupported variants;
// the future compiler can safely exhaust all cases with a type switch.
type MatchPattern interface {
	isMatchPattern()
}

// WildcardMatchPattern matches any value accepted by the surrounding
// match, introducing no lexical binding. It is the only way to represent
// an explicit catch-all case — there is no separate default, else, or
// fallback field on a match declaration. The future compiler normally
// requires a wildcard to be the final reachable case, but this package
// still preserves one placed earlier so the compiler can produce an
// unreachable-case diagnostic.
type WildcardMatchPattern struct{}

func (WildcardMatchPattern) isMatchPattern() {}

// EnumValueMatchPattern matches the statically named ValueName of the
// statically named enum type TypeName (for example MatchPhase.PLAYING).
// Both names are kept explicit; this package does not infer TypeName from
// an unqualified value name. The pattern introduces no lexical bindings.
// The future compiler validates that TypeName exists and is an enum, that
// ValueName is one of its values, that the matched expression's type is
// compatible, and duplicate or unreachable enum cases.
type EnumValueMatchPattern struct {
	TypeName  string
	ValueName string
}

func (EnumValueMatchPattern) isMatchPattern() {}

// UnionVariantMatchPattern matches the statically named VariantName of the
// statically named tagged-union type TypeName, optionally extracting some
// of that variant's fields into immutable lexical bindings via Bindings.
//
// Each MatchFieldBinding maps one declared field of the matched variant to
// an immutable lexical name available only inside the selected case;
// bindings preserve source order, extract direct fields only (no nested or
// recursive field patterns), never modify the matched value, and may omit
// fields the case does not need — a variant with fields may still use a
// nil or empty Bindings when the case only needs to identify the variant,
// and a zero-field variant is naturally represented with a nil or empty
// Bindings.
//
// The future compiler validates that TypeName exists and is a union, that
// VariantName is one of its variants, that every bound field exists on
// that variant, duplicate field bindings, duplicate lexical names,
// binding-name conflicts, and matched-value compatibility; this package
// preserves invalid bindings for deterministic diagnostics.
type UnionVariantMatchPattern struct {
	TypeName    string
	VariantName string
	Bindings    []MatchFieldBinding
}

func (UnionVariantMatchPattern) isMatchPattern() {}

// OptionalNoneMatchPattern matches the absent case of an optional value.
// The optional's element type is inferred from the matched expression, so
// this pattern carries no element-type field. It introduces no lexical
// binding.
type OptionalNoneMatchPattern struct{}

func (OptionalNoneMatchPattern) isMatchPattern() {}

// OptionalSomeMatchPattern matches the present case of an optional value,
// optionally binding the wrapped value to the immutable lexical name
// Binding, scoped only to the selected case. An empty Binding means the
// case matches a present value without binding it. The future compiler
// validates binding-name conflicts and matched-value compatibility.
type OptionalSomeMatchPattern struct {
	Binding string
}

func (OptionalSomeMatchPattern) isMatchPattern() {}

// MatchFieldBinding maps Field, a declared field of a matched
// UnionVariantMatchPattern's variant, to the immutable lexical name Name,
// available only within that case.
type MatchFieldBinding struct {
	Field string
	Name  string
}

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
