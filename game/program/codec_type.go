package program

import (
	"encoding/json"
	"fmt"
)

// --- TypeReference wire structs ---

type wireBuiltinTypeReference struct {
	Kind string      `json:"kind"`
	Type BuiltinType `json:"type"`
}

type wireNamedTypeReference struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

type wireListTypeReference struct {
	Kind    string          `json:"kind"`
	Element json.RawMessage `json:"element"`
}

type wireMapTypeReference struct {
	Kind  string          `json:"kind"`
	Key   json.RawMessage `json:"key"`
	Value json.RawMessage `json:"value"`
}

type wireOptionalTypeReference struct {
	Kind    string          `json:"kind"`
	Element json.RawMessage `json:"element"`
}

// encodeTypeReference encodes value as its JSON wire representation, or as
// JSON null when value is a nil interface or a typed nil pointer.
func encodeTypeReference(path string, value TypeReference) (json.RawMessage, error) {
	return encodeUnion(value, func(resolved any) (json.RawMessage, error) {
		switch v := resolved.(type) {
		case BuiltinTypeReference:
			return json.Marshal(wireBuiltinTypeReference{Kind: "builtin", Type: v.Type})
		case NamedTypeReference:
			return json.Marshal(wireNamedTypeReference{Kind: "named", Name: v.Name})
		case ListTypeReference:
			element, err := encodeTypeReference(pathField(path, "element"), v.Element)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireListTypeReference{Kind: "list", Element: element})
		case MapTypeReference:
			key, err := encodeTypeReference(pathField(path, "key"), v.Key)
			if err != nil {
				return nil, err
			}
			value, err := encodeTypeReference(pathField(path, "value"), v.Value)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireMapTypeReference{Kind: "map", Key: key, Value: value})
		case OptionalTypeReference:
			element, err := encodeTypeReference(pathField(path, "element"), v.Element)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireOptionalTypeReference{Kind: "optional", Element: element})
		default:
			return nil, fmt.Errorf("%s: unsupported TypeReference implementation %T", path, value)
		}
	})
}

// decodeTypeReference decodes data as a TypeReference, or returns a nil
// interface for JSON null or a missing value.
func decodeTypeReference(path string, data json.RawMessage) (TypeReference, error) {
	return decodeUnion(path, data, func(path, kind string, raw json.RawMessage) (TypeReference, error) {
		switch kind {
		case "builtin":
			var wire wireBuiltinTypeReference
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return BuiltinTypeReference{Type: wire.Type}, nil
		case "named":
			var wire wireNamedTypeReference
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return NamedTypeReference{Name: wire.Name}, nil
		case "list":
			var wire wireListTypeReference
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			element, err := decodeTypeReference(pathField(path, "element"), wire.Element)
			if err != nil {
				return nil, err
			}
			return ListTypeReference{Element: element}, nil
		case "map":
			var wire wireMapTypeReference
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			key, err := decodeTypeReference(pathField(path, "key"), wire.Key)
			if err != nil {
				return nil, err
			}
			value, err := decodeTypeReference(pathField(path, "value"), wire.Value)
			if err != nil {
				return nil, err
			}
			return MapTypeReference{Key: key, Value: value}, nil
		case "optional":
			var wire wireOptionalTypeReference
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			element, err := decodeTypeReference(pathField(path, "element"), wire.Element)
			if err != nil {
				return nil, err
			}
			return OptionalTypeReference{Element: element}, nil
		default:
			return nil, newDecodeError(path, fmt.Sprintf("unsupported type reference kind %q", kind), nil)
		}
	})
}

// --- FieldDeclaration (non-union, reused by record and union-variant declarations) ---

type wireFieldDeclaration struct {
	Name string          `json:"name"`
	Type json.RawMessage `json:"type"`
}

func encodeFieldDeclarations(path string, fields []FieldDeclaration) ([]wireFieldDeclaration, error) {
	if fields == nil {
		return nil, nil
	}
	result := make([]wireFieldDeclaration, len(fields))
	for i, field := range fields {
		itemPath := pathIndex(path, i)
		typeRaw, err := encodeTypeReference(pathField(itemPath, "type"), field.Type)
		if err != nil {
			return nil, err
		}
		result[i] = wireFieldDeclaration{Name: field.Name, Type: typeRaw}
	}
	return result, nil
}

func decodeFieldDeclarations(path string, fields []wireFieldDeclaration) ([]FieldDeclaration, error) {
	if fields == nil {
		return nil, nil
	}
	result := make([]FieldDeclaration, len(fields))
	for i, field := range fields {
		itemPath := pathIndex(path, i)
		fieldType, err := decodeTypeReference(pathField(itemPath, "type"), field.Type)
		if err != nil {
			return nil, err
		}
		result[i] = FieldDeclaration{Name: field.Name, Type: fieldType}
	}
	return result, nil
}

// --- TypeDeclaration wire structs ---

type wireEnumValueDeclaration struct {
	Name string `json:"name"`
}

