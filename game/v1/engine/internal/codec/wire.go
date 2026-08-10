package codec

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// wireKind peeks the "kind" discriminator of an encoded closed-interface
// value before decoding it strictly into its concrete wire struct.
type wireKind struct {
	Kind string `json:"kind"`
}

func isEmptyOrNull(data json.RawMessage) bool {
	trimmed := bytes.TrimSpace(data)
	return len(trimmed) == 0 || string(trimmed) == "null"
}

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

func pathField(path, field string) string {
	return path + "." + field
}

func pathIndex(path string, index int) string {
	return fmt.Sprintf("%s[%d]", path, index)
}

// nilIfEmpty normalizes a decoded slice built with make([]T, len(wire))
// back to nil when wire was nil or empty — make always returns a
// non-nil slice even for length 0, but the engine types this package
// decodes into use nil to mean "no entries at all" (matching how they
// are normally constructed, and what reflect.DeepEqual-based tests
// expect after a round trip).
func nilIfEmpty[T any](s []T) []T {
	if len(s) == 0 {
		return nil
	}
	return s
}

// decodeUnion is a small dispatch helper shared by every closed-interface
// decoder in this package: it isolates exactly one top-level JSON value,
// reads its "kind" discriminator, and hands off to dispatch to decode
// the matching concrete wire struct.
func decodeUnion[T any](path string, data json.RawMessage, dispatch func(path, kind string, raw json.RawMessage) (T, error)) (T, error) {
	var zero T
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

// decodeOptionalUnion is decodeUnion, but a null or missing value
// decodes to (zero, false, nil) instead of an error — used for the
// handful of nullable closed-interface fields (a pending slot, a
// terminal outcome).
func decodeOptionalUnion[T any](path string, data json.RawMessage, dispatch func(path, kind string, raw json.RawMessage) (T, error)) (T, bool, error) {
	var zero T
	if isEmptyOrNull(data) {
		return zero, false, nil
	}
	v, err := decodeUnion(path, data, dispatch)
	return v, err == nil, err
}
