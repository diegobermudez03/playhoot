package session

// Kind identifies which variant a Value is, without requiring a caller to
// import game/language/v1/engine to distinguish them.
type Kind int

const (
	KindUnit Kind = iota
	KindBool
	KindNumber
	KindString
	KindUser
	KindEnum
	KindRecord
	KindUnion
	KindNewType
	KindOptional
	KindList
	KindMap
)

// Type is this package's own mirror of the Game Language engine's own Type:
// the shape a Value conforms to, exposed so a consumer can render/act on a
// value's shape even without an instance to inspect (an empty ListValue's
// element shape, and so on). Type is a closed interface: its marker method
// is unexported so packages outside this one cannot introduce unsupported
// variants.
type Type interface {
	isType()
	Kind() Kind
}

type UnitType struct{}

func (UnitType) isType()    {}
func (UnitType) Kind() Kind { return KindUnit }

type BoolType struct{}

func (BoolType) isType()    {}
func (BoolType) Kind() Kind { return KindBool }

type NumberType struct{}

func (NumberType) isType()    {}
func (NumberType) Kind() Kind { return KindNumber }

type StringType struct{}

func (StringType) isType()    {}
func (StringType) Kind() Kind { return KindString }

type UserType struct{}

func (UserType) isType()    {}
func (UserType) Kind() Kind { return KindUser }

// EnumType describes values that are exactly one of Values, for the named
// enum type Name.
type EnumType struct {
	Name   string
	Values []string
}

func (EnumType) isType()    {}
func (EnumType) Kind() Kind { return KindEnum }

// FieldType declares the type of one named field of a RecordType or of one
// UnionVariantType.
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

// UnionVariantType declares the fields of one variant of a UnionType.
type UnionVariantType struct {
	Name   string
	Fields []FieldType
}

// UnionType describes values that contain exactly one of Variants, for the
// named union type Name.
type UnionType struct {
	Name     string
	Variants []UnionVariantType
}

func (UnionType) isType()    {}
func (UnionType) Kind() Kind { return KindUnion }

// NewType describes values nominally distinct from Underlying, for the
// named new type Name.
type NewType struct {
	Name       string
	Underlying Type
}

func (NewType) isType()    {}
func (NewType) Kind() Kind { return KindNewType }

// OptionalType describes either a value of Element or its absence.
type OptionalType struct {
	Element Type
}

func (OptionalType) isType()    {}
func (OptionalType) Kind() Kind { return KindOptional }

// ListType describes an ordered list of elements of type Element.
type ListType struct {
	Element Type
}

func (ListType) isType()    {}
func (ListType) Kind() Kind { return KindList }

// MapType describes a key/value map from Key to Value.
type MapType struct {
	Key   Type
	Value Type
}

func (MapType) isType()    {}
func (MapType) Kind() Kind { return KindMap }

// Value is this package's own mirror of the Game Language engine's own
// Value: every kind of value a Presentation's Model or an Effect/Question's
// Arguments can carry, exposed so a consumer never needs to import
// game/language/v1/engine to read one. Value is a closed interface, the
// same way Type is: its marker method is unexported so packages outside
// this one cannot introduce unsupported variants.
type Value interface {
	isValue()
	Kind() Kind
}

type UnitValue struct{}

func (UnitValue) isValue()   {}
func (UnitValue) Kind() Kind { return KindUnit }

type BoolValue struct{ Value bool }

func (BoolValue) isValue()   {}
func (BoolValue) Kind() Kind { return KindBool }

type NumberValue struct{ Value float64 }

func (NumberValue) isValue()   {}
func (NumberValue) Kind() Kind { return KindNumber }

type StringValue struct{ Value string }

func (StringValue) isValue()   {}
func (StringValue) Kind() Kind { return KindString }

// UserValue is a real user, resolved to their public UserUUID.
type UserValue struct{ UserUUID UserUUID }

func (UserValue) isValue()   {}
func (UserValue) Kind() Kind { return KindUser }

// EnumValue is exactly one symbolic value, ValueName, of the named enum type
// TypeName.
type EnumValue struct {
	TypeName  string
	ValueName string
}

func (EnumValue) isValue()   {}
func (EnumValue) Kind() Kind { return KindEnum }

// FieldValue assigns a value to a named field within a RecordValue or a
// union variant's Fields.
type FieldValue struct {
	Name  string
	Value Value
}

// RecordValue is a value of the named record type TypeName, containing
// Fields.
type RecordValue struct {
	TypeName string
	Fields   []FieldValue
}

func (RecordValue) isValue()   {}
func (RecordValue) Kind() Kind { return KindRecord }

// UnionValue is a value of the named union type TypeName, selecting the
// single variant VariantName.
type UnionValue struct {
	TypeName    string
	VariantName string
	Fields      []FieldValue
}

func (UnionValue) isValue()   {}
func (UnionValue) Kind() Kind { return KindUnion }

// NewTypeValue is a value of the named new type TypeName, wrapping
// Underlying.
type NewTypeValue struct {
	TypeName   string
	Underlying Value
}

func (NewTypeValue) isValue()   {}
func (NewTypeValue) Kind() Kind { return KindNewType }

// OptionalValue is either a wrapped Value of ElementType or its absence
// (Value nil).
type OptionalValue struct {
	ElementType Type
	Value       Value
}

func (OptionalValue) isValue()   {}
func (OptionalValue) Kind() Kind { return KindOptional }

// ListValue is an ordered list of Elements, each of type ElementType.
type ListValue struct {
	ElementType Type
	Elements    []Value
}

func (ListValue) isValue()   {}
func (ListValue) Kind() Kind { return KindList }

// MapEntry is a single key/value entry of a MapValue.
type MapEntry struct {
	Key   Value
	Value Value
}

// MapValue is a key/value map from KeyType to ValueType, holding Entries.
type MapValue struct {
	KeyType   Type
	ValueType Type
	Entries   []MapEntry
}

func (MapValue) isValue()   {}
func (MapValue) Kind() Kind { return KindMap }

// Output is one client-facing occurrence the Session lifecycle workflow
// fans out to its own caller. Output is a closed interface for the same
// reason Value is.
type Output interface {
	isOutput()
}

// EffectEmitted reports that one instance of the named effect Effect was
// produced for Recipients, with the given Arguments.
type EffectEmitted struct {
	Effect     string
	Recipients []UserUUID
	Arguments  []FieldValue
}

func (EffectEmitted) isOutput() {}

// PresentationActivated reports that the presentation named Name was newly
// mounted for Recipient in the presentation slot Slot, showing the view
// named View with the visible model Model.
type PresentationActivated struct {
	Slot      string
	Recipient UserUUID
	Name      string
	View      string
	Model     Value
}

func (PresentationActivated) isOutput() {}

// PresentationUpdated reports that the presentation named Name, already
// active for Recipient in the presentation slot Slot, now has the visible
// model Model.
type PresentationUpdated struct {
	Slot      string
	Recipient UserUUID
	Name      string
	Model     Value
}

func (PresentationUpdated) isOutput() {}

// PresentationRemoved reports that the presentation named Name, previously
// active for Recipient in the presentation slot Slot, was unmounted.
type PresentationRemoved struct {
	Slot      string
	Recipient UserUUID
	Name      string
}

func (PresentationRemoved) isOutput() {}
