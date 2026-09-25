package sessionlifecycle

import (
	"context"
	"fmt"
	"strconv"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	internalrepo "github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/repo"
	"gorm.io/gorm"
)

// outputsRepoAPI is this file's own narrow persistence contract, shared by
// Start and AnswerInteraction (both call mapOutputs) to resolve Output
// recipients' internal actor ids to their UserUUID.
type outputsRepoAPI interface {
	FindActorsByIDs(ctx context.Context, tx *gorm.DB, sessionID uint, ids []uint) ([]internalrepo.Actor, error)
}

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

// Type is sessionlifecycle's own mirror of engine.Type: the shape a Value
// conforms to, exposed so a consumer can render/act on a value's shape even
// without an instance to inspect (an empty ListValue's element shape, and so
// on). Type is a closed interface, the same way engine.Type is: its marker
// method is unexported so packages outside sessionlifecycle cannot introduce
// unsupported variants.
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

// Value is sessionlifecycle's own mirror of engine.Value: every kind of
// value a Presentation's Model or an Effect/Question's Arguments can carry,
// exposed so a consumer never needs to import game/language/v1/engine to
// read one. Value is a closed interface, the same way engine.Value is: its
// marker method is unexported so packages outside sessionlifecycle cannot
// introduce unsupported variants.
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

// UserValue is a real user, resolved to their public UserUUID - never the
// internal actor id engine.UserID carries.
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

// Output is one client-facing occurrence sessionlifecycle fans out to its
// own caller - sessionlifecycle's own mirror of the 4 engine Output kinds
// internal/clientoutputs.ClientFacing already selects (EmitEffectOutput,
// ActivatePresentationOutput, UpdatePresentationOutput,
// RemovePresentationOutput). Output is a closed interface for the same
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

// mapOutputs translates outputs (already filtered to the client-facing
// subset by internal/clientoutputs.ClientFacing) into sessionlifecycle's own
// Output schema, resolving every recipient's internal actor id (carried by
// engine.UserID) into its real UserUUID via one batched repository lookup.
func (m *Manager) mapOutputs(ctx context.Context, tx *gorm.DB, sessionID uint, outputs []engine.Output) ([]Output, error) {
	actorIDs, err := collectActorIDs(outputs)
	if err != nil {
		return nil, err
	}
	actors, err := m.outputsRepo.FindActorsByIDs(ctx, tx, sessionID, actorIDs)
	if err != nil {
		return nil, fmt.Errorf("resolving output recipients: %s", err)
	}
	userUUIDByActorID := make(map[uint]UserUUID, len(actors))
	for _, a := range actors {
		userUUIDByActorID[a.ID] = UserUUID(a.UserUUID)
	}
	resolve := func(id engine.UserID) (UserUUID, error) {
		actorID, err := parseActorID(id)
		if err != nil {
			return "", err
		}
		userUUID, ok := userUUIDByActorID[actorID]
		if !ok {
			return "", fmt.Errorf("output recipient actor id %d not found in session %d", actorID, sessionID)
		}
		return userUUID, nil
	}

	// var, not make([]Output, 0, ...): stays nil when there is nothing to
	// map, the same way internal/clientoutputs.ClientFacing already reports
	// "no client-facing output" as nil rather than an empty slice.
	var mapped []Output
	for _, o := range outputs {
		switch v := o.(type) {
		case engine.EmitEffectOutput:
			recipients := make([]UserUUID, len(v.Recipients))
			for i, r := range v.Recipients {
				recipients[i], err = resolve(r)
				if err != nil {
					return nil, err
				}
			}
			arguments, err := mapFieldValues(v.Arguments, resolve)
			if err != nil {
				return nil, err
			}
			mapped = append(mapped, EffectEmitted{Effect: v.Effect, Recipients: recipients, Arguments: arguments})
		case engine.ActivatePresentationOutput:
			recipient, err := resolve(v.Recipient)
			if err != nil {
				return nil, err
			}
			model, err := mapValue(v.Model, resolve)
			if err != nil {
				return nil, err
			}
			mapped = append(mapped, PresentationActivated{Slot: v.Slot, Recipient: recipient, Name: v.Name, View: v.View, Model: model})
		case engine.UpdatePresentationOutput:
			recipient, err := resolve(v.Recipient)
			if err != nil {
				return nil, err
			}
			model, err := mapValue(v.Model, resolve)
			if err != nil {
				return nil, err
			}
			mapped = append(mapped, PresentationUpdated{Slot: v.Slot, Recipient: recipient, Name: v.Name, Model: model})
		case engine.RemovePresentationOutput:
			recipient, err := resolve(v.Recipient)
			if err != nil {
				return nil, err
			}
			mapped = append(mapped, PresentationRemoved{Slot: v.Slot, Recipient: recipient, Name: v.Name})
		default:
			return nil, fmt.Errorf("mapping client-facing output: unexpected type %T", o)
		}
	}
	return mapped, nil
}

