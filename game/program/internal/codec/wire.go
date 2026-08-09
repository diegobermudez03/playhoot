package codec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
)

// wireKind is used to peek the "kind" discriminator of an encoded closed-
// interface value before decoding it strictly into its concrete wire
// struct.
type wireKind struct {
	Kind string `json:"kind"`
}

// isEmptyOrNull reports whether data represents a missing value or JSON
// null. A missing field (zero-length RawMessage, which occurs when a JSON
// object omits an optional key) is treated the same as an explicit null,
// since both mean "no value" for the optional closed-interface fields
// handled by this codec.
func isEmptyOrNull(data json.RawMessage) bool {
	trimmed := bytes.TrimSpace(data)
	return len(trimmed) == 0 || string(trimmed) == "null"
}

// decodeTopLevelValue decodes exactly one JSON value from data and
// rejects any trailing content after it.
func decodeTopLevelValue(path string, data []byte) (json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return nil, newDecodeError(path, "invalid JSON", err)
	}
	if dec.More() {
		return nil, newDecodeError(path, "unexpected trailing data after JSON value", nil)
	}
	return raw, nil
}

// readKind extracts the "kind" discriminator from a JSON object without
// otherwise validating its shape; full structural validation happens once
// the kind is known and the value is strictly decoded into its concrete
// wire struct.
func readKind(path string, data json.RawMessage) (string, error) {
	var k wireKind
	if err := json.Unmarshal(data, &k); err != nil {
		return "", newDecodeError(path, `expected a JSON object with a "kind" field`, err)
	}
	if k.Kind == "" {
		return "", newDecodeError(path, `missing required "kind" field`, nil)
	}
	return k.Kind, nil
}

// strictDecodeInto decodes data into v, rejecting unknown fields and any
// trailing content after the object.
func strictDecodeInto(path string, data json.RawMessage, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return newDecodeError(path, "invalid structure", err)
	}
	if dec.More() {
		return newDecodeError(path, "unexpected trailing data", nil)
	}
	return nil
}

// pathField appends a named object field to path.
func pathField(path, field string) string {
	return path + "." + field
}

// pathIndex appends an array index to path.
func pathIndex(path string, index int) string {
	return fmt.Sprintf("%s[%d]", path, index)
}

// dereferencePointer normalizes value for encoding: a nil interface or a
// typed nil pointer is reported via the second return value, and a non-nil
// pointer is dereferenced to the value it points to so callers can type
// switch on concrete node types regardless of whether the caller passed a
// value or a pointer. This is the codec's only use of reflection; it is
// never used to serialize AST fields generically.
func dereferencePointer(value any) (resolved any, isNil bool) {
	if value == nil {
		return nil, true
	}
	rv := reflect.ValueOf(value)
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return nil, true
		}
		return rv.Elem().Interface(), false
	}
	return value, false
}

// decodeOrdinaryObject decodes data into v, an ordinary (non-interface)
// wire struct such as a program.Block, a program.SignalPattern, or one of this package's
// reusable declarations. Unlike decodeUnion, JSON null is not a valid
// encoding of an ordinary object: it produces a path-aware structural
// error instead of silently leaving v at its zero value.
func decodeOrdinaryObject(path string, data json.RawMessage, v any) error {
	if isEmptyOrNull(data) {
		return newDecodeError(path, "expected an object, got null or missing value", nil)
	}
	raw, err := decodeTopLevelValue(path, data)
	if err != nil {
		return err
	}
	return strictDecodeInto(path, raw, v)
}

// decodeUnion is a small dispatch helper shared by every closed-interface
// decoder in this package: it handles the null/missing case, decodes and
// isolates exactly one top-level JSON value, reads its "kind"
// discriminator, and hands off to dispatch to decode the matching concrete
// wire struct. It performs no reflection-based field mapping; dispatch is
// responsible for that, one concrete type at a time.
func decodeUnion[T any](path string, data json.RawMessage, dispatch func(path, kind string, raw json.RawMessage) (T, error)) (T, error) {
	var zero T
	if isEmptyOrNull(data) {
		return zero, nil
	}
	raw, err := decodeTopLevelValue(path, data)
	if err != nil {
		return zero, err
	}
	kind, err := readKind(path, raw)
	if err != nil {
		return zero, err
	}
	return dispatch(path, kind, raw)
}

// encodeUnion is the encode-side counterpart to decodeUnion: it handles
// the nil/typed-nil case and normalizes a pointer to its pointed-to value
// before handing off to encode, which type switches over the concrete
// node types one at a time.
func encodeUnion(value any, encode func(resolved any) (json.RawMessage, error)) (json.RawMessage, error) {
	resolved, isNil := dereferencePointer(value)
	if isNil {
		return json.RawMessage("null"), nil
	}
	return encode(resolved)
}
