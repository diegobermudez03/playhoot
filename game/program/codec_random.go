package program

import (
	"encoding/json"
	"fmt"
)

type wireRandomIntegerGenerator struct {
	Kind    string          `json:"kind"`
	Minimum json.RawMessage `json:"minimum"`
	Maximum json.RawMessage `json:"maximum"`
}

type wireRandomElementGenerator struct {
	Kind       string          `json:"kind"`
	Collection json.RawMessage `json:"collection"`
}

type wireRandomShuffleGenerator struct {
	Kind       string          `json:"kind"`
	Collection json.RawMessage `json:"collection"`
}

// encodeRandomGenerator encodes value as its JSON wire representation, or
// as JSON null when value is a nil interface or a typed nil pointer.
func encodeRandomGenerator(path string, value RandomGenerator) (json.RawMessage, error) {
	return encodeUnion(value, func(resolved any) (json.RawMessage, error) {
		switch v := resolved.(type) {
		case RandomIntegerGenerator:
			minimum, err := encodeExpression(pathField(path, "minimum"), v.Minimum)
			if err != nil {
				return nil, err
			}
			maximum, err := encodeExpression(pathField(path, "maximum"), v.Maximum)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireRandomIntegerGenerator{Kind: "random_integer", Minimum: minimum, Maximum: maximum})
		case RandomElementGenerator:
			collection, err := encodeExpression(pathField(path, "collection"), v.Collection)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireRandomElementGenerator{Kind: "random_element", Collection: collection})
		case RandomShuffleGenerator:
			collection, err := encodeExpression(pathField(path, "collection"), v.Collection)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireRandomShuffleGenerator{Kind: "random_shuffle", Collection: collection})
		default:
			return nil, fmt.Errorf("%s: unsupported RandomGenerator implementation %T", path, value)
		}
	})
}

// decodeRandomGenerator decodes data as a RandomGenerator, or returns a
// nil interface for JSON null or a missing value.
func decodeRandomGenerator(path string, data json.RawMessage) (RandomGenerator, error) {
	return decodeUnion(path, data, func(path, kind string, raw json.RawMessage) (RandomGenerator, error) {
		switch kind {
		case "random_integer":
			var wire wireRandomIntegerGenerator
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			minimum, err := decodeExpression(pathField(path, "minimum"), wire.Minimum)
			if err != nil {
				return nil, err
			}
			maximum, err := decodeExpression(pathField(path, "maximum"), wire.Maximum)
			if err != nil {
				return nil, err
			}
			return RandomIntegerGenerator{Minimum: minimum, Maximum: maximum}, nil
		case "random_element":
			var wire wireRandomElementGenerator
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			collection, err := decodeExpression(pathField(path, "collection"), wire.Collection)
			if err != nil {
				return nil, err
			}
			return RandomElementGenerator{Collection: collection}, nil
		case "random_shuffle":
			var wire wireRandomShuffleGenerator
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			collection, err := decodeExpression(pathField(path, "collection"), wire.Collection)
			if err != nil {
				return nil, err
			}
			return RandomShuffleGenerator{Collection: collection}, nil
		default:
			return nil, newDecodeError(path, fmt.Sprintf("unsupported random generator kind %q", kind), nil)
		}
	})
}