// parseActorID recovers the internal session_actors.id an engine.UserID
// carries - see step_start.go's engine.UserID(strconv.FormatUint(...)),
// the only place a UserID is ever constructed.
func parseActorID(id engine.UserID) (uint, error) {
	parsed, err := strconv.ParseUint(string(id), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing output recipient actor id %q: %s", string(id), err)
	}
	return uint(parsed), nil
}

// collectActorIDs returns every distinct internal actor id referenced
// anywhere in outputs - as a direct Recipient/Recipients, or nested inside a
// Model/Arguments' own UserValue - so mapOutputs can resolve all of them in
// one batched repository lookup instead of one query per recipient.
func collectActorIDs(outputs []engine.Output) ([]uint, error) {
	seen := make(map[uint]struct{})
	var ids []uint
	add := func(id engine.UserID) error {
		actorID, err := parseActorID(id)
		if err != nil {
			return err
		}
		if _, ok := seen[actorID]; !ok {
			seen[actorID] = struct{}{}
			ids = append(ids, actorID)
		}
		return nil
	}
	for _, o := range outputs {
		switch v := o.(type) {
		case engine.EmitEffectOutput:
			for _, r := range v.Recipients {
				if err := add(r); err != nil {
					return nil, err
				}
			}
			if err := walkFieldValueActorIDs(v.Arguments, add); err != nil {
				return nil, err
			}
		case engine.ActivatePresentationOutput:
			if err := add(v.Recipient); err != nil {
				return nil, err
			}
			if err := walkValueActorIDs(v.Model, add); err != nil {
				return nil, err
			}
		case engine.UpdatePresentationOutput:
			if err := add(v.Recipient); err != nil {
				return nil, err
			}
			if err := walkValueActorIDs(v.Model, add); err != nil {
				return nil, err
			}
		case engine.RemovePresentationOutput:
			if err := add(v.Recipient); err != nil {
				return nil, err
			}
		}
	}
	return ids, nil
}

// walkValueActorIDs recursively visits every engine.UserValue reachable from
// v, calling add with its ID.
func walkValueActorIDs(v engine.Value, add func(engine.UserID) error) error {
	switch vv := v.(type) {
	case engine.UserValue:
		return add(vv.ID)
	case engine.RecordValue:
		return walkFieldValueActorIDs(vv.Fields, add)
	case engine.UnionValue:
		return walkFieldValueActorIDs(vv.Fields, add)
	case engine.NewTypeValue:
		return walkValueActorIDs(vv.Underlying, add)
	case engine.OptionalValue:
		if vv.Value == nil {
			return nil
		}
		return walkValueActorIDs(vv.Value, add)
	case engine.ListValue:
		for _, e := range vv.Elements {
			if err := walkValueActorIDs(e, add); err != nil {
				return err
			}
		}
	case engine.MapValue:
		for _, e := range vv.Entries {
			if err := walkValueActorIDs(e.Key, add); err != nil {
				return err
			}
			if err := walkValueActorIDs(e.Value, add); err != nil {
				return err
			}
		}
	}
	return nil
}

func walkFieldValueActorIDs(fields []engine.FieldValue, add func(engine.UserID) error) error {
	for _, f := range fields {
		if err := walkValueActorIDs(f.Value, add); err != nil {
			return err
		}
	}
	return nil
}

