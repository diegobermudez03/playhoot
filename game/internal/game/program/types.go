package program

// TypeReference identifies the type of a value: a built-in, a user-declared
// type, or a compound type built from other type references.
//
// TypeReference is a closed interface. Its marker method is unexported so
// that packages outside program cannot introduce unsupported variants; the
// future compiler can safely exhaust all cases with a type switch.
type TypeReference interface {
	isTypeReference()
}

// BuiltinTypeReference refers to a platform-provided built-in type.
type BuiltinTypeReference struct {
	Type BuiltinType
}

func (BuiltinTypeReference) isTypeReference() {}

// NamedTypeReference refers to a user-declared type by name.
type NamedTypeReference struct {
	Name string
}

func (NamedTypeReference) isTypeReference() {}

// ListTypeReference represents an ordered list of elements of the same
// type.
type ListTypeReference struct {
	Element TypeReference
}

func (ListTypeReference) isTypeReference() {}

// MapTypeReference represents a key/value map. This package does not
// restrict which types may be used as keys; the engine compiler is
// responsible for validating that a key type is supported.
type MapTypeReference struct {
	Key   TypeReference
	Value TypeReference
}

func (MapTypeReference) isTypeReference() {}

// OptionalTypeReference represents either a value of the wrapped type or
// its absence.
type OptionalTypeReference struct {
	Element TypeReference
}

func (OptionalTypeReference) isTypeReference() {}

// TypeDeclaration is a user-declared type in a game definition: an enum, a
// record, a tagged union, or a nominal new type.
//
// TypeDeclaration is a closed interface. Its marker method is unexported so
// that packages outside program cannot introduce unsupported variants; the
// future compiler can safely exhaust all cases with a type switch.
type TypeDeclaration interface {
	isTypeDeclaration()
}

// EnumTypeDeclaration declares a type whose values are exactly one of a
// fixed set of symbolic names.
type EnumTypeDeclaration struct {
	Name   string
	Values []EnumValueDeclaration
}

func (EnumTypeDeclaration) isTypeDeclaration() {}

// EnumValueDeclaration declares a single symbolic value of an enum type.
type EnumValueDeclaration struct {
	Name string
}

// RecordTypeDeclaration declares a type whose values contain all of the
// declared fields.
type RecordTypeDeclaration struct {
	Name   string
	Fields []FieldDeclaration
}

func (RecordTypeDeclaration) isTypeDeclaration() {}

// FieldDeclaration declares a named, typed field of a record or a union
// variant.
type FieldDeclaration struct {
	Name string
	Type TypeReference
}

// UnionTypeDeclaration declares a type whose values contain exactly one of
// the declared variants.
type UnionTypeDeclaration struct {
	Name     string
	Variants []UnionVariantDeclaration
}

func (UnionTypeDeclaration) isTypeDeclaration() {}

// UnionVariantDeclaration declares a single variant of a union type. A
// variant may have zero or more fields; a zero-field variant represents
// cases such as Jail or Finished that carry no data of their own.
type UnionVariantDeclaration struct {
	Name   string
	Fields []FieldDeclaration
}

// NewTypeDeclaration declares a type that is nominally distinct from its
// underlying type. Unlike a type alias, two new types built on the same
// underlying type remain distinct from one another.
type NewTypeDeclaration struct {
	Name       string
	Underlying TypeReference
}

func (NewTypeDeclaration) isTypeDeclaration() {}
