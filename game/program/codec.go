package program

import "encoding/json"

// EncodeJSON encodes definition as compact JSON using this package's
// private wire schema, preserving declaration order, duplicate names,
// the distinction between nil and empty slices, and nil closed-interface
// values as JSON null.
//
// EncodeJSON performs no semantic validation: a structurally valid but
// semantically invalid Definition still encodes successfully, since
// semantic diagnostics belong to a future engine compiler. Encoding the
// same in-memory Definition twice always produces identical bytes, but
// this determinism is a property of the current wire schema and source
// model, not a cross-version canonical hash format.
func EncodeJSON(definition Definition) ([]byte, error) {
	raw, err := encodeDefinition("$", definition)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

// DecodeJSON decodes data as a Definition using this package's private
// wire schema.
//
// DecodeJSON requires data to hold exactly one JSON object (trailing
// non-whitespace data, JSON null, and non-object top-level values are all
// rejected); every nested value is decoded through this package's
// existing per-construct codecs, so decode failures are reported as a
// path-aware *DecodeError rooted at "$". DecodeJSON is structurally strict
// but semantically permissive: it rejects unknown fields and unrecognized
// discriminators, but a structurally valid, semantically invalid
// definition (duplicate names, unresolved references, and so on) still
// decodes successfully — those diagnostics belong to a future engine
// compiler, not this codec. On success, DecodeJSON always returns a
// non-nil *Definition.
func DecodeJSON(data []byte) (*Definition, error) {
	definition, err := decodeDefinition("$", json.RawMessage(data))
	if err != nil {
		return nil, err
	}
	return &definition, nil
}
