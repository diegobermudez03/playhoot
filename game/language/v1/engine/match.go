package engine

// MatchPattern is the engine's compiled representation of one case
// pattern of a MatchExpression, mirroring program.MatchPattern once
// compiled and resolved: every named type is known to exist and be the
// right kind, and every field binding is known to exist on the matched
// variant.
//
// MatchPattern is a closed interface, mirroring program's own
// closed-interface pattern: its marker method is unexported so packages
// outside engine cannot introduce unsupported variants.
type MatchPattern interface {
	isMatchPattern()
}

// WildcardMatchPattern matches any value, introducing no lexical
// binding.
type WildcardMatchPattern struct{}

func (WildcardMatchPattern) isMatchPattern() {}

// EnumValueMatchPattern matches the statically named ValueName of the
// statically named, resolved enum type TypeName.
type EnumValueMatchPattern struct {
	TypeName  string
	ValueName string
}

func (EnumValueMatchPattern) isMatchPattern() {}

// UnionVariantMatchPattern matches the statically named VariantName of
// the statically named, resolved union type TypeName, optionally
// extracting some of that variant's fields into lexical bindings via
// Bindings.
type UnionVariantMatchPattern struct {
	TypeName    string
	VariantName string
	Bindings    []MatchFieldBinding
}

func (UnionVariantMatchPattern) isMatchPattern() {}

// MatchFieldBinding maps Field, a declared field of a matched variant,
// to the immutable lexical name Name, available only within the
// selected case.
type MatchFieldBinding struct {
	Field string
	Name  string
}

// OptionalNoneMatchPattern matches the absent case of an optional value.
type OptionalNoneMatchPattern struct{}

func (OptionalNoneMatchPattern) isMatchPattern() {}

// OptionalSomeMatchPattern matches the present case of an optional
// value, optionally binding the wrapped value to the lexical name
// Binding. An empty Binding means the case matches without binding it.
type OptionalSomeMatchPattern struct {
	Binding string
}

func (OptionalSomeMatchPattern) isMatchPattern() {}
