package engine

// UserID identifies a real user connected to a room, wherever a Value
// needs to refer to one. This package does not define how a UserID is
// assigned, authenticated, or connected to a room; that belongs to a
// future application layer. A UserID is only ever compared for
// equality.
type UserID string

// Value is the engine's runtime representation of one value of the
// language's type system: every kind program can express — unit, bool,
// number, string, user, enum, record, union, new type, optional, list,
// and map — has exactly one corresponding Value variant below.
//
// A Value is plain data: it carries no mutable state of its own, is safe
// to store inside a Snapshot, to copy, to compare, and to share across
// concurrent reads. Callers must not mutate a Value's slice fields
// (Fields, Elements, Entries) after constructing it — the same
// convention program itself uses for its own declaration slices.
//
// Value is a closed interface, mirroring program's own closed-interface
// pattern: its marker method is unexported so packages outside engine
// cannot introduce unsupported variants, and a type switch inside this
// package can safely be treated as exhaustive.
type Value interface {
	isValue()

	// Kind identifies which of Value's variants this is, without
	// requiring a type switch.
	Kind() Kind

	// Equal reports whether other is a deterministically identical
	// value: the same Kind, the same nominal identity where one
	// applies, and, for compound values, recursively equal
	// elements/fields.
	Equal(other Value) bool

	// Validate reports whether this value's shape conforms to t: the
	// same Kind, the same nominal identity where one applies, and, for
	// compound values, every element/field individually valid against
	// its corresponding element/field Type in t.
	//
	// Validate never mutates this value or t. A false result means only
	// that this value does not conform to t; it says nothing about
	// whether t itself is otherwise well-formed.
	Validate(t Type) bool
}

// UnitValue is the single value of the unit type.
type UnitValue struct{}

func (UnitValue) isValue()   {}
func (UnitValue) Kind() Kind { return KindUnit }
func (UnitValue) Equal(other Value) bool {
	_, ok := other.(UnitValue)
	return ok
}
func (UnitValue) Validate(t Type) bool {
	_, ok := t.(UnitType)
	return ok
}

// BoolValue is a boolean value.
type BoolValue struct {
	Value bool
}

func (BoolValue) isValue()   {}
func (BoolValue) Kind() Kind { return KindBool }
func (v BoolValue) Equal(other Value) bool {
	o, ok := other.(BoolValue)
	return ok && v.Value == o.Value
}
func (BoolValue) Validate(t Type) bool {
	_, ok := t.(BoolType)
	return ok
}

// NumberValue is a numeric value.
//
// Value is a 64-bit floating-point number. This package does not model
// the language's "number" as a distinct integer/float pair — a future
// compiler and evaluator are responsible for any additional constraint a
// specific use requires (for example, requiring an integer number of
// milliseconds for a timer delay). Equality compares Value with the
// ordinary Go == operator, so two NumberValues built from the same
// IEEE-754 bit pattern always compare equal, and this comparison is
// deterministic across any Go-compliant runtime.
type NumberValue struct {
	Value float64
}

func (NumberValue) isValue()   {}
func (NumberValue) Kind() Kind { return KindNumber }
func (v NumberValue) Equal(other Value) bool {
	o, ok := other.(NumberValue)
	return ok && v.Value == o.Value
}
func (NumberValue) Validate(t Type) bool {
	_, ok := t.(NumberType)
	return ok
}

// StringValue is a text value.
type StringValue struct {
	Value string
}

func (StringValue) isValue()   {}
func (StringValue) Kind() Kind { return KindString }
func (v StringValue) Equal(other Value) bool {
	o, ok := other.(StringValue)
	return ok && v.Value == o.Value
}
func (StringValue) Validate(t Type) bool {
	_, ok := t.(StringType)
	return ok
}

// UserValue is a real user connected to a room (see
// program.BuiltinTypeUser).
type UserValue struct {
	ID UserID
}

func (UserValue) isValue()   {}
func (UserValue) Kind() Kind { return KindUser }
func (v UserValue) Equal(other Value) bool {
	o, ok := other.(UserValue)
	return ok && v.ID == o.ID
}
func (UserValue) Validate(t Type) bool {
	_, ok := t.(UserType)
	return ok
}

// EnumValue is exactly one symbolic value, ValueName, of the named enum
// type TypeName.
//
// TypeName carries this value's nominal identity: an EnumValue is only
// ever Equal to another EnumValue with the same TypeName, even when
// ValueName also matches a value of a different, structurally identical
// enum type.
type EnumValue struct {
	TypeName  string
	ValueName string
}

