package codec

import "fmt"

// DecodeError describes a structural failure while decoding the JSON
// wire representation of a runtime engine value (a Snapshot, a Value,
// a WorkflowInstance, or anything nested within one).
//
// Path identifies the location of the failure using a stable,
// JSON-path-like format rooted at "$" (for example "$.root.state",
// "$.root.question_slots[0].pending.answer"). Message describes the
// structural problem; Cause, when non-nil, wraps the underlying error
// (typically from encoding/json).
//
// DecodeError reports only wire-structure problems, never semantic
// ones: a Snapshot that decodes successfully may still be incompatible
// with a particular engine.Program (a different RootWorkflow, an
// unknown slot name) — see engineservice's CheckSnapshotCompatibility.
type DecodeError struct {
	Path    string
	Message string
	Cause   error
}

func (e *DecodeError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %s", e.Path, e.Message, e.Cause.Error())
	}
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}

func (e *DecodeError) Unwrap() error {
	return e.Cause
}

func newDecodeError(path, message string, cause error) *DecodeError {
	return &DecodeError{Path: path, Message: message, Cause: cause}
}
