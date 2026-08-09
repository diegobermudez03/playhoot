package program

import (
	"encoding/json"
	"fmt"
)

// --- UILayout ---

type wireStackLayout struct {
	Kind string `json:"kind"`
}

type wireAbsoluteLayout struct {
	Kind string `json:"kind"`
}

type wireLinearLayout struct {
	Kind      string                `json:"kind"`
	Direction LinearLayoutDirection `json:"direction"`
	Gap       json.RawMessage       `json:"gap"`
}

type wireGridLayout struct {
	Kind      string          `json:"kind"`
	Columns   json.RawMessage `json:"columns"`
	RowGap    json.RawMessage `json:"row_gap"`
	ColumnGap json.RawMessage `json:"column_gap"`
}

// encodeUILayout encodes value as its JSON wire representation, or as
// JSON null when value is a nil interface or a typed nil pointer.
func encodeUILayout(path string, value UILayout) (json.RawMessage, error) {
	return encodeUnion(value, func(resolved any) (json.RawMessage, error) {
		switch v := resolved.(type) {
		case StackLayout:
			return json.Marshal(wireStackLayout{Kind: "stack"})
		case AbsoluteLayout:
			return json.Marshal(wireAbsoluteLayout{Kind: "absolute"})
		case LinearLayout:
			gap, err := encodeExpression(pathField(path, "gap"), v.Gap)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireLinearLayout{Kind: "linear", Direction: v.Direction, Gap: gap})
		case GridLayout:
			columns, err := encodeExpression(pathField(path, "columns"), v.Columns)
			if err != nil {
				return nil, err
			}
			rowGap, err := encodeExpression(pathField(path, "row_gap"), v.RowGap)
			if err != nil {
				return nil, err
			}
			columnGap, err := encodeExpression(pathField(path, "column_gap"), v.ColumnGap)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireGridLayout{Kind: "grid", Columns: columns, RowGap: rowGap, ColumnGap: columnGap})
		default:
			return nil, fmt.Errorf("%s: unsupported UILayout implementation %T", path, value)
		}
	})
}

// decodeUILayout decodes data as a UILayout, or returns a nil interface
// for JSON null or a missing value.
func decodeUILayout(path string, data json.RawMessage) (UILayout, error) {
	return decodeUnion(path, data, func(path, kind string, raw json.RawMessage) (UILayout, error) {
		switch kind {
		case "stack":
			var wire wireStackLayout
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return StackLayout{}, nil
		case "absolute":
			var wire wireAbsoluteLayout
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return AbsoluteLayout{}, nil
		case "linear":
			var wire wireLinearLayout
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			gap, err := decodeExpression(pathField(path, "gap"), wire.Gap)
			if err != nil {
				return nil, err
			}
			return LinearLayout{Direction: wire.Direction, Gap: gap}, nil
		case "grid":
			var wire wireGridLayout
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			columns, err := decodeExpression(pathField(path, "columns"), wire.Columns)
			if err != nil {
				return nil, err
			}
			rowGap, err := decodeExpression(pathField(path, "row_gap"), wire.RowGap)
			if err != nil {
				return nil, err
			}
			columnGap, err := decodeExpression(pathField(path, "column_gap"), wire.ColumnGap)
			if err != nil {
				return nil, err
			}
			return GridLayout{Columns: columns, RowGap: rowGap, ColumnGap: columnGap}, nil
		default:
			return nil, newDecodeError(path, fmt.Sprintf("unsupported UI layout kind %q", kind), nil)
		}
	})
}

// --- UIAction ---

type wireSetLocalStateAction struct {
	Kind   string          `json:"kind"`
	Target json.RawMessage `json:"target"`
	Value  json.RawMessage `json:"value"`
}

type wireAnswerQuestionAction struct {
	Kind  string          `json:"kind"`
	Value json.RawMessage `json:"value"`
}

type wireEmitUserIntentAction struct {
	Kind      string             `json:"kind"`
	Intent    string             `json:"intent"`
	Arguments []wireCallArgument `json:"arguments"`
}

// encodeUIAction encodes value as its JSON wire representation, or as
// JSON null when value is a nil interface or a typed nil pointer.
func encodeUIAction(path string, value UIAction) (json.RawMessage, error) {
	return encodeUnion(value, func(resolved any) (json.RawMessage, error) {
		switch v := resolved.(type) {
		case SetLocalStateAction:
			target, err := encodeAssignmentTarget(pathField(path, "target"), v.Target)
			if err != nil {
				return nil, err
			}
			value, err := encodeExpression(pathField(path, "value"), v.Value)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireSetLocalStateAction{Kind: "set_local_state", Target: target, Value: value})
		case AnswerQuestionAction:
			value, err := encodeExpression(pathField(path, "value"), v.Value)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireAnswerQuestionAction{Kind: "answer_question", Value: value})
		case EmitUserIntentAction:
			arguments, err := encodeCallArguments(pathField(path, "arguments"), v.Arguments)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireEmitUserIntentAction{Kind: "emit_user_intent", Intent: v.Intent, Arguments: arguments})
		default:
			return nil, fmt.Errorf("%s: unsupported UIAction implementation %T", path, value)
		}
	})
}

