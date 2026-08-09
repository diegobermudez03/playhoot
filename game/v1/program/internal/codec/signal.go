package codec

import (
	"encoding/json"
	"fmt"

	"github.com/diegobermudez03/playhoot/game/v1/program"
)

type wireNamedSignalSource struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

type wireUserIntentSignalSource struct {
	Kind   string `json:"kind"`
	Intent string `json:"intent"`
}

type wireQuestionAnsweredSignalSource struct {
	Kind string `json:"kind"`
	Slot string `json:"slot"`
}

type wireTimerExpiredSignalSource struct {
	Kind string `json:"kind"`
	Slot string `json:"slot"`
}

type wireChildCompletedSignalSource struct {
	Kind string `json:"kind"`
	Slot string `json:"slot"`
}

type wireChildFailedSignalSource struct {
	Kind string `json:"kind"`
	Slot string `json:"slot"`
}

type wireChildCancelledSignalSource struct {
	Kind string `json:"kind"`
	Slot string `json:"slot"`
}

type wireAskGroupCompletedSignalSource struct {
	Kind string `json:"kind"`
	Slot string `json:"slot"`
}

type wireTaskGroupCompletedSignalSource struct {
	Kind string `json:"kind"`
	Slot string `json:"slot"`
}

// encodeSignalSource encodes value as its JSON wire representation, or as
// JSON null when value is a nil interface or a typed nil pointer.
func encodeSignalSource(path string, value program.SignalSource) (json.RawMessage, error) {
	return encodeUnion(value, func(resolved any) (json.RawMessage, error) {
		switch v := resolved.(type) {
		case program.NamedSignalSource:
			return json.Marshal(wireNamedSignalSource{Kind: "named", Name: v.Name})
		case program.UserIntentSignalSource:
			return json.Marshal(wireUserIntentSignalSource{Kind: "user_intent", Intent: v.Intent})
		case program.QuestionAnsweredSignalSource:
			return json.Marshal(wireQuestionAnsweredSignalSource{Kind: "question_answered", Slot: v.Slot})
		case program.TimerExpiredSignalSource:
			return json.Marshal(wireTimerExpiredSignalSource{Kind: "timer_expired", Slot: v.Slot})
		case program.ChildCompletedSignalSource:
			return json.Marshal(wireChildCompletedSignalSource{Kind: "child_completed", Slot: v.Slot})
		case program.ChildFailedSignalSource:
			return json.Marshal(wireChildFailedSignalSource{Kind: "child_failed", Slot: v.Slot})
		case program.ChildCancelledSignalSource:
			return json.Marshal(wireChildCancelledSignalSource{Kind: "child_cancelled", Slot: v.Slot})
		case program.AskGroupCompletedSignalSource:
			return json.Marshal(wireAskGroupCompletedSignalSource{Kind: "ask_group_completed", Slot: v.Slot})
		case program.TaskGroupCompletedSignalSource:
			return json.Marshal(wireTaskGroupCompletedSignalSource{Kind: "task_group_completed", Slot: v.Slot})
		default:
			return nil, fmt.Errorf("%s: unsupported program.SignalSource implementation %T", path, value)
		}
	})
}

// decodeSignalSource decodes data as a program.SignalSource, or returns a nil
// interface for JSON null or a missing value.
func decodeSignalSource(path string, data json.RawMessage) (program.SignalSource, error) {
	return decodeUnion(path, data, func(path, kind string, raw json.RawMessage) (program.SignalSource, error) {
		switch kind {
		case "named":
			var wire wireNamedSignalSource
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return program.NamedSignalSource{Name: wire.Name}, nil
		case "user_intent":
			var wire wireUserIntentSignalSource
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return program.UserIntentSignalSource{Intent: wire.Intent}, nil
		case "question_answered":
			var wire wireQuestionAnsweredSignalSource
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return program.QuestionAnsweredSignalSource{Slot: wire.Slot}, nil
		case "timer_expired":
			var wire wireTimerExpiredSignalSource
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return program.TimerExpiredSignalSource{Slot: wire.Slot}, nil
		case "child_completed":
			var wire wireChildCompletedSignalSource
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return program.ChildCompletedSignalSource{Slot: wire.Slot}, nil
		case "child_failed":
			var wire wireChildFailedSignalSource
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return program.ChildFailedSignalSource{Slot: wire.Slot}, nil
		case "child_cancelled":
			var wire wireChildCancelledSignalSource
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return program.ChildCancelledSignalSource{Slot: wire.Slot}, nil
		case "ask_group_completed":
			var wire wireAskGroupCompletedSignalSource
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return program.AskGroupCompletedSignalSource{Slot: wire.Slot}, nil
		case "task_group_completed":
			var wire wireTaskGroupCompletedSignalSource
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return program.TaskGroupCompletedSignalSource{Slot: wire.Slot}, nil
		default:
			return nil, newDecodeError(path, fmt.Sprintf("unsupported signal source kind %q", kind), nil)
		}
	})
}

// --- program.SignalPattern (ordinary struct, not a closed interface) ---

type wireSignalBinding struct {
	Field string `json:"field"`
	Name  string `json:"name"`
}

type wireSignalPattern struct {
	Source   json.RawMessage     `json:"source"`
	Bindings []wireSignalBinding `json:"bindings"`
}

func encodeSignalBindings(bindings []program.SignalBinding) []wireSignalBinding {
	if bindings == nil {
		return nil
	}
	result := make([]wireSignalBinding, len(bindings))
	for i, b := range bindings {
		result[i] = wireSignalBinding{Field: b.Field, Name: b.Name}
	}
	return result
}

func decodeSignalBindings(bindings []wireSignalBinding) []program.SignalBinding {
	if bindings == nil {
		return nil
	}
	result := make([]program.SignalBinding, len(bindings))
	for i, b := range bindings {
		result[i] = program.SignalBinding{Field: b.Field, Name: b.Name}
	}
	return result
}

// encodeSignalPattern encodes value, an ordinary (non-interface) struct,
// as its JSON wire representation.
func encodeSignalPattern(path string, value program.SignalPattern) (json.RawMessage, error) {
	source, err := encodeSignalSource(pathField(path, "source"), value.Source)
	if err != nil {
		return nil, err
	}
	return json.Marshal(wireSignalPattern{Source: source, Bindings: encodeSignalBindings(value.Bindings)})
}

// decodeSignalPattern decodes data as a program.SignalPattern. Because
// program.SignalPattern is an ordinary struct rather than a closed interface, JSON
// null is not a valid encoding of it and produces a path-aware structural
// error instead of a silent zero value.
func decodeSignalPattern(path string, data json.RawMessage) (program.SignalPattern, error) {
	if isEmptyOrNull(data) {
		return program.SignalPattern{}, newDecodeError(path, "expected a signal pattern object, got null or missing value", nil)
	}
	raw, err := decodeTopLevelValue(path, data)
	if err != nil {
		return program.SignalPattern{}, err
	}
	var wire wireSignalPattern
	if err := strictDecodeInto(path, raw, &wire); err != nil {
		return program.SignalPattern{}, err
	}
	source, err := decodeSignalSource(pathField(path, "source"), wire.Source)
	if err != nil {
		return program.SignalPattern{}, err
	}
	return program.SignalPattern{Source: source, Bindings: decodeSignalBindings(wire.Bindings)}, nil
}
