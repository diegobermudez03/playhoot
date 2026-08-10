package codec

import (
	"encoding/json"
	"fmt"

	"github.com/diegobermudez03/playhoot/game/v1/engine"
)

type valueWire struct {
	Kind string `json:"kind"`

	// Scalars.
	Bool   bool    `json:"bool,omitempty"`
	Number float64 `json:"number,omitempty"`
	String string  `json:"string,omitempty"`
	User   string  `json:"user,omitempty"`

	// EnumValue / RecordValue / UnionValue nominal identity.
	TypeName    string `json:"type_name,omitempty"`
	ValueName   string `json:"value_name,omitempty"`
	VariantName string `json:"variant_name,omitempty"`

	Fields []fieldValueWire `json:"fields,omitempty"`

	// NewTypeValue.
	Underlying json.RawMessage `json:"underlying,omitempty"`

	// OptionalValue / ListValue.
	ElementType json.RawMessage   `json:"element_type,omitempty"`
	Value       json.RawMessage   `json:"value,omitempty"`
	Elements    []json.RawMessage `json:"elements,omitempty"`

	// MapValue.
	KeyType   json.RawMessage `json:"key_type,omitempty"`
	ValueType json.RawMessage `json:"value_type,omitempty"`
	Entries   []mapEntryWire  `json:"entries,omitempty"`
}

type fieldValueWire struct {
	Name  string          `json:"name"`
	Value json.RawMessage `json:"value"`
}

type mapEntryWire struct {
	Key   json.RawMessage `json:"key"`
	Value json.RawMessage `json:"value"`
}

// EncodeValue encodes v, or JSON null if v is nil.
func EncodeValue(path string, v engine.Value) (json.RawMessage, error) {
	if v == nil {
		return json.RawMessage("null"), nil
	}
	switch val := v.(type) {
	case engine.UnitValue:
		return json.Marshal(valueWire{Kind: "unit"})
	case engine.BoolValue:
		return json.Marshal(valueWire{Kind: "bool", Bool: val.Value})
	case engine.NumberValue:
		return json.Marshal(valueWire{Kind: "number", Number: val.Value})
	case engine.StringValue:
		return json.Marshal(valueWire{Kind: "string", String: val.Value})
	case engine.UserValue:
		return json.Marshal(valueWire{Kind: "user", User: string(val.ID)})
	case engine.EnumValue:
		return json.Marshal(valueWire{Kind: "enum", TypeName: val.TypeName, ValueName: val.ValueName})
	case engine.RecordValue:
		fields, err := encodeFieldValues(path, val.Fields)
		if err != nil {
			return nil, err
		}
		return json.Marshal(valueWire{Kind: "record", TypeName: val.TypeName, Fields: fields})
	case engine.UnionValue:
		fields, err := encodeFieldValues(path, val.Fields)
		if err != nil {
			return nil, err
		}
		return json.Marshal(valueWire{Kind: "union", TypeName: val.TypeName, VariantName: val.VariantName, Fields: fields})
	case engine.NewTypeValue:
		underlying, err := EncodeValue(pathField(path, "underlying"), val.Underlying)
		if err != nil {
			return nil, err
		}
		return json.Marshal(valueWire{Kind: "new_type", TypeName: val.TypeName, Underlying: underlying})
	case engine.OptionalValue:
		elementType, err := EncodeType(pathField(path, "element_type"), val.ElementType)
		if err != nil {
			return nil, err
		}
		inner, err := EncodeValue(pathField(path, "value"), val.Value)
		if err != nil {
			return nil, err
		}
		return json.Marshal(valueWire{Kind: "optional", ElementType: elementType, Value: inner})
	case engine.ListValue:
		elementType, err := EncodeType(pathField(path, "element_type"), val.ElementType)
		if err != nil {
			return nil, err
		}
		elements := make([]json.RawMessage, len(val.Elements))
		for i, e := range val.Elements {
			raw, err := EncodeValue(pathIndex(pathField(path, "elements"), i), e)
			if err != nil {
				return nil, err
			}
			elements[i] = raw
		}
		return json.Marshal(valueWire{Kind: "list", ElementType: elementType, Elements: elements})
	case engine.MapValue:
		keyType, err := EncodeType(pathField(path, "key_type"), val.KeyType)
		if err != nil {
			return nil, err
		}
		valueType, err := EncodeType(pathField(path, "value_type"), val.ValueType)
		if err != nil {
			return nil, err
		}
		entries := make([]mapEntryWire, len(val.Entries))
		for i, e := range val.Entries {
			epath := pathIndex(pathField(path, "entries"), i)
			k, err := EncodeValue(pathField(epath, "key"), e.Key)
			if err != nil {
				return nil, err
			}
			v, err := EncodeValue(pathField(epath, "value"), e.Value)
			if err != nil {
				return nil, err
			}
			entries[i] = mapEntryWire{Key: k, Value: v}
		}
		return json.Marshal(valueWire{Kind: "map", KeyType: keyType, ValueType: valueType, Entries: entries})
	default:
		return nil, newDecodeError(path, fmt.Sprintf("unsupported engine.Value %T", v), nil)
	}
}