func (EnumValue) isValue()   {}
func (EnumValue) Kind() Kind { return KindEnum }
func (v EnumValue) Equal(other Value) bool {
	o, ok := other.(EnumValue)
	return ok && v.TypeName == o.TypeName && v.ValueName == o.ValueName
}
func (v EnumValue) Validate(t Type) bool {
	et, ok := t.(EnumType)
	if !ok || et.Name != v.TypeName {
		return false
	}
	return et.HasValue(v.ValueName)
}

// FieldValue assigns a value to a named field within a RecordValue or a
// union variant's Fields.
type FieldValue struct {
	Name  string
	Value Value
}

// RecordValue is a value of the named record type TypeName, containing
// exactly Fields — no field of the declared type may be missing, and no
// extra field may be present.
//
// TypeName carries this value's nominal identity, the same way it does
// for EnumValue.
type RecordValue struct {
	TypeName string
	Fields   []FieldValue
}

func (RecordValue) isValue()   {}
func (RecordValue) Kind() Kind { return KindRecord }
func (v RecordValue) Equal(other Value) bool {
	o, ok := other.(RecordValue)
	if !ok || v.TypeName != o.TypeName || len(v.Fields) != len(o.Fields) {
		return false
	}
	for _, f := range v.Fields {
		of, ok := fieldValueByName(o.Fields, f.Name)
		if !ok || !f.Value.Equal(of.Value) {
			return false
		}
	}
	return true
}

// FieldByName returns v's field named name, if any.
func (v RecordValue) FieldByName(name string) (FieldValue, bool) {
	return fieldValueByName(v.Fields, name)
}

func (v RecordValue) Validate(t Type) bool {
	rt, ok := t.(RecordType)
	if !ok || rt.Name != v.TypeName || len(v.Fields) != len(rt.Fields) {
		return false
	}
	for _, f := range v.Fields {
		ft, ok := rt.FieldByName(f.Name)
		if !ok || !f.Value.Validate(ft.Type) {
			return false
		}
	}
	return true
}

// UnionValue is a value of the named union type TypeName, selecting the
// single variant VariantName and, if that variant declares fields,
// providing them via Fields. A zero-field variant is represented with
// an empty Fields slice.
//
// TypeName carries this value's nominal identity, the same way it does
// for EnumValue.
type UnionValue struct {
	TypeName    string
	VariantName string
	Fields      []FieldValue
}

func (UnionValue) isValue()   {}
func (UnionValue) Kind() Kind { return KindUnion }
func (v UnionValue) Equal(other Value) bool {
	o, ok := other.(UnionValue)
	if !ok || v.TypeName != o.TypeName || v.VariantName != o.VariantName || len(v.Fields) != len(o.Fields) {
		return false
	}
	for _, f := range v.Fields {
		of, ok := fieldValueByName(o.Fields, f.Name)
		if !ok || !f.Value.Equal(of.Value) {
			return false
		}
	}
	return true
}

// FieldByName returns v's field named name, if any.
func (v UnionValue) FieldByName(name string) (FieldValue, bool) {
	return fieldValueByName(v.Fields, name)
}

func (v UnionValue) Validate(t Type) bool {
	ut, ok := t.(UnionType)
	if !ok || ut.Name != v.TypeName {
		return false
	}
	variant, ok := ut.VariantByName(v.VariantName)
	if !ok || len(v.Fields) != len(variant.Fields) {
		return false
	}
	for _, f := range v.Fields {
		ft, ok := variant.FieldByName(f.Name)
		if !ok || !f.Value.Validate(ft.Type) {
			return false
		}
	}
	return true
}

// NewTypeValue is a value of the named new type TypeName, wrapping
// Underlying.
//
// TypeName carries this value's nominal identity: a NewTypeValue is
// never Equal to its own Underlying value directly, nor to a
// NewTypeValue of a different TypeName wrapping an equal Underlying
// value — new types built on the same underlying type remain distinct
// from one another, per program.NewTypeDeclaration.
type NewTypeValue struct {
	TypeName   string
	Underlying Value
}

