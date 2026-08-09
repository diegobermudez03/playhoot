package program

import (
	"encoding/json"
	"fmt"
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
func encodeSignalSource(path string, value SignalSource) (json.RawMessage, error) {
	return encodeUnion(value, func(resolved any) (json.RawMessage, error) {
		switch v := resolved.(type) {
		case NamedSignalSource:
			return json.Marshal(wireNamedSignalSource{Kind: "named", Name: v.Name})
		case UserIntentSignalSource:
			return json.Marshal(wireUserIntentSignalSource{Kind: "user_intent", Intent: v.Intent})
		case QuestionAnsweredSignalSource:
			return json.Marshal(wireQuestionAnsweredSignalSource{Kind: "question_answered", Slot: v.Slot})
		case TimerExpiredSignalSource:
			return json.Marshal(wireTimerExpiredSignalSource{Kind: "timer_expired", Slot: v.Slot})
		case ChildCompletedSignalSource:
			return json.Marshal(wireChildCompletedSignalSource{Kind: "child_completed", Slot: v.Slot})
		case ChildFailedSignalSource:
			return json.Marshal(wireChildFailedSignalSource{Kind: "child_failed", Slot: v.Slot})
		case ChildCancelledSignalSource:
			return json.Marshal(wireChildCancelledSignalSource{Kind: "child_cancelled", Slot: v.Slot})
		case AskGroupCompletedSignalSource:
			return json.Marshal(wireAskGroupCompletedSignalSource{Kind: "ask_group_completed", Slot: v.Slot})
		case TaskGroupCompletedSignalSource:
			return json.Marshal(wireTaskGroupCompletedSignalSource{Kind: "task_group_completed", Slot: v.Slot})
		default:
			return nil, fmt.Errorf("%s: unsupported SignalSource implementation %T", path, value)
		}
	})
}

// decodeSignalSource decodes data as a SignalSource, or returns a nil
// interface for JSON null or a missing value.
func decodeSignalSource(path string, data json.RawMessage) (SignalSource, error) {
	return decodeUnion(path, data, func(path, kind string, raw json.RawMessage) (SignalSource, error) {
		switch kind {
		case "named":
			var wire wireNamedSignalSource
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return NamedSignalSource{Name: wire.Name}, nil
		case "user_intent":
			var wire wireUserIntentSignalSource
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return UserIntentSignalSource{Intent: wire.Intent}, nil
		case "question_answered":
			var wire wireQuestionAnsweredSignalSource
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return QuestionAnsweredSignalSource{Slot: wire.Slot}, nil
		case "timer_expired":
			var wire wireTimerExpiredSignalSource
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return TimerExpiredSignalSource{Slot: wire.Slot}, nil
		case "child_completed":
			var wire wireChildCompletedSignalSource
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return ChildCompletedSignalSource{Slot: wire.Slot}, nil
		case "child_failed":
			var wire wireChildFailedSignalSource
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return ChildFailedSignalSource{Slot: wire.Slot}, nil
		case "child_cancelled":
			var wire wireChildCancelledSignalSource
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return ChildCancelledSignalSource{Slot: wire.Slot}, nil
		case "ask_group_completed":
			var wire wireAskGroupCompletedSignalSource
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return AskGroupCompletedSignalSource{Slot: wire.Slot}, nil
		case "task_group_completed":
			var wire wireTaskGroupCompletedSignalSource
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return TaskGroupCompletedSignalSource{Slot: wire.Slot}, nil
		default:
			return nil, newDecodeError(path, fmt.Sprintf("unsupported signal source kind %q", kind), nil)
		}
	})
}

// --- SignalPattern (ordinary struct, not a closed interface) ---

type wireSignalBinding struct {
	Field string `json:"field"`
	Name  string `json:"name"`
}

type wireSignalPattern struct {
	Source   json.RawMessage     `json:"source"`
	Bindings []wireSignalBinding `json:"bindings"`
}

func encodeSignalBindings(bindings []SignalBinding) []wireSignalBinding {
	if bindings == nil {
		return nil
	}
	result := make([]wireSignalBinding, len(bindings))
	for i, b := range bindings {
		result[i] = wireSignalBinding{Field: b.Field, Name: b.Name}
	}
	return result
}

func decodeSignalBindings(bindings []wireSignalBinding) []SignalBinding {
	if bindings == nil {
		return nil
	}
	result := make([]SignalBinding, len(bindings))
	for i, b := range bindings {
		result[i] = SignalBinding{Field: b.Field, Name: b.Name}
	}
	return result
}

// encodeSignalPattern encodes value, an ordinary (non-interface) struct,
// as its JSON wire representation.
func encodeSignalPattern(path string, value SignalPattern) (json.RawMessage, error) {
	source, err := encodeSignalSource(pathField(path, "source"), value.Source)
	if err != nil {
		return nil, err
	}
	return json.Marshal(wireSignalPattern{Source: source, Bindings: encodeSignalBindings(value.Bindings)})
}

// decodeSignalPattern decodes data as a SignalPattern. Because
// SignalPattern is an ordinary struct rather than a closed interface, JSON
// null is not a valid encoding of it and produces a path-aware structural
// error instead of a silent zero value.
func decodeSignalPattern(path string, data json.RawMessage) (SignalPattern, error) {
	if isEmptyOrNull(data) {
		return SignalPattern{}, newDecodeError(path, "expected a signal pattern object, got null or missing value", nil)
	}
	raw, err := decodeTopLevelValue(path, data)
	if err != nil {
		return SignalPattern{}, err
	}
	var wire wireSignalPattern
	if err := strictDecodeInto(path, raw, &wire); err != nil {
		return SignalPattern{}, err
	}
	source, err := decodeSignalSource(pathField(path, "source"), wire.Source)
	if err != nil {
		return SignalPattern{}, err
	}
	return SignalPattern{Source: source, Bindings: decodeSignalBindings(wire.Bindings)}, nil
}
