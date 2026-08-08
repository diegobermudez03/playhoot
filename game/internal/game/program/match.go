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