func (NewTypeValue) isValue()   {}
func (NewTypeValue) Kind() Kind { return KindNewType }
func (v NewTypeValue) Equal(other Value) bool {
	o, ok := other.(NewTypeValue)
	return ok && v.TypeName == o.TypeName && v.Underlying.Equal(o.Underlying)
}
func (v NewTypeValue) Validate(t Type) bool {
	nt, ok := t.(NewType)
	if !ok || nt.Name != v.TypeName {
		return false
	}
	return v.Underlying.Validate(nt.Underlying)
}

// OptionalValue is either a wrapped Value of ElementType or its
// absence. Value is nil exactly when this OptionalValue represents
// absence; this package uses nil, rather than a separate boolean field,
// as the single source of truth for presence.
//
// ElementType makes an absent OptionalValue's element type inspectable
// without a wrapped value to inspect, the same way
// program.OptionalNoneExpression's ElementType does at the source
// level. Validate does not compare ElementType against the Type it is
// given; ElementType is informational only.
type OptionalValue struct {
	ElementType Type
	Value       Value
}

func (OptionalValue) isValue()   {}
func (OptionalValue) Kind() Kind { return KindOptional }
func (v OptionalValue) Equal(other Value) bool {
	o, ok := other.(OptionalValue)
	if !ok {
		return false
	}
	if v.Value == nil || o.Value == nil {
		return v.Value == nil && o.Value == nil
	}
	return v.Value.Equal(o.Value)
}
func (v OptionalValue) Validate(t Type) bool {
	ot, ok := t.(OptionalType)
	if !ok {
		return false
	}
	if v.Value == nil {
		return true
	}
	return v.Value.Validate(ot.Element)
}

// ListValue is an ordered list of Elements, each of type ElementType.
//
// ElementType makes an empty ListValue's element type inspectable
// without any element to inspect. Validate does not compare ElementType
// against the Type it is given; ElementType is informational only.
type ListValue struct {
	ElementType Type
	Elements    []Value
}

func (ListValue) isValue()   {}
func (ListValue) Kind() Kind { return KindList }
func (v ListValue) Equal(other Value) bool {
	o, ok := other.(ListValue)
	if !ok || len(v.Elements) != len(o.Elements) {
		return false
	}
	for i, e := range v.Elements {
		if !e.Equal(o.Elements[i]) {
			return false
		}
	}
	return true
}
func (v ListValue) Validate(t Type) bool {
	lt, ok := t.(ListType)
	if !ok {
		return false
	}
	for _, e := range v.Elements {
		if !e.Validate(lt.Element) {
			return false
		}
	}
	return true
}

// MapEntry is a single key/value entry of a MapValue.
type MapEntry struct {
	Key   Value
	Value Value
}

// MapValue is a key/value map from KeyType to ValueType, holding
// Entries.
//
// Entries is a slice, not a Go map, the same way program.MapExpression
// stores its entries as a slice: Value is an interface that may wrap a
// slice internally, so it is not usable as a Go map key, and a slice
// keeps a MapValue's representation deterministic regardless of how it
// was built. This package does not itself enforce that Entries has no
// duplicate keys; Validate only checks that every key and value
// conforms to KeyType/ValueType. Equal assumes neither operand has
// duplicate keys — its result is unspecified if either does.
type MapValue struct {
	KeyType   Type
	ValueType Type
	Entries   []MapEntry
}

func (MapValue) isValue()   {}
func (MapValue) Kind() Kind { return KindMap }
func (v MapValue) Equal(other Value) bool {
	o, ok := other.(MapValue)
	if !ok || len(v.Entries) != len(o.Entries) {
		return false
	}
	for _, e := range v.Entries {
		ov, ok := mapEntryValueByKey(o.Entries, e.Key)
		if !ok || !e.Value.Equal(ov) {
			return false
		}
	}
	return true
}
func (v MapValue) Validate(t Type) bool {
	mt, ok := t.(MapType)
	if !ok {
		return false
	}
	for _, e := range v.Entries {
		if !e.Key.Validate(mt.Key) || !e.Value.Validate(mt.Value) {
			return false
		}
	}
	return true
}

func fieldValueByName(fields []FieldValue, name string) (FieldValue, bool) {
	for _, f := range fields {
		if f.Name == name {
			return f, true
		}
	}
	return FieldValue{}, false
}

func mapEntryValueByKey(entries []MapEntry, key Value) (Value, bool) {
	for _, e := range entries {
		if e.Key.Equal(key) {
			return e.Value, true
		}
	}
	return nil, false
}