type wireEnumTypeDeclaration struct {
	Kind   string                     `json:"kind"`
	Name   string                     `json:"name"`
	Values []wireEnumValueDeclaration `json:"values"`
}

type wireRecordTypeDeclaration struct {
	Kind   string                 `json:"kind"`
	Name   string                 `json:"name"`
	Fields []wireFieldDeclaration `json:"fields"`
}

type wireUnionVariantDeclaration struct {
	Name   string                 `json:"name"`
	Fields []wireFieldDeclaration `json:"fields"`
}

type wireUnionTypeDeclaration struct {
	Kind     string                        `json:"kind"`
	Name     string                        `json:"name"`
	Variants []wireUnionVariantDeclaration `json:"variants"`
}

type wireNewTypeDeclaration struct {
	Kind       string          `json:"kind"`
	Name       string          `json:"name"`
	Underlying json.RawMessage `json:"underlying"`
}

// encodeTypeDeclaration encodes value as its JSON wire representation, or
// as JSON null when value is a nil interface or a typed nil pointer.
func encodeTypeDeclaration(path string, value TypeDeclaration) (json.RawMessage, error) {
	return encodeUnion(value, func(resolved any) (json.RawMessage, error) {
		switch v := resolved.(type) {
		case EnumTypeDeclaration:
			var values []wireEnumValueDeclaration
			if v.Values != nil {
				values = make([]wireEnumValueDeclaration, len(v.Values))
				for i, ev := range v.Values {
					values[i] = wireEnumValueDeclaration{Name: ev.Name}
				}
			}
			return json.Marshal(wireEnumTypeDeclaration{Kind: "enum", Name: v.Name, Values: values})
		case RecordTypeDeclaration:
			fields, err := encodeFieldDeclarations(pathField(path, "fields"), v.Fields)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireRecordTypeDeclaration{Kind: "record", Name: v.Name, Fields: fields})
		case UnionTypeDeclaration:
			var variants []wireUnionVariantDeclaration
			if v.Variants != nil {
				variants = make([]wireUnionVariantDeclaration, len(v.Variants))
				for i, variant := range v.Variants {
					variantPath := pathIndex(pathField(path, "variants"), i)
					fields, err := encodeFieldDeclarations(pathField(variantPath, "fields"), variant.Fields)
					if err != nil {
						return nil, err
					}
					variants[i] = wireUnionVariantDeclaration{Name: variant.Name, Fields: fields}
				}
			}
			return json.Marshal(wireUnionTypeDeclaration{Kind: "union", Name: v.Name, Variants: variants})
		case NewTypeDeclaration:
			underlying, err := encodeTypeReference(pathField(path, "underlying"), v.Underlying)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireNewTypeDeclaration{Kind: "new_type", Name: v.Name, Underlying: underlying})
		default:
			return nil, fmt.Errorf("%s: unsupported TypeDeclaration implementation %T", path, value)
		}
	})
}

// decodeTypeDeclaration decodes data as a TypeDeclaration, or returns a
// nil interface for JSON null or a missing value.
func decodeTypeDeclaration(path string, data json.RawMessage) (TypeDeclaration, error) {
	return decodeUnion(path, data, func(path, kind string, raw json.RawMessage) (TypeDeclaration, error) {
		switch kind {
		case "enum":
			var wire wireEnumTypeDeclaration
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			var values []EnumValueDeclaration
			if wire.Values != nil {
				values = make([]EnumValueDeclaration, len(wire.Values))
				for i, ev := range wire.Values {
					values[i] = EnumValueDeclaration{Name: ev.Name}
				}
			}
			return EnumTypeDeclaration{Name: wire.Name, Values: values}, nil
		case "record":
			var wire wireRecordTypeDeclaration
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			fields, err := decodeFieldDeclarations(pathField(path, "fields"), wire.Fields)
			if err != nil {
				return nil, err
			}
			return RecordTypeDeclaration{Name: wire.Name, Fields: fields}, nil
		case "union":
			var wire wireUnionTypeDeclaration
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			var variants []UnionVariantDeclaration
			if wire.Variants != nil {
				variants = make([]UnionVariantDeclaration, len(wire.Variants))
				for i, variant := range wire.Variants {
					variantPath := pathIndex(pathField(path, "variants"), i)
					fields, err := decodeFieldDeclarations(pathField(variantPath, "fields"), variant.Fields)
					if err != nil {
						return nil, err
					}
					variants[i] = UnionVariantDeclaration{Name: variant.Name, Fields: fields}
				}
			}
			return UnionTypeDeclaration{Name: wire.Name, Variants: variants}, nil
		case "new_type":
			var wire wireNewTypeDeclaration
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			underlying, err := decodeTypeReference(pathField(path, "underlying"), wire.Underlying)
			if err != nil {
				return nil, err
			}
			return NewTypeDeclaration{Name: wire.Name, Underlying: underlying}, nil
		default:
			return nil, newDecodeError(path, fmt.Sprintf("unsupported type declaration kind %q", kind), nil)
		}
	})
}
