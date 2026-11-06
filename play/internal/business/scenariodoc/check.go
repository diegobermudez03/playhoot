package scenariodoc

import (
	"encoding/json"
	"reflect"
)

// Verdict is how an expectation fared against a document: Actual is the
// value found at its path (nil when absent).
type Verdict struct {
	Passed bool
	Actual json.RawMessage
	// Problem explains a comparison that could not be made (a number op on
	// a value that is not a number, ...), empty otherwise.
	Problem string
}

// Check compares the value at expectation's path in document, the global
// state or a participant's view, as expectation's op says. Decode accepted
// the expectation, so its path and value are valid.
func Check(expectation Expectation, document json.RawMessage) Verdict {
	segments, err := ParsePath(expectation.Path)
	if err != nil {
		return Verdict{Problem: err.Error()}
	}
	actual, found := Resolve(document, segments)
	verdict := Verdict{Actual: actual}
	switch expectation.Op {
	case OpExists:
		verdict.Passed = found
		return verdict
	case OpAbsent:
		verdict.Passed = !found
		return verdict
	}
	if !found {
		verdict.Problem = "nothing at that path"
		return verdict
	}
	var got, want any
	_ = json.Unmarshal(actual, &got)
	_ = json.Unmarshal(expectation.Value, &want)
	switch expectation.Op {
	case OpEq:
		verdict.Passed = reflect.DeepEqual(got, want)
	case OpNe:
		verdict.Passed = !reflect.DeepEqual(got, want)
	case OpIn:
		for _, accepted := range want.([]any) {
			if reflect.DeepEqual(got, accepted) {
				verdict.Passed = true
			}
		}
	case OpLen:
		length := -1
		switch typed := got.(type) {
		case []any:
			length = len(typed)
		case map[string]any:
			length = len(typed)
		case string:
			length = len([]rune(typed))
		}
		if length < 0 {
			verdict.Problem = "len needs an array, an object or a string"
			return verdict
		}
		verdict.Passed = float64(length) == want.(float64)
	case OpLt, OpLte, OpGt, OpGte:
		number, ok := got.(float64)
		if !ok {
			verdict.Problem = expectation.Op + " needs a number at that path"
			return verdict
		}
		limit := want.(float64)
		switch expectation.Op {
		case OpLt:
			verdict.Passed = number < limit
		case OpLte:
			verdict.Passed = number <= limit
		case OpGt:
			verdict.Passed = number > limit
		default:
			verdict.Passed = number >= limit
		}
	}
	return verdict
}
