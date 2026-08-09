package program

import (
	"encoding/json"
	"fmt"
)

// --- AskGroupCompletionPolicy ---

type wireAskGroupAllResponsesPolicy struct {
	Kind string `json:"kind"`
}

type wireAskGroupFirstResponsePolicy struct {
	Kind string `json:"kind"`
}

type wireAskGroupQuorumPolicy struct {
	Kind  string          `json:"kind"`
	Count json.RawMessage `json:"count"`
}

// encodeAskGroupCompletionPolicy encodes value as its JSON wire
// representation, or as JSON null when value is a nil interface or a
// typed nil pointer.
func encodeAskGroupCompletionPolicy(path string, value AskGroupCompletionPolicy) (json.RawMessage, error) {
	return encodeUnion(value, func(resolved any) (json.RawMessage, error) {
		switch v := resolved.(type) {
		case AskGroupAllResponsesPolicy:
			return json.Marshal(wireAskGroupAllResponsesPolicy{Kind: "all_responses"})
		case AskGroupFirstResponsePolicy:
			return json.Marshal(wireAskGroupFirstResponsePolicy{Kind: "first_response"})
		case AskGroupQuorumPolicy:
			count, err := encodeExpression(pathField(path, "count"), v.Count)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireAskGroupQuorumPolicy{Kind: "quorum", Count: count})
		default:
			return nil, fmt.Errorf("%s: unsupported AskGroupCompletionPolicy implementation %T", path, value)
		}
	})
}

// decodeAskGroupCompletionPolicy decodes data as an
// AskGroupCompletionPolicy, or returns a nil interface for JSON null or a
// missing value.
func decodeAskGroupCompletionPolicy(path string, data json.RawMessage) (AskGroupCompletionPolicy, error) {
	return decodeUnion(path, data, func(path, kind string, raw json.RawMessage) (AskGroupCompletionPolicy, error) {
		switch kind {
		case "all_responses":
			var wire wireAskGroupAllResponsesPolicy
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return AskGroupAllResponsesPolicy{}, nil
		case "first_response":
			var wire wireAskGroupFirstResponsePolicy
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return AskGroupFirstResponsePolicy{}, nil
		case "quorum":
			var wire wireAskGroupQuorumPolicy
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			count, err := decodeExpression(pathField(path, "count"), wire.Count)
			if err != nil {
				return nil, err
			}
			return AskGroupQuorumPolicy{Count: count}, nil
		default:
			return nil, newDecodeError(path, fmt.Sprintf("unsupported ask-group completion policy kind %q", kind), nil)
		}
	})
}

// --- TaskGroupCompletionPolicy ---

type wireTaskGroupAllTerminalPolicy struct {
	Kind string `json:"kind"`
}

type wireTaskGroupFirstTerminalPolicy struct {
	Kind string `json:"kind"`
}

type wireTaskGroupQuorumTerminalPolicy struct {
	Kind  string          `json:"kind"`
	Count json.RawMessage `json:"count"`
}

// encodeTaskGroupCompletionPolicy encodes value as its JSON wire
// representation, or as JSON null when value is a nil interface or a
// typed nil pointer.
func encodeTaskGroupCompletionPolicy(path string, value TaskGroupCompletionPolicy) (json.RawMessage, error) {
	return encodeUnion(value, func(resolved any) (json.RawMessage, error) {
		switch v := resolved.(type) {
		case TaskGroupAllTerminalPolicy:
			return json.Marshal(wireTaskGroupAllTerminalPolicy{Kind: "all_terminal"})
		case TaskGroupFirstTerminalPolicy:
			return json.Marshal(wireTaskGroupFirstTerminalPolicy{Kind: "first_terminal"})
		case TaskGroupQuorumTerminalPolicy:
			count, err := encodeExpression(pathField(path, "count"), v.Count)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireTaskGroupQuorumTerminalPolicy{Kind: "quorum_terminal", Count: count})
		default:
			return nil, fmt.Errorf("%s: unsupported TaskGroupCompletionPolicy implementation %T", path, value)
		}
	})
}

// decodeTaskGroupCompletionPolicy decodes data as a
// TaskGroupCompletionPolicy, or returns a nil interface for JSON null or a
// missing value.
func decodeTaskGroupCompletionPolicy(path string, data json.RawMessage) (TaskGroupCompletionPolicy, error) {
	return decodeUnion(path, data, func(path, kind string, raw json.RawMessage) (TaskGroupCompletionPolicy, error) {
		switch kind {
		case "all_terminal":
			var wire wireTaskGroupAllTerminalPolicy
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return TaskGroupAllTerminalPolicy{}, nil
		case "first_terminal":
			var wire wireTaskGroupFirstTerminalPolicy
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return TaskGroupFirstTerminalPolicy{}, nil
		case "quorum_terminal":
			var wire wireTaskGroupQuorumTerminalPolicy
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			count, err := decodeExpression(pathField(path, "count"), wire.Count)
			if err != nil {
				return nil, err
			}
			return TaskGroupQuorumTerminalPolicy{Count: count}, nil
		default:
			return nil, newDecodeError(path, fmt.Sprintf("unsupported task-group completion policy kind %q", kind), nil)
		}
	})
}