// decodeUIAction decodes data as a UIAction, or returns a nil interface
// for JSON null or a missing value.
func decodeUIAction(path string, data json.RawMessage) (UIAction, error) {
	return decodeUnion(path, data, func(path, kind string, raw json.RawMessage) (UIAction, error) {
		switch kind {
		case "set_local_state":
			var wire wireSetLocalStateAction
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			target, err := decodeAssignmentTarget(pathField(path, "target"), wire.Target)
			if err != nil {
				return nil, err
			}
			value, err := decodeExpression(pathField(path, "value"), wire.Value)
			if err != nil {
				return nil, err
			}
			return SetLocalStateAction{Target: target, Value: value}, nil
		case "answer_question":
			var wire wireAnswerQuestionAction
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			value, err := decodeExpression(pathField(path, "value"), wire.Value)
			if err != nil {
				return nil, err
			}
			return AnswerQuestionAction{Value: value}, nil
		case "emit_user_intent":
			var wire wireEmitUserIntentAction
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			arguments, err := decodeCallArguments(pathField(path, "arguments"), wire.Arguments)
			if err != nil {
				return nil, err
			}
			return EmitUserIntentAction{Intent: wire.Intent, Arguments: arguments}, nil
		default:
			return nil, newDecodeError(path, fmt.Sprintf("unsupported UI action kind %q", kind), nil)
		}
	})
}

func encodeUIActionSlice(path string, items []UIAction) ([]json.RawMessage, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]json.RawMessage, len(items))
	for i, item := range items {
		raw, err := encodeUIAction(pathIndex(path, i), item)
		if err != nil {
			return nil, err
		}
		result[i] = raw
	}
	return result, nil
}

func decodeUIActionSlice(path string, items []json.RawMessage) ([]UIAction, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]UIAction, len(items))
	for i, raw := range items {
		item, err := decodeUIAction(pathIndex(path, i), raw)
		if err != nil {
			return nil, err
		}
		result[i] = item
	}
	return result, nil
}

// --- UIEventHandler and UIElementConfiguration (ordinary objects) ---

type wireUIEventHandler struct {
	Event   UIEventType       `json:"event"`
	Actions []json.RawMessage `json:"actions"`
}

func encodeUIEventHandlers(path string, handlers []UIEventHandler) ([]wireUIEventHandler, error) {
	if handlers == nil {
		return nil, nil
	}
	result := make([]wireUIEventHandler, len(handlers))
	for i, h := range handlers {
		itemPath := pathIndex(path, i)
		actions, err := encodeUIActionSlice(pathField(itemPath, "actions"), h.Actions)
		if err != nil {
			return nil, err
		}
		result[i] = wireUIEventHandler{Event: h.Event, Actions: actions}
	}
	return result, nil
}

func decodeUIEventHandlers(path string, handlers []wireUIEventHandler) ([]UIEventHandler, error) {
	if handlers == nil {
		return nil, nil
	}
	result := make([]UIEventHandler, len(handlers))
	for i, h := range handlers {
		itemPath := pathIndex(path, i)
		actions, err := decodeUIActionSlice(pathField(itemPath, "actions"), h.Actions)
		if err != nil {
			return nil, err
		}
		result[i] = UIEventHandler{Event: h.Event, Actions: actions}
	}
	return result, nil
}

type wireUIProperty struct {
	Name  string          `json:"name"`
	Value json.RawMessage `json:"value"`
}

func encodeUIProperties(path string, properties []UIProperty) ([]wireUIProperty, error) {
	if properties == nil {
		return nil, nil
	}
	result := make([]wireUIProperty, len(properties))
	for i, p := range properties {
		itemPath := pathIndex(path, i)
		value, err := encodeExpression(pathField(itemPath, "value"), p.Value)
		if err != nil {
			return nil, err
		}
		result[i] = wireUIProperty{Name: p.Name, Value: value}
	}
	return result, nil
}

func decodeUIProperties(path string, properties []wireUIProperty) ([]UIProperty, error) {
	if properties == nil {
		return nil, nil
	}
	result := make([]UIProperty, len(properties))
	for i, p := range properties {
		itemPath := pathIndex(path, i)
		value, err := decodeExpression(pathField(itemPath, "value"), p.Value)
		if err != nil {
			return nil, err
		}
		result[i] = UIProperty{Name: p.Name, Value: value}
	}
	return result, nil
}

type wireUIElementConfiguration struct {
	Properties []wireUIProperty     `json:"properties"`
	Events     []wireUIEventHandler `json:"events"`
}

// encodeUIElementConfiguration encodes value, an ordinary (non-interface)
// struct, as its JSON wire representation.
func encodeUIElementConfiguration(path string, value UIElementConfiguration) (json.RawMessage, error) {
	properties, err := encodeUIProperties(pathField(path, "properties"), value.Properties)
	if err != nil {
		return nil, err
	}
	events, err := encodeUIEventHandlers(pathField(path, "events"), value.Events)
	if err != nil {
		return nil, err
	}
	return json.Marshal(wireUIElementConfiguration{Properties: properties, Events: events})
}

// decodeUIElementConfiguration decodes data as a UIElementConfiguration.
// Because it is an ordinary struct rather than a closed interface, JSON
// null is not a valid encoding of it and produces a path-aware structural
// error instead of a silent zero value.
func decodeUIElementConfiguration(path string, data json.RawMessage) (UIElementConfiguration, error) {
	if isEmptyOrNull(data) {
		return UIElementConfiguration{}, newDecodeError(path, "expected a UI element configuration object, got null or missing value", nil)
	}
	raw, err := decodeTopLevelValue(path, data)
	if err != nil {
		return UIElementConfiguration{}, err
	}
	var wire wireUIElementConfiguration
	if err := strictDecodeInto(path, raw, &wire); err != nil {
		return UIElementConfiguration{}, err
	}
	properties, err := decodeUIProperties(pathField(path, "properties"), wire.Properties)
	if err != nil {
		return UIElementConfiguration{}, err
	}
	events, err := decodeUIEventHandlers(pathField(path, "events"), wire.Events)
	if err != nil {
		return UIElementConfiguration{}, err
	}
	return UIElementConfiguration{Properties: properties, Events: events}, nil
}
