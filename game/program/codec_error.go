package program

import "fmt"

// DecodeError describes a structural failure while decoding the JSON wire
// representation of a source-model node.
//
// Path identifies the location of the failure using a stable,
// JSON-path-like format rooted at "$" (for example "$.right",
// "$.cases[1].pattern", or "$.entries[0].value"). Message describes the
// structural problem (an unrecognized discriminator, an unexpected field,
// malformed JSON, and similar); Cause, when non-nil, wraps the underlying
// error (typically from encoding/json).
//
// DecodeError reports only wire-structure problems, never language
// semantics: a value that decodes successfully may still be a
// semantically invalid source object (an unknown built-in type string, an
// unknown operator, a duplicate name, and so on) — those remain the
// responsibility of the future semantic compiler.
type DecodeError struct {
	Path    string
	Message string
	Cause   error
}

// Error implements the error interface.
func (e *DecodeError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %s", e.Path, e.Message, e.Cause.Error())
	}
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}

// Unwrap returns Cause, allowing errors.Is and errors.As to see through a
// DecodeError to its underlying cause.
func (e *DecodeError) Unwrap() error {
	return e.Cause
}

// newDecodeError constructs a *DecodeError anchored at path.
func newDecodeError(path, message string, cause error) *DecodeError {
	return &DecodeError{Path: path, Message: message, Cause: cause}
}