func encodeFieldValues(path string, fields []engine.FieldValue) ([]fieldValueWire, error) {
	result := make([]fieldValueWire, len(fields))
	for i, f := range fields {
		raw, err := EncodeValue(pathIndex(pathField(path, "fields"), i), f.Value)
		if err != nil {
			return nil, err
		}
		result[i] = fieldValueWire{Name: f.Name, Value: raw}
	}
	return result, nil
}

// DecodeValue decodes data into an engine.Value, or nil for JSON null.
func DecodeValue(path string, data json.RawMessage) (engine.Value, error) {
	if isEmptyOrNull(data) {
		return nil, nil
	}
	return decodeUnion(path, data, decodeValueDispatch)
}

func decodeValueDispatch(path, kind string, raw json.RawMessage) (engine.Value, error) {
	var w valueWire
	if err := strictDecodeInto(path, raw, &w); err != nil {
		return nil, err
	}
	switch kind {
	case "unit":
		return engine.UnitValue{}, nil
	case "bool":
		return engine.BoolValue{Value: w.Bool}, nil
	case "number":
		return engine.NumberValue{Value: w.Number}, nil
	case "string":
		return engine.StringValue{Value: w.String}, nil
	case "user":
		return engine.UserValue{ID: engine.UserID(w.User)}, nil
	case "enum":
		return engine.EnumValue{TypeName: w.TypeName, ValueName: w.ValueName}, nil
	case "record":
		fields, err := decodeFieldValues(path, w.Fields)
		if err != nil {
			return nil, err
		}
		return engine.RecordValue{TypeName: w.TypeName, Fields: fields}, nil
	case "union":
		fields, err := decodeFieldValues(path, w.Fields)
		if err != nil {
			return nil, err
		}
		return engine.UnionValue{TypeName: w.TypeName, VariantName: w.VariantName, Fields: fields}, nil
	case "new_type":
		underlying, err := DecodeValue(pathField(path, "underlying"), w.Underlying)
		if err != nil {
			return nil, err
		}
		return engine.NewTypeValue{TypeName: w.TypeName, Underlying: underlying}, nil
	case "optional":
		elementType, err := DecodeType(pathField(path, "element_type"), w.ElementType)
		if err != nil {
			return nil, err
		}
		inner, err := DecodeValue(pathField(path, "value"), w.Value)
		if err != nil {
			return nil, err
		}
		return engine.OptionalValue{ElementType: elementType, Value: inner}, nil
	case "list":
		elementType, err := DecodeType(pathField(path, "element_type"), w.ElementType)
		if err != nil {
			return nil, err
		}
		elements := make([]engine.Value, len(w.Elements))
		for i, raw := range w.Elements {
			v, err := DecodeValue(pathIndex(pathField(path, "elements"), i), raw)
			if err != nil {
				return nil, err
			}
			elements[i] = v
		}
		return engine.ListValue{ElementType: elementType, Elements: nilIfEmpty(elements)}, nil
	case "map":
		keyType, err := DecodeType(pathField(path, "key_type"), w.KeyType)
		if err != nil {
			return nil, err
		}
		valueType, err := DecodeType(pathField(path, "value_type"), w.ValueType)
		if err != nil {
			return nil, err
		}
		entries := make([]engine.MapEntry, len(w.Entries))
		for i, e := range w.Entries {
			epath := pathIndex(pathField(path, "entries"), i)
			k, err := DecodeValue(pathField(epath, "key"), e.Key)
			if err != nil {
				return nil, err
			}
			v, err := DecodeValue(pathField(epath, "value"), e.Value)
			if err != nil {
				return nil, err
			}
			entries[i] = engine.MapEntry{Key: k, Value: v}
		}
		return engine.MapValue{KeyType: keyType, ValueType: valueType, Entries: nilIfEmpty(entries)}, nil
	default:
		return nil, newDecodeError(path, fmt.Sprintf("unrecognized value kind %q", kind), nil)
	}
}

func decodeFieldValues(path string, fields []fieldValueWire) ([]engine.FieldValue, error) {
	result := make([]engine.FieldValue, len(fields))
	for i, f := range fields {
		v, err := DecodeValue(pathIndex(pathField(path, "fields"), i), f.Value)
		if err != nil {
			return nil, err
		}
		result[i] = engine.FieldValue{Name: f.Name, Value: v}
	}
	return nilIfEmpty(result), nil
}
