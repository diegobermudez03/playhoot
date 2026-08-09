package engine

// Type is the engine's compiled representation of a type: the resolved
// shape a Value must have to be valid where that Type is required.
//
// Type is engine's own internal representation, decoupled from
// program.TypeReference and program.TypeDeclaration the same way
// engine.Program is decoupled from program.Definition (see program.go
// and doc.go): a future compilation step builds a Type from a
// program.Definition's type declarations, resolving every named
// reference into the shape declared here. This package does not yet
// implement that resolution; it only defines the shape Type takes once
// resolved.
//
// Type is a closed interface, mirroring program's own closed-interface
// pattern: its marker method is unexported so packages outside engine
// cannot introduce unsupported variants, and a type switch inside this
// package can safely be treated as exhaustive.
type Type interface {
	isType()

	// Kind identifies which of Type's variants this is, without
	// requiring a type switch.
	Kind() Kind

	// Equal reports whether other describes the same type. For a named
	// type (EnumType, RecordType, UnionType, NewType) this compares
	// only the declared Name: a compiled program never declares two
	// types of the same kind sharing a name, so Name alone is a type's
	// complete nominal identity. For a structural type (UnitType,
	// BoolType, NumberType, StringType, UserType, OptionalType,
	// ListType, MapType) this compares Kind and, for the compound
	// cases, recursively compares element/key/value types.
	Equal(other Type) bool
}

// UnitType describes the single value of the unit type.
type UnitType struct{}

func (UnitType) isType()    {}
func (UnitType) Kind() Kind { return KindUnit }
func (UnitType) Equal(other Type) bool {
	_, ok := other.(UnitType)
	return ok
}

// BoolType describes boolean values.
type BoolType struct{}

func (BoolType) isType()    {}
func (BoolType) Kind() Kind { return KindBool }
func (BoolType) Equal(other Type) bool {
	_, ok := other.(BoolType)
	return ok
}

// NumberType describes numeric values.
type NumberType struct{}

func (NumberType) isType()    {}
func (NumberType) Kind() Kind { return KindNumber }
func (NumberType) Equal(other Type) bool {
	_, ok := other.(NumberType)
	return ok
}

// StringType describes text values.
type StringType struct{}

func (StringType) isType()    {}
func (StringType) Kind() Kind { return KindString }
func (StringType) Equal(other Type) bool {
	_, ok := other.(StringType)
	return ok
}

// UserType describes a real user connected to a room (see
// program.BuiltinTypeUser). Its runtime meaning is defined by this
// engine.
type UserType struct{}

func (UserType) isType()    {}
func (UserType) Kind() Kind { return KindUser }
func (UserType) Equal(other Type) bool {
	_, ok := other.(UserType)
	return ok
}

// EnumType describes values that are exactly one of Values, a fixed set
// of symbolic names, for the named enum type Name.
type EnumType struct {
	Name   string
	Values []string
}

func (EnumType) isType()    {}
func (EnumType) Kind() Kind { return KindEnum }
func (t EnumType) Equal(other Type) bool {
	o, ok := other.(EnumType)
	return ok && t.Name == o.Name
}

// HasValue reports whether name is one of t's declared symbolic values.
func (t EnumType) HasValue(name string) bool {
	for _, v := range t.Values {
		if v == name {
			return true
		}
	}
	return false
}

// FieldType declares the type of one named field of a RecordType or of
// one UnionVariantType.
type FieldType struct {
	Name string
	Type Type
}

// RecordType describes values that contain exactly Fields, for the named
// record type Name.
type RecordType struct {
	Name   string
	Fields []FieldType
}

func (RecordType) isType()    {}
func (RecordType) Kind() Kind { return KindRecord }
func (t RecordType) Equal(other Type) bool {
	o, ok := other.(RecordType)
	return ok && t.Name == o.Name
}

// FieldByName returns t's field declaration named name, if any.
func (t RecordType) FieldByName(name string) (FieldType, bool) {
	return fieldTypeByName(t.Fields, name)
}

// UnionVariantType declares the fields of one variant of a UnionType. A
// variant with no fields carries no data of its own.
type UnionVariantType struct {
	Name   string
	Fields []FieldType
}

// FieldByName returns v's field declaration named name, if any.
func (v UnionVariantType) FieldByName(name string) (FieldType, bool) {
	return fieldTypeByName(v.Fields, name)
}

// UnionType describes values that contain exactly one of Variants, for
// the named union type Name.
type UnionType struct {
	Name     string
	Variants []UnionVariantType
}

func (UnionType) isType()    {}
func (UnionType) Kind() Kind { return KindUnion }
func (t UnionType) Equal(other Type) bool {
	o, ok := other.(UnionType)
	return ok && t.Name == o.Name
}

// VariantByName returns t's variant declaration named name, if any.
func (t UnionType) VariantByName(name string) (UnionVariantType, bool) {
	for _, v := range t.Variants {
		if v.Name == name {
			return v, true
		}
	}
	return UnionVariantType{}, false
}

// NewType describes values nominally distinct from Underlying, for the
// named new type Name. Two NewType values are only Equal to one another
// when Name also matches, even if both wrap an equal Underlying.
type NewType struct {
	Name       string
	Underlying Type
}

func (NewType) isType()    {}
func (NewType) Kind() Kind { return KindNewType }
func (t NewType) Equal(other Type) bool {
	o, ok := other.(NewType)
	return ok && t.Name == o.Name
}

// OptionalType describes either a value of Element or its absence.
type OptionalType struct {
	Element Type
}

func (OptionalType) isType()    {}
func (OptionalType) Kind() Kind { return KindOptional }
func (t OptionalType) Equal(other Type) bool {
	o, ok := other.(OptionalType)
	return ok && t.Element.Equal(o.Element)
}

// ListType describes an ordered list of elements of type Element.
type ListType struct {
	Element Type
}

func (ListType) isType()    {}
func (ListType) Kind() Kind { return KindList }
func (t ListType) Equal(other Type) bool {
	o, ok := other.(ListType)
	return ok && t.Element.Equal(o.Element)
}

// MapType describes a key/value map from Key to Value.
type MapType struct {
	Key   Type
	Value Type
}

func (MapType) isType()    {}
func (MapType) Kind() Kind { return KindMap }
func (t MapType) Equal(other Type) bool {
	o, ok := other.(MapType)
	return ok && t.Key.Equal(o.Key) && t.Value.Equal(o.Value)
}

func fieldTypeByName(fields []FieldType, name string) (FieldType, bool) {
	for _, f := range fields {
		if f.Name == name {
			return f, true
		}
	}
	return FieldType{}, false
}
