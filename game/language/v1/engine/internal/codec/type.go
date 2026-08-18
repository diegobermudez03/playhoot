package codec

import (
	"encoding/json"
	"fmt"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
)

// Type encoding here is deliberately lightweight: a named type
// (EnumType, RecordType, UnionType, NewType) encodes only its Kind and
// Name, never its full Values/Fields/Variants/Underlying definition.
// This is safe because every place a Value carries a Type (ListValue's
// ElementType, MapValue's KeyType/ValueType, OptionalValue's
// ElementType) is documented as "informational only" — Value.Equal
// never inspects it, and Value.Validate is always given the type to
// validate against from elsewhere. A decoded Snapshot is always used
// together with a freshly recompiled engine.Program, whose Program.Types
// holds every named type's real, authoritative definition; nothing
// needs to reconstruct one from a Value's own informational Type
// fields. This keeps the wire format small and avoids duplicating type
// definitions throughout a large Snapshot.
type typeWire struct {
	Kind    string          `json:"kind"`
	Name    string          `json:"name,omitempty"`
	Element json.RawMessage `json:"element,omitempty"`
	Key     json.RawMessage `json:"key,omitempty"`
	Value   json.RawMessage `json:"value,omitempty"`
}

// EncodeType encodes t, or JSON null if t is nil.
func EncodeType(path string, t engine.Type) (json.RawMessage, error) {
	if t == nil {
		return json.RawMessage("null"), nil
	}
	switch v := t.(type) {
	case engine.UnitType:
		return json.Marshal(typeWire{Kind: "unit"})
	case engine.BoolType:
		return json.Marshal(typeWire{Kind: "bool"})
	case engine.NumberType:
		return json.Marshal(typeWire{Kind: "number"})
	case engine.StringType:
		return json.Marshal(typeWire{Kind: "string"})
	case engine.UserType:
		return json.Marshal(typeWire{Kind: "user"})
	case engine.EnumType:
		return json.Marshal(typeWire{Kind: "enum", Name: v.Name})
	case engine.RecordType:
		return json.Marshal(typeWire{Kind: "record", Name: v.Name})
	case engine.UnionType:
		return json.Marshal(typeWire{Kind: "union", Name: v.Name})
	case engine.NewType:
		return json.Marshal(typeWire{Kind: "new_type", Name: v.Name})
	case engine.OptionalType:
		element, err := EncodeType(pathField(path, "element"), v.Element)
		if err != nil {
			return nil, err
		}
		return json.Marshal(typeWire{Kind: "optional", Element: element})
	case engine.ListType:
		element, err := EncodeType(pathField(path, "element"), v.Element)
		if err != nil {
			return nil, err
		}
		return json.Marshal(typeWire{Kind: "list", Element: element})
	case engine.MapType:
		key, err := EncodeType(pathField(path, "key"), v.Key)
		if err != nil {
			return nil, err
		}
		value, err := EncodeType(pathField(path, "value"), v.Value)
		if err != nil {
			return nil, err
		}
		return json.Marshal(typeWire{Kind: "map", Key: key, Value: value})
	default:
		return nil, newDecodeError(path, "unsupported engine.Type", nil)
	}
}

// DecodeType decodes data into an engine.Type, or nil for JSON null.
func DecodeType(path string, data json.RawMessage) (engine.Type, error) {
	if isEmptyOrNull(data) {
		return nil, nil
	}
	var w typeWire
	if err := strictDecodeInto(path, data, &w); err != nil {
		return nil, err
	}
	switch w.Kind {
	case "unit":
		return engine.UnitType{}, nil
	case "bool":
		return engine.BoolType{}, nil
	case "number":
		return engine.NumberType{}, nil
	case "string":
		return engine.StringType{}, nil
	case "user":
		return engine.UserType{}, nil
	case "enum":
		return engine.EnumType{Name: w.Name}, nil
	case "record":
		return engine.RecordType{Name: w.Name}, nil
	case "union":
		return engine.UnionType{Name: w.Name}, nil
	case "new_type":
		return engine.NewType{Name: w.Name}, nil
	case "optional":
		element, err := DecodeType(pathField(path, "element"), w.Element)
		if err != nil {
			return nil, err
		}
		return engine.OptionalType{Element: element}, nil
	case "list":
		element, err := DecodeType(pathField(path, "element"), w.Element)
		if err != nil {
			return nil, err
		}
		return engine.ListType{Element: element}, nil
	case "map":
		key, err := DecodeType(pathField(path, "key"), w.Key)
		if err != nil {
			return nil, err
		}
		value, err := DecodeType(pathField(path, "value"), w.Value)
		if err != nil {
			return nil, err
		}
		return engine.MapType{Key: key, Value: value}, nil
	default:
		return nil, newDecodeError(path, fmt.Sprintf("unrecognized type kind %q", w.Kind), nil)
	}
}
