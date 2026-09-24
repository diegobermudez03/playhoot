package codec

import (
	"encoding/json"
	"fmt"

	"github.com/diegobermudez03/playhoot/game/language/v1/program"
)

// --- program.AskGroupCompletionPolicy ---

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
func encodeAskGroupCompletionPolicy(path string, value program.AskGroupCompletionPolicy) (json.RawMessage, error) {
	return encodeUnion(value, func(resolved any) (json.RawMessage, error) {
		switch v := resolved.(type) {
		case program.AskGroupAllResponsesPolicy:
			return json.Marshal(wireAskGroupAllResponsesPolicy{Kind: "all_responses"})
		case program.AskGroupFirstResponsePolicy:
			return json.Marshal(wireAskGroupFirstResponsePolicy{Kind: "first_response"})
		case program.AskGroupQuorumPolicy:
			count, err := encodeExpression(pathField(path, "count"), v.Count)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireAskGroupQuorumPolicy{Kind: "quorum", Count: count})
		default:
			return nil, fmt.Errorf("%s: unsupported program.AskGroupCompletionPolicy implementation %T", path, value)
		}
	})
}

// decodeAskGroupCompletionPolicy decodes data as an
// program.AskGroupCompletionPolicy, or returns a nil interface for JSON null or a
// missing value.
func decodeAskGroupCompletionPolicy(path string, data json.RawMessage) (program.AskGroupCompletionPolicy, error) {
	return decodeUnion(path, data, func(path, kind string, raw json.RawMessage) (program.AskGroupCompletionPolicy, error) {
		switch kind {
		case "all_responses":
			var wire wireAskGroupAllResponsesPolicy
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return program.AskGroupAllResponsesPolicy{}, nil
		case "first_response":
			var wire wireAskGroupFirstResponsePolicy
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return program.AskGroupFirstResponsePolicy{}, nil
		case "quorum":
			var wire wireAskGroupQuorumPolicy
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			count, err := decodeExpression(pathField(path, "count"), wire.Count)
			if err != nil {
				return nil, err
			}
			return program.AskGroupQuorumPolicy{Count: count}, nil
		default:
			return nil, newDecodeError(path, fmt.Sprintf("unsupported ask-group completion policy kind %q", kind), nil)
		}
	})
}
