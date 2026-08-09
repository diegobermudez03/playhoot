package codec

import (
	"encoding/json"
	"fmt"

	"github.com/diegobermudez03/playhoot/game/v1/program"
)

type wireGotoControl struct {
	Kind  string `json:"kind"`
	State string `json:"state"`
}

type wireStayControl struct {
	Kind string `json:"kind"`
}

type wireCompleteControl struct {
	Kind   string          `json:"kind"`
	Result json.RawMessage `json:"result"`
}

type wireFailControl struct {
	Kind  string          `json:"kind"`
	Error json.RawMessage `json:"error"`
}

type wireCancelControl struct {
	Kind   string          `json:"kind"`
	Reason json.RawMessage `json:"reason"`
}

type wireConditionalControl struct {
	Kind      string          `json:"kind"`
	Condition json.RawMessage `json:"condition"`
	Then      json.RawMessage `json:"then"`
	Else      json.RawMessage `json:"else"`
}

type wireMatchControlCase struct {
	Pattern json.RawMessage `json:"pattern"`
	Control json.RawMessage `json:"control"`
}

type wireMatchControl struct {
	Kind  string                 `json:"kind"`
	Value json.RawMessage        `json:"value"`
	Cases []wireMatchControlCase `json:"cases"`
}

func encodeMatchControlCases(path string, cases []program.MatchControlCase) ([]wireMatchControlCase, error) {
	if cases == nil {
		return nil, nil
	}
	result := make([]wireMatchControlCase, len(cases))
	for i, c := range cases {
		itemPath := pathIndex(path, i)
		pattern, err := encodeMatchPattern(pathField(itemPath, "pattern"), c.Pattern)
		if err != nil {
			return nil, err
		}
		control, err := encodeWorkflowControl(pathField(itemPath, "control"), c.Control)
		if err != nil {
			return nil, err
		}
		result[i] = wireMatchControlCase{Pattern: pattern, Control: control}
	}
	return result, nil
}

func decodeMatchControlCases(path string, cases []wireMatchControlCase) ([]program.MatchControlCase, error) {
	if cases == nil {
		return nil, nil
	}
	result := make([]program.MatchControlCase, len(cases))
	for i, c := range cases {
		itemPath := pathIndex(path, i)
		pattern, err := decodeMatchPattern(pathField(itemPath, "pattern"), c.Pattern)
		if err != nil {
			return nil, err
		}
		control, err := decodeWorkflowControl(pathField(itemPath, "control"), c.Control)
		if err != nil {
			return nil, err
		}
		result[i] = program.MatchControlCase{Pattern: pattern, Control: control}
	}
	return result, nil
}

// encodeWorkflowControl encodes value as its JSON wire representation, or
// as JSON null when value is a nil interface or a typed nil pointer.
func encodeWorkflowControl(path string, value program.WorkflowControl) (json.RawMessage, error) {
	return encodeUnion(value, func(resolved any) (json.RawMessage, error) {
		switch v := resolved.(type) {
		case program.GotoControl:
			return json.Marshal(wireGotoControl{Kind: "goto", State: v.State})
		case program.StayControl:
			return json.Marshal(wireStayControl{Kind: "stay"})
		case program.CompleteControl:
			result, err := encodeExpression(pathField(path, "result"), v.Result)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireCompleteControl{Kind: "complete", Result: result})
		case program.FailControl:
			errExpr, err := encodeExpression(pathField(path, "error"), v.Error)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireFailControl{Kind: "fail", Error: errExpr})
		case program.CancelControl:
			reason, err := encodeExpression(pathField(path, "reason"), v.Reason)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireCancelControl{Kind: "cancel", Reason: reason})
		case program.ConditionalControl:
			condition, err := encodeExpression(pathField(path, "condition"), v.Condition)
			if err != nil {
				return nil, err
			}
			then, err := encodeWorkflowControl(pathField(path, "then"), v.Then)
			if err != nil {
				return nil, err
			}
			elseControl, err := encodeWorkflowControl(pathField(path, "else"), v.Else)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireConditionalControl{Kind: "conditional", Condition: condition, Then: then, Else: elseControl})
		case program.MatchControl:
			matchValue, err := encodeExpression(pathField(path, "value"), v.Value)
			if err != nil {
				return nil, err
			}
			cases, err := encodeMatchControlCases(pathField(path, "cases"), v.Cases)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireMatchControl{Kind: "match", Value: matchValue, Cases: cases})
		default:
			return nil, fmt.Errorf("%s: unsupported program.WorkflowControl implementation %T", path, value)
		}
	})
}

// decodeWorkflowControl decodes data as a program.WorkflowControl, or returns a
// nil interface for JSON null or a missing value.
func decodeWorkflowControl(path string, data json.RawMessage) (program.WorkflowControl, error) {
	return decodeUnion(path, data, func(path, kind string, raw json.RawMessage) (program.WorkflowControl, error) {
		switch kind {
		case "goto":
			var wire wireGotoControl
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return program.GotoControl{State: wire.State}, nil
		case "stay":
			var wire wireStayControl
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return program.StayControl{}, nil
		case "complete":
			var wire wireCompleteControl
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			result, err := decodeExpression(pathField(path, "result"), wire.Result)
			if err != nil {
				return nil, err
			}
			return program.CompleteControl{Result: result}, nil
		case "fail":
			var wire wireFailControl
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			errExpr, err := decodeExpression(pathField(path, "error"), wire.Error)
			if err != nil {
				return nil, err
			}
			return program.FailControl{Error: errExpr}, nil
		case "cancel":
			var wire wireCancelControl
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			reason, err := decodeExpression(pathField(path, "reason"), wire.Reason)
			if err != nil {
				return nil, err
			}
			return program.CancelControl{Reason: reason}, nil
		case "conditional":
			var wire wireConditionalControl
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			condition, err := decodeExpression(pathField(path, "condition"), wire.Condition)
			if err != nil {
				return nil, err
			}
			then, err := decodeWorkflowControl(pathField(path, "then"), wire.Then)
			if err != nil {
				return nil, err
			}
			elseControl, err := decodeWorkflowControl(pathField(path, "else"), wire.Else)
			if err != nil {
				return nil, err
			}
			return program.ConditionalControl{Condition: condition, Then: then, Else: elseControl}, nil
		case "match":
			var wire wireMatchControl
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			matchValue, err := decodeExpression(pathField(path, "value"), wire.Value)
			if err != nil {
				return nil, err
			}
			cases, err := decodeMatchControlCases(pathField(path, "cases"), wire.Cases)
			if err != nil {
				return nil, err
			}
			return program.MatchControl{Value: matchValue, Cases: cases}, nil
		default:
			return nil, newDecodeError(path, fmt.Sprintf("unsupported workflow control kind %q", kind), nil)
		}
	})
}
