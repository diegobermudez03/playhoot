package sessionlifecycle

import (
	"context"
	"fmt"
	"strconv"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/session"
	internalrepo "github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/repo"
	"gorm.io/gorm"
)

// outputsRepoAPI is this file's own narrow persistence contract, shared by
// Start and AnswerInteraction (both call mapOutputs) to resolve Output
// recipients' internal actor ids to their UserUUID.
type outputsRepoAPI interface {
	FindActorsByIDs(ctx context.Context, tx *gorm.DB, sessionID uint, ids []uint) ([]internalrepo.Actor, error)
}

// mapOutputs translates outputs (already filtered to the client-facing
// subset by internal/clientoutputs.ClientFacing) into game/session's own
// Output schema, resolving every recipient's internal actor id (carried by
// engine.UserID) into its real UserUUID via one batched repository lookup.
func (m *Manager) mapOutputs(ctx context.Context, tx *gorm.DB, sessionID uint, outputs []engine.Output) ([]session.Output, error) {
	actorIDs, err := collectActorIDs(outputs)
	if err != nil {
		return nil, err
	}
	actors, err := m.outputsRepo.FindActorsByIDs(ctx, tx, sessionID, actorIDs)
	if err != nil {
		return nil, fmt.Errorf("resolving output recipients: %s", err)
	}
	userUUIDByActorID := make(map[uint]session.UserUUID, len(actors))
	for _, a := range actors {
		userUUIDByActorID[a.ID] = session.UserUUID(a.UserUUID)
	}
	resolve := func(id engine.UserID) (session.UserUUID, error) {
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

	// var, not make([]session.Output, 0, ...): stays nil when there is
	// nothing to map, the same way internal/clientoutputs.ClientFacing
	// already reports "no client-facing output" as nil rather than an empty
	// slice.
	var mapped []session.Output
	for _, o := range outputs {
		switch v := o.(type) {
		case engine.EmitEffectOutput:
			recipients := make([]session.UserUUID, len(v.Recipients))
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
			mapped = append(mapped, session.EffectEmitted{Effect: v.Effect, Recipients: recipients, Arguments: arguments})
		case engine.ActivatePresentationOutput:
			recipient, err := resolve(v.Recipient)
			if err != nil {
				return nil, err
			}
			model, err := mapValue(v.Model, resolve)
			if err != nil {
				return nil, err
			}
			mapped = append(mapped, session.PresentationActivated{Slot: v.Slot, Recipient: recipient, Name: v.Name, View: v.View, Model: model})
		case engine.UpdatePresentationOutput:
			recipient, err := resolve(v.Recipient)
			if err != nil {
				return nil, err
			}
			model, err := mapValue(v.Model, resolve)
			if err != nil {
				return nil, err
			}
			mapped = append(mapped, session.PresentationUpdated{Slot: v.Slot, Recipient: recipient, Name: v.Name, Model: model})
		case engine.RemovePresentationOutput:
			recipient, err := resolve(v.Recipient)
			if err != nil {
				return nil, err
			}
			mapped = append(mapped, session.PresentationRemoved{Slot: v.Slot, Recipient: recipient, Name: v.Name})
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

// mapValue translates v into game/session's own Value schema, resolving
// any nested engine.UserValue via resolve.
func mapValue(v engine.Value, resolve func(engine.UserID) (session.UserUUID, error)) (session.Value, error) {
	switch vv := v.(type) {
	case engine.UnitValue:
		return session.UnitValue{}, nil
	case engine.BoolValue:
		return session.BoolValue{Value: vv.Value}, nil
	case engine.NumberValue:
		return session.NumberValue{Value: vv.Value}, nil
	case engine.StringValue:
		return session.StringValue{Value: vv.Value}, nil
	case engine.UserValue:
		userUUID, err := resolve(vv.ID)
		if err != nil {
			return nil, err
		}
		return session.UserValue{UserUUID: userUUID}, nil
	case engine.EnumValue:
		return session.EnumValue{TypeName: vv.TypeName, ValueName: vv.ValueName}, nil
	case engine.RecordValue:
		fields, err := mapFieldValues(vv.Fields, resolve)
		if err != nil {
			return nil, err
		}
		return session.RecordValue{TypeName: vv.TypeName, Fields: fields}, nil
	case engine.UnionValue:
		fields, err := mapFieldValues(vv.Fields, resolve)
		if err != nil {
			return nil, err
		}
		return session.UnionValue{TypeName: vv.TypeName, VariantName: vv.VariantName, Fields: fields}, nil
	case engine.NewTypeValue:
		underlying, err := mapValue(vv.Underlying, resolve)
		if err != nil {
			return nil, err
		}
		return session.NewTypeValue{TypeName: vv.TypeName, Underlying: underlying}, nil
	case engine.OptionalValue:
		elementType, err := mapType(vv.ElementType)
		if err != nil {
			return nil, err
		}
		if vv.Value == nil {
			return session.OptionalValue{ElementType: elementType}, nil
		}
		inner, err := mapValue(vv.Value, resolve)
		if err != nil {
			return nil, err
		}
		return session.OptionalValue{ElementType: elementType, Value: inner}, nil
	case engine.ListValue:
		elementType, err := mapType(vv.ElementType)
		if err != nil {
			return nil, err
		}
		elements := make([]session.Value, len(vv.Elements))
		for i, e := range vv.Elements {
			mapped, err := mapValue(e, resolve)
			if err != nil {
				return nil, err
			}
			elements[i] = mapped
		}
		return session.ListValue{ElementType: elementType, Elements: elements}, nil
	case engine.MapValue:
		keyType, err := mapType(vv.KeyType)
		if err != nil {
			return nil, err
		}
		valueType, err := mapType(vv.ValueType)
		if err != nil {
			return nil, err
		}
		entries := make([]session.MapEntry, len(vv.Entries))
		for i, e := range vv.Entries {
			key, err := mapValue(e.Key, resolve)
			if err != nil {
				return nil, err
			}
			value, err := mapValue(e.Value, resolve)
			if err != nil {
				return nil, err
			}
			entries[i] = session.MapEntry{Key: key, Value: value}
		}
		return session.MapValue{KeyType: keyType, ValueType: valueType, Entries: entries}, nil
	default:
		return nil, fmt.Errorf("mapping value: unexpected type %T", v)
	}
}

func mapFieldValues(fields []engine.FieldValue, resolve func(engine.UserID) (session.UserUUID, error)) ([]session.FieldValue, error) {
	mapped := make([]session.FieldValue, len(fields))
	for i, f := range fields {
		value, err := mapValue(f.Value, resolve)
		if err != nil {
			return nil, err
		}
		mapped[i] = session.FieldValue{Name: f.Name, Value: value}
	}
	return mapped, nil
}

// mapType translates t into game/session's own Type schema.
func mapType(t engine.Type) (session.Type, error) {
	switch tt := t.(type) {
	case engine.UnitType:
		return session.UnitType{}, nil
	case engine.BoolType:
		return session.BoolType{}, nil
	case engine.NumberType:
		return session.NumberType{}, nil
	case engine.StringType:
		return session.StringType{}, nil
	case engine.UserType:
		return session.UserType{}, nil
	case engine.EnumType:
		return session.EnumType{Name: tt.Name, Values: append([]string(nil), tt.Values...)}, nil
	case engine.RecordType:
		fields, err := mapFieldTypes(tt.Fields)
		if err != nil {
			return nil, err
		}
		return session.RecordType{Name: tt.Name, Fields: fields}, nil
	case engine.UnionType:
		variants := make([]session.UnionVariantType, len(tt.Variants))
		for i, variant := range tt.Variants {
			fields, err := mapFieldTypes(variant.Fields)
			if err != nil {
				return nil, err
			}
			variants[i] = session.UnionVariantType{Name: variant.Name, Fields: fields}
		}
		return session.UnionType{Name: tt.Name, Variants: variants}, nil
	case engine.NewType:
		underlying, err := mapType(tt.Underlying)
		if err != nil {
			return nil, err
		}
		return session.NewType{Name: tt.Name, Underlying: underlying}, nil
	case engine.OptionalType:
		element, err := mapType(tt.Element)
		if err != nil {
			return nil, err
		}
		return session.OptionalType{Element: element}, nil
	case engine.ListType:
		element, err := mapType(tt.Element)
		if err != nil {
			return nil, err
		}
		return session.ListType{Element: element}, nil
	case engine.MapType:
		key, err := mapType(tt.Key)
		if err != nil {
			return nil, err
		}
		value, err := mapType(tt.Value)
		if err != nil {
			return nil, err
		}
		return session.MapType{Key: key, Value: value}, nil
	default:
		return nil, fmt.Errorf("mapping type: unexpected type %T", t)
	}
}

func mapFieldTypes(fields []engine.FieldType) ([]session.FieldType, error) {
	mapped := make([]session.FieldType, len(fields))
	for i, f := range fields {
		t, err := mapType(f.Type)
		if err != nil {
			return nil, err
		}
		mapped[i] = session.FieldType{Name: f.Name, Type: t}
	}
	return mapped, nil
}
