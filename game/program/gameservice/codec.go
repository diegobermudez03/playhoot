// Package gameservice exposes the public behavior built on top of the
// program package's source model: encoding a Definition to JSON, decoding
// JSON back into a Definition, and validating a Definition against the
// game language's own rules.
//
// The program package itself owns only the source-model types (Definition
// and everything it is built from) and depends on nothing outside the Go
// standard library. All serialization and validation behavior lives here
// instead, built on top of program's exported types plus the private wire
// codec in program/internal/codec.
package gameservice

import (
	"github.com/diegobermudez03/playhoot/game/program"
	"github.com/diegobermudez03/playhoot/game/program/internal/codec"
)

// DecodeError describes a structural failure while decoding the JSON wire
// representation of a source-model node. See the underlying
// program/internal/codec.DecodeError for field and path documentation.
type DecodeError = codec.DecodeError

// EncodeJSON encodes definition as compact JSON, preserving declaration
// order, duplicate names, the distinction between nil and empty slices,
// and nil closed-interface values as JSON null.
//
// EncodeJSON performs no semantic validation: a structurally valid but
// semantically invalid Definition still encodes successfully. Call
// Validate separately if you need language-rule diagnostics. Encoding the
// same in-memory Definition twice always produces identical bytes, but
// this determinism is a property of the current wire schema and source
// model, not a cross-version canonical hash format.
func EncodeJSON(definition program.Definition) ([]byte, error) {
	return codec.EncodeDefinition("$", definition)
}

// DecodeJSON decodes data as a program.Definition.
//
// DecodeJSON requires data to hold exactly one JSON object (trailing
// non-whitespace data, JSON null, and non-object top-level values are all
// rejected); every nested value is decoded through program/internal/codec,
// so decode failures are reported as a path-aware *DecodeError rooted at
// "$". DecodeJSON is structurally strict but semantically permissive: it
// rejects unknown fields and unrecognized discriminators, but a
// structurally valid, semantically invalid definition (duplicate names,
// unresolved references, and so on) still decodes successfully — call
// Validate separately for those diagnostics. On success, DecodeJSON always
// returns a non-nil *program.Definition.
func DecodeJSON(data []byte) (*program.Definition, error) {
	definition, err := codec.DecodeDefinition("$", data)
	if err != nil {
		return nil, err
	}
	return &definition, nil
}