// mapValue translates v into sessionlifecycle's own Value schema, resolving
// any nested engine.UserValue via resolve.
func mapValue(v engine.Value, resolve func(engine.UserID) (UserUUID, error)) (Value, error) {
	switch vv := v.(type) {
	case engine.UnitValue:
		return UnitValue{}, nil
	case engine.BoolValue:
		return BoolValue{Value: vv.Value}, nil
	case engine.NumberValue:
		return NumberValue{Value: vv.Value}, nil
	case engine.StringValue:
		return StringValue{Value: vv.Value}, nil
	case engine.UserValue:
		userUUID, err := resolve(vv.ID)
		if err != nil {
			return nil, err
		}
		return UserValue{UserUUID: userUUID}, nil
	case engine.EnumValue:
		return EnumValue{TypeName: vv.TypeName, ValueName: vv.ValueName}, nil
	case engine.RecordValue:
		fields, err := mapFieldValues(vv.Fields, resolve)
		if err != nil {
			return nil, err
		}
		return RecordValue{TypeName: vv.TypeName, Fields: fields}, nil
	case engine.UnionValue:
		fields, err := mapFieldValues(vv.Fields, resolve)
		if err != nil {
			return nil, err
		}
		return UnionValue{TypeName: vv.TypeName, VariantName: vv.VariantName, Fields: fields}, nil
	case engine.NewTypeValue:
		underlying, err := mapValue(vv.Underlying, resolve)
		if err != nil {
			return nil, err
		}
		return NewTypeValue{TypeName: vv.TypeName, Underlying: underlying}, nil
	case engine.OptionalValue:
		elementType, err := mapType(vv.ElementType)
		if err != nil {
			return nil, err
		}
		if vv.Value == nil {
			return OptionalValue{ElementType: elementType}, nil
		}
		inner, err := mapValue(vv.Value, resolve)
		if err != nil {
			return nil, err
		}
		return OptionalValue{ElementType: elementType, Value: inner}, nil
	case engine.ListValue:
		elementType, err := mapType(vv.ElementType)
		if err != nil {
			return nil, err
		}
		elements := make([]Value, len(vv.Elements))
		for i, e := range vv.Elements {
			mapped, err := mapValue(e, resolve)
			if err != nil {
				return nil, err
			}
			elements[i] = mapped
		}
		return ListValue{ElementType: elementType, Elements: elements}, nil
	case engine.MapValue:
		keyType, err := mapType(vv.KeyType)
		if err != nil {
			return nil, err
		}
		valueType, err := mapType(vv.ValueType)
		if err != nil {
			return nil, err
		}
		entries := make([]MapEntry, len(vv.Entries))
		for i, e := range vv.Entries {
			key, err := mapValue(e.Key, resolve)
			if err != nil {
				return nil, err
			}
			value, err := mapValue(e.Value, resolve)
			if err != nil {
				return nil, err
			}
			entries[i] = MapEntry{Key: key, Value: value}
		}
		return MapValue{KeyType: keyType, ValueType: valueType, Entries: entries}, nil
	default:
		return nil, fmt.Errorf("mapping value: unexpected type %T", v)
	}
}

func mapFieldValues(fields []engine.FieldValue, resolve func(engine.UserID) (UserUUID, error)) ([]FieldValue, error) {
	mapped := make([]FieldValue, len(fields))
	for i, f := range fields {
		value, err := mapValue(f.Value, resolve)
		if err != nil {
			return nil, err
		}
		mapped[i] = FieldValue{Name: f.Name, Value: value}
	}
	return mapped, nil
}

// mapType translates t into sessionlifecycle's own Type schema.
func mapType(t engine.Type) (Type, error) {
	switch tt := t.(type) {
	case engine.UnitType:
		return UnitType{}, nil
	case engine.BoolType:
		return BoolType{}, nil
	case engine.NumberType:
		return NumberType{}, nil
	case engine.StringType:
		return StringType{}, nil
	case engine.UserType:
		return UserType{}, nil
	case engine.EnumType:
		return EnumType{Name: tt.Name, Values: append([]string(nil), tt.Values...)}, nil
	case engine.RecordType:
		fields, err := mapFieldTypes(tt.Fields)
		if err != nil {
			return nil, err
		}
		return RecordType{Name: tt.Name, Fields: fields}, nil
	case engine.UnionType:
		variants := make([]UnionVariantType, len(tt.Variants))
		for i, variant := range tt.Variants {
			fields, err := mapFieldTypes(variant.Fields)
			if err != nil {
				return nil, err
			}
			variants[i] = UnionVariantType{Name: variant.Name, Fields: fields}
		}
		return UnionType{Name: tt.Name, Variants: variants}, nil
	case engine.NewType:
		underlying, err := mapType(tt.Underlying)
		if err != nil {
			return nil, err
		}
		return NewType{Name: tt.Name, Underlying: underlying}, nil
	case engine.OptionalType:
		element, err := mapType(tt.Element)
		if err != nil {
			return nil, err
		}
		return OptionalType{Element: element}, nil
	case engine.ListType:
		element, err := mapType(tt.Element)
		if err != nil {
			return nil, err
		}
		return ListType{Element: element}, nil
	case engine.MapType:
		key, err := mapType(tt.Key)
		if err != nil {
			return nil, err
		}
		value, err := mapType(tt.Value)
		if err != nil {
			return nil, err
		}
		return MapType{Key: key, Value: value}, nil
	default:
		return nil, fmt.Errorf("mapping type: unexpected type %T", t)
	}
}

func mapFieldTypes(fields []engine.FieldType) ([]FieldType, error) {
	mapped := make([]FieldType, len(fields))
	for i, f := range fields {
		t, err := mapType(f.Type)
		if err != nil {
			return nil, err
		}
		mapped[i] = FieldType{Name: f.Name, Type: t}
	}
	return mapped, nil
}
