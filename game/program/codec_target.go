package program

import (
	"encoding/json"
	"fmt"
)

type wireNameTarget struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

type wireFieldTarget struct {
	Kind   string          `json:"kind"`
	Target json.RawMessage `json:"target"`
	Field  string          `json:"field"`
}

type wireIndexTarget struct {
	Kind   string          `json:"kind"`
	Target json.RawMessage `json:"target"`
	Index  json.RawMessage `json:"index"`
}

// encodeAssignmentTarget encodes value as its JSON wire representation, or
// as JSON null when value is a nil interface or a typed nil pointer.
func encodeAssignmentTarget(path string, value AssignmentTarget) (json.RawMessage, error) {
	return encodeUnion(value, func(resolved any) (json.RawMessage, error) {
		switch v := resolved.(type) {
		case NameTarget:
			return json.Marshal(wireNameTarget{Kind: "name", Name: v.Name})
		case FieldTarget:
			target, err := encodeAssignmentTarget(pathField(path, "target"), v.Target)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireFieldTarget{Kind: "field", Target: target, Field: v.Field})
		case IndexTarget:
			target, err := encodeAssignmentTarget(pathField(path, "target"), v.Target)
			if err != nil {
				return nil, err
			}
			index, err := encodeExpression(pathField(path, "index"), v.Index)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireIndexTarget{Kind: "index", Target: target, Index: index})
		default:
			return nil, fmt.Errorf("%s: unsupported AssignmentTarget implementation %T", path, value)
		}
	})
}

// decodeAssignmentTarget decodes data as an AssignmentTarget, or returns a
// nil interface for JSON null or a missing value.
func decodeAssignmentTarget(path string, data json.RawMessage) (AssignmentTarget, error) {
	return decodeUnion(path, data, func(path, kind string, raw json.RawMessage) (AssignmentTarget, error) {
		switch kind {
		case "name":
			var wire wireNameTarget
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return NameTarget{Name: wire.Name}, nil
		case "field":
			var wire wireFieldTarget
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			target, err := decodeAssignmentTarget(pathField(path, "target"), wire.Target)
			if err != nil {
				return nil, err
			}
			return FieldTarget{Target: target, Field: wire.Field}, nil
		case "index":
			var wire wireIndexTarget
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			target, err := decodeAssignmentTarget(pathField(path, "target"), wire.Target)
			if err != nil {
				return nil, err
			}
			index, err := decodeExpression(pathField(path, "index"), wire.Index)
			if err != nil {
				return nil, err
			}
			return IndexTarget{Target: target, Index: index}, nil
		default:
			return nil, newDecodeError(path, fmt.Sprintf("unsupported assignment target kind %q", kind), nil)
		}
	})
}
