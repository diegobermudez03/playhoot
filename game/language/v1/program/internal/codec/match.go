package codec

import (
	"encoding/json"
	"fmt"

	"github.com/diegobermudez03/playhoot/game/language/v1/program"
)

type wireWildcardMatchPattern struct {
	Kind string `json:"kind"`
}

type wireEnumValueMatchPattern struct {
	Kind      string `json:"kind"`
	TypeName  string `json:"type_name"`
	ValueName string `json:"value_name"`
}

type wireMatchFieldBinding struct {
	Field string `json:"field"`
	Name  string `json:"name"`
}

type wireUnionVariantMatchPattern struct {
	Kind        string                  `json:"kind"`
	TypeName    string                  `json:"type_name"`
	VariantName string                  `json:"variant_name"`
	Bindings    []wireMatchFieldBinding `json:"bindings"`
}

type wireOptionalNoneMatchPattern struct {
	Kind string `json:"kind"`
}

type wireOptionalSomeMatchPattern struct {
	Kind    string `json:"kind"`
	Binding string `json:"binding"`
}

// encodeMatchPattern encodes value as its JSON wire representation, or as
// JSON null when value is a nil interface or a typed nil pointer.
func encodeMatchPattern(path string, value program.MatchPattern) (json.RawMessage, error) {
	return encodeUnion(value, func(resolved any) (json.RawMessage, error) {
		switch v := resolved.(type) {
		case program.WildcardMatchPattern:
			return json.Marshal(wireWildcardMatchPattern{Kind: "wildcard"})
		case program.EnumValueMatchPattern:
			return json.Marshal(wireEnumValueMatchPattern{
				Kind:      "enum_value",
				TypeName:  v.TypeName,
				ValueName: v.ValueName,
			})
		case program.UnionVariantMatchPattern:
			var bindings []wireMatchFieldBinding
			if v.Bindings != nil {
				bindings = make([]wireMatchFieldBinding, len(v.Bindings))
				for i, b := range v.Bindings {
					bindings[i] = wireMatchFieldBinding{Field: b.Field, Name: b.Name}
				}
			}
			return json.Marshal(wireUnionVariantMatchPattern{
				Kind:        "union_variant",
				TypeName:    v.TypeName,
				VariantName: v.VariantName,
				Bindings:    bindings,
			})
		case program.OptionalNoneMatchPattern:
			return json.Marshal(wireOptionalNoneMatchPattern{Kind: "optional_none"})
		case program.OptionalSomeMatchPattern:
			return json.Marshal(wireOptionalSomeMatchPattern{Kind: "optional_some", Binding: v.Binding})
		default:
			return nil, fmt.Errorf("%s: unsupported program.MatchPattern implementation %T", path, value)
		}
	})
}

// decodeMatchPattern decodes data as a program.MatchPattern, or returns a nil
// interface for JSON null or a missing value.
func decodeMatchPattern(path string, data json.RawMessage) (program.MatchPattern, error) {
	return decodeUnion(path, data, func(path, kind string, raw json.RawMessage) (program.MatchPattern, error) {
		switch kind {
		case "wildcard":
			var wire wireWildcardMatchPattern
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return program.WildcardMatchPattern{}, nil
		case "enum_value":
			var wire wireEnumValueMatchPattern
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return program.EnumValueMatchPattern{TypeName: wire.TypeName, ValueName: wire.ValueName}, nil
		case "union_variant":
			var wire wireUnionVariantMatchPattern
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			var bindings []program.MatchFieldBinding
			if wire.Bindings != nil {
				bindings = make([]program.MatchFieldBinding, len(wire.Bindings))
				for i, b := range wire.Bindings {
					bindings[i] = program.MatchFieldBinding{Field: b.Field, Name: b.Name}
				}
			}
			return program.UnionVariantMatchPattern{
				TypeName:    wire.TypeName,
				VariantName: wire.VariantName,
				Bindings:    bindings,
			}, nil
		case "optional_none":
			var wire wireOptionalNoneMatchPattern
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return program.OptionalNoneMatchPattern{}, nil
		case "optional_some":
			var wire wireOptionalSomeMatchPattern
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return program.OptionalSomeMatchPattern{Binding: wire.Binding}, nil
		default:
			return nil, newDecodeError(path, fmt.Sprintf("unsupported match pattern kind %q", kind), nil)
		}
	})
}
