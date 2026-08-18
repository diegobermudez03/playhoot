package codec

import (
	"encoding/json"
	"fmt"

	"github.com/diegobermudez03/playhoot/game/language/v1/program"
)

// --- program.Block (ordinary struct, not a closed interface) ---

type wireBlock struct {
	Operations []json.RawMessage `json:"operations"`
}

func encodeOperationSlice(path string, items []program.Operation) ([]json.RawMessage, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]json.RawMessage, len(items))
	for i, item := range items {
		raw, err := encodeOperation(pathIndex(path, i), item)
		if err != nil {
			return nil, err
		}
		result[i] = raw
	}
	return result, nil
}

func decodeOperationSlice(path string, items []json.RawMessage) ([]program.Operation, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]program.Operation, len(items))
	for i, raw := range items {
		item, err := decodeOperation(pathIndex(path, i), raw)
		if err != nil {
			return nil, err
		}
		result[i] = item
	}
	return result, nil
}

// encodeBlock encodes block, an ordinary (non-interface) struct, as its
// JSON wire representation.
func encodeBlock(path string, block program.Block) (json.RawMessage, error) {
	operations, err := encodeOperationSlice(pathField(path, "operations"), block.Operations)
	if err != nil {
		return nil, err
	}
	return json.Marshal(wireBlock{Operations: operations})
}

// decodeBlock decodes data as a program.Block. Because program.Block is an ordinary struct
// rather than a closed interface, JSON null is not a valid encoding of it
// and produces a path-aware structural error instead of a silent zero
// value.
func decodeBlock(path string, data json.RawMessage) (program.Block, error) {
	if isEmptyOrNull(data) {
		return program.Block{}, newDecodeError(path, "expected a block object, got null or missing value", nil)
	}
	raw, err := decodeTopLevelValue(path, data)
	if err != nil {
		return program.Block{}, err
	}
	var wire wireBlock
	if err := strictDecodeInto(path, raw, &wire); err != nil {
		return program.Block{}, err
	}
	operations, err := decodeOperationSlice(pathField(path, "operations"), wire.Operations)
	if err != nil {
		return program.Block{}, err
	}
	return program.Block{Operations: operations}, nil
}

// --- wire structs for every program.Operation variant ---

type wireLetOperation struct {
	Kind  string          `json:"kind"`
	Name  string          `json:"name"`
	Type  json.RawMessage `json:"type"`
	Value json.RawMessage `json:"value"`
}

type wireSetOperation struct {
	Kind   string          `json:"kind"`
	Target json.RawMessage `json:"target"`
	Value  json.RawMessage `json:"value"`
}

type wireListAppendOperation struct {
	Kind   string          `json:"kind"`
	Target json.RawMessage `json:"target"`
	Value  json.RawMessage `json:"value"`
}

type wireListInsertOperation struct {
	Kind   string          `json:"kind"`
	Target json.RawMessage `json:"target"`
	Index  json.RawMessage `json:"index"`
	Value  json.RawMessage `json:"value"`
}

type wireListRemoveAtOperation struct {
	Kind   string          `json:"kind"`
	Target json.RawMessage `json:"target"`
	Index  json.RawMessage `json:"index"`
}

type wireMapPutOperation struct {
	Kind   string          `json:"kind"`
	Target json.RawMessage `json:"target"`
	Key    json.RawMessage `json:"key"`
	Value  json.RawMessage `json:"value"`
}

type wireMapDeleteOperation struct {
	Kind   string          `json:"kind"`
	Target json.RawMessage `json:"target"`
	Key    json.RawMessage `json:"key"`
}

type wireIfOperation struct {
	Kind      string          `json:"kind"`
	Condition json.RawMessage `json:"condition"`
	Then      json.RawMessage `json:"then"`
	Else      json.RawMessage `json:"else"`
}

type wireForEachOperation struct {
	Kind       string          `json:"kind"`
	Collection json.RawMessage `json:"collection"`
	ItemName   string          `json:"item_name"`
	IndexName  string          `json:"index_name"`
	Body       json.RawMessage `json:"body"`
}

type wireMatchOperationCase struct {
	Pattern json.RawMessage `json:"pattern"`
	Body    json.RawMessage `json:"body"`
}

type wireMatchOperation struct {
	Kind  string                   `json:"kind"`
	Value json.RawMessage          `json:"value"`
	Cases []wireMatchOperationCase `json:"cases"`
}

type wireOpenQuestionOperation struct {
	Kind      string             `json:"kind"`
	Slot      string             `json:"slot"`
	Recipient json.RawMessage    `json:"recipient"`
	Arguments []wireCallArgument `json:"arguments"`
}

type wireCloseQuestionOperation struct {
	Kind string `json:"kind"`
	Slot string `json:"slot"`
}

type wireEmitEffectOperation struct {
	Kind       string             `json:"kind"`
	Effect     string             `json:"effect"`
	Recipients json.RawMessage    `json:"recipients"`
	Arguments  []wireCallArgument `json:"arguments"`
}

type wireScheduleTimerOperation struct {
	Kind              string          `json:"kind"`
	Slot              string          `json:"slot"`
	DelayMilliseconds json.RawMessage `json:"delay_milliseconds"`
}

type wireCancelTimerOperation struct {
	Kind string `json:"kind"`
	Slot string `json:"slot"`
}

type wireSpawnChildWorkflowOperation struct {
	Kind      string             `json:"kind"`
	Slot      string             `json:"slot"`
	Arguments []wireCallArgument `json:"arguments"`
}

type wireCancelChildWorkflowOperation struct {
	Kind   string          `json:"kind"`
	Slot   string          `json:"slot"`
	Reason json.RawMessage `json:"reason"`
}

type wireOpenAskGroupOperation struct {
	Kind       string             `json:"kind"`
	Slot       string             `json:"slot"`
	Recipients json.RawMessage    `json:"recipients"`
	Arguments  []wireCallArgument `json:"arguments"`
	Completion json.RawMessage    `json:"completion"`
}

type wireFinalizeAskGroupOperation struct {
	Kind string `json:"kind"`
	Slot string `json:"slot"`
}

type wireCancelAskGroupOperation struct {
	Kind string `json:"kind"`
	Slot string `json:"slot"`
}

type wireBeginTaskGroupOperation struct {
	Kind       string          `json:"kind"`
	Slot       string          `json:"slot"`
	Completion json.RawMessage `json:"completion"`
}

type wireSpawnTaskGroupChildOperation struct {
	Kind      string             `json:"kind"`
	Slot      string             `json:"slot"`
	Key       json.RawMessage    `json:"key"`
	Arguments []wireCallArgument `json:"arguments"`
}

type wireSealTaskGroupOperation struct {
	Kind string `json:"kind"`
	Slot string `json:"slot"`
}

type wireFinalizeTaskGroupOperation struct {
	Kind string `json:"kind"`
	Slot string `json:"slot"`
}

type wireCancelTaskGroupOperation struct {
	Kind   string          `json:"kind"`
	Slot   string          `json:"slot"`
	Reason json.RawMessage `json:"reason"`
}

type wireDrawRandomOperation struct {
	Kind      string          `json:"kind"`
	Name      string          `json:"name"`
	Generator json.RawMessage `json:"generator"`
}

// --- match-operation case helpers ---

func encodeMatchOperationCases(path string, cases []program.MatchOperationCase) ([]wireMatchOperationCase, error) {
	if cases == nil {
		return nil, nil
	}
	result := make([]wireMatchOperationCase, len(cases))
	for i, c := range cases {
		itemPath := pathIndex(path, i)
		pattern, err := encodeMatchPattern(pathField(itemPath, "pattern"), c.Pattern)
		if err != nil {
			return nil, err
		}
		body, err := encodeBlock(pathField(itemPath, "body"), c.Body)
		if err != nil {
			return nil, err
		}
		result[i] = wireMatchOperationCase{Pattern: pattern, Body: body}
	}
	return result, nil
}

func decodeMatchOperationCases(path string, cases []wireMatchOperationCase) ([]program.MatchOperationCase, error) {
	if cases == nil {
		return nil, nil
	}
	result := make([]program.MatchOperationCase, len(cases))
	for i, c := range cases {
		itemPath := pathIndex(path, i)
		pattern, err := decodeMatchPattern(pathField(itemPath, "pattern"), c.Pattern)
		if err != nil {
			return nil, err
		}
		body, err := decodeBlock(pathField(itemPath, "body"), c.Body)
		if err != nil {
			return nil, err
		}
		result[i] = program.MatchOperationCase{Pattern: pattern, Body: body}
	}
	return result, nil
}

// encodeOperation encodes value as its JSON wire representation, or as
// JSON null when value is a nil interface or a typed nil pointer.
func encodeOperation(path string, value program.Operation) (json.RawMessage, error) {
	return encodeUnion(value, func(resolved any) (json.RawMessage, error) {
		switch v := resolved.(type) {
		case program.LetOperation:
			typeRef, err := encodeTypeReference(pathField(path, "type"), v.Type)
			if err != nil {
				return nil, err
			}
			value, err := encodeExpression(pathField(path, "value"), v.Value)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireLetOperation{Kind: "let", Name: v.Name, Type: typeRef, Value: value})
		case program.SetOperation:
			target, err := encodeAssignmentTarget(pathField(path, "target"), v.Target)
			if err != nil {
				return nil, err
			}
			value, err := encodeExpression(pathField(path, "value"), v.Value)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireSetOperation{Kind: "set", Target: target, Value: value})
		case program.ListAppendOperation:
			target, err := encodeAssignmentTarget(pathField(path, "target"), v.Target)
			if err != nil {
				return nil, err
			}
			value, err := encodeExpression(pathField(path, "value"), v.Value)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireListAppendOperation{Kind: "list_append", Target: target, Value: value})
		case program.ListInsertOperation:
			target, err := encodeAssignmentTarget(pathField(path, "target"), v.Target)
			if err != nil {
				return nil, err
			}
			index, err := encodeExpression(pathField(path, "index"), v.Index)
			if err != nil {
				return nil, err
			}
			value, err := encodeExpression(pathField(path, "value"), v.Value)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireListInsertOperation{Kind: "list_insert", Target: target, Index: index, Value: value})
		case program.ListRemoveAtOperation:
			target, err := encodeAssignmentTarget(pathField(path, "target"), v.Target)
			if err != nil {
				return nil, err
			}
			index, err := encodeExpression(pathField(path, "index"), v.Index)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireListRemoveAtOperation{Kind: "list_remove_at", Target: target, Index: index})
		case program.MapPutOperation:
			target, err := encodeAssignmentTarget(pathField(path, "target"), v.Target)
			if err != nil {
				return nil, err
			}
			key, err := encodeExpression(pathField(path, "key"), v.Key)
			if err != nil {
				return nil, err
			}
			value, err := encodeExpression(pathField(path, "value"), v.Value)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireMapPutOperation{Kind: "map_put", Target: target, Key: key, Value: value})
		case program.MapDeleteOperation:
			target, err := encodeAssignmentTarget(pathField(path, "target"), v.Target)
			if err != nil {
				return nil, err
			}
			key, err := encodeExpression(pathField(path, "key"), v.Key)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireMapDeleteOperation{Kind: "map_delete", Target: target, Key: key})
		case program.IfOperation:
			condition, err := encodeExpression(pathField(path, "condition"), v.Condition)
			if err != nil {
				return nil, err
			}
			then, err := encodeBlock(pathField(path, "then"), v.Then)
			if err != nil {
				return nil, err
			}
			elseBlock, err := encodeBlock(pathField(path, "else"), v.Else)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireIfOperation{Kind: "if", Condition: condition, Then: then, Else: elseBlock})
		case program.ForEachOperation:
			collection, err := encodeExpression(pathField(path, "collection"), v.Collection)
			if err != nil {
				return nil, err
			}
			body, err := encodeBlock(pathField(path, "body"), v.Body)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireForEachOperation{Kind: "for_each", Collection: collection, ItemName: v.ItemName, IndexName: v.IndexName, Body: body})
		case program.MatchOperation:
			matchValue, err := encodeExpression(pathField(path, "value"), v.Value)
			if err != nil {
				return nil, err
			}
			cases, err := encodeMatchOperationCases(pathField(path, "cases"), v.Cases)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireMatchOperation{Kind: "match", Value: matchValue, Cases: cases})
		case program.OpenQuestionOperation:
			recipient, err := encodeExpression(pathField(path, "recipient"), v.Recipient)
			if err != nil {
				return nil, err
			}
			arguments, err := encodeCallArguments(pathField(path, "arguments"), v.Arguments)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireOpenQuestionOperation{Kind: "open_question", Slot: v.Slot, Recipient: recipient, Arguments: arguments})
		case program.CloseQuestionOperation:
			return json.Marshal(wireCloseQuestionOperation{Kind: "close_question", Slot: v.Slot})
		case program.EmitEffectOperation:
			recipients, err := encodeExpression(pathField(path, "recipients"), v.Recipients)
			if err != nil {
				return nil, err
			}
			arguments, err := encodeCallArguments(pathField(path, "arguments"), v.Arguments)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireEmitEffectOperation{Kind: "emit_effect", Effect: v.Effect, Recipients: recipients, Arguments: arguments})
		case program.ScheduleTimerOperation:
			delay, err := encodeExpression(pathField(path, "delay_milliseconds"), v.DelayMilliseconds)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireScheduleTimerOperation{Kind: "schedule_timer", Slot: v.Slot, DelayMilliseconds: delay})
		case program.CancelTimerOperation:
			return json.Marshal(wireCancelTimerOperation{Kind: "cancel_timer", Slot: v.Slot})
		case program.SpawnChildWorkflowOperation:
			arguments, err := encodeCallArguments(pathField(path, "arguments"), v.Arguments)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireSpawnChildWorkflowOperation{Kind: "spawn_child_workflow", Slot: v.Slot, Arguments: arguments})
		case program.CancelChildWorkflowOperation:
			reason, err := encodeExpression(pathField(path, "reason"), v.Reason)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireCancelChildWorkflowOperation{Kind: "cancel_child_workflow", Slot: v.Slot, Reason: reason})
		case program.OpenAskGroupOperation:
			recipients, err := encodeExpression(pathField(path, "recipients"), v.Recipients)
			if err != nil {
				return nil, err
			}
			arguments, err := encodeCallArguments(pathField(path, "arguments"), v.Arguments)
			if err != nil {
				return nil, err
			}
			completion, err := encodeAskGroupCompletionPolicy(pathField(path, "completion"), v.Completion)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireOpenAskGroupOperation{Kind: "open_ask_group", Slot: v.Slot, Recipients: recipients, Arguments: arguments, Completion: completion})
		case program.FinalizeAskGroupOperation:
			return json.Marshal(wireFinalizeAskGroupOperation{Kind: "finalize_ask_group", Slot: v.Slot})
		case program.CancelAskGroupOperation:
			return json.Marshal(wireCancelAskGroupOperation{Kind: "cancel_ask_group", Slot: v.Slot})
		case program.BeginTaskGroupOperation:
			completion, err := encodeTaskGroupCompletionPolicy(pathField(path, "completion"), v.Completion)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireBeginTaskGroupOperation{Kind: "begin_task_group", Slot: v.Slot, Completion: completion})
		case program.SpawnTaskGroupChildOperation:
			key, err := encodeExpression(pathField(path, "key"), v.Key)
			if err != nil {
				return nil, err
			}
			arguments, err := encodeCallArguments(pathField(path, "arguments"), v.Arguments)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireSpawnTaskGroupChildOperation{Kind: "spawn_task_group_child", Slot: v.Slot, Key: key, Arguments: arguments})
		case program.SealTaskGroupOperation:
			return json.Marshal(wireSealTaskGroupOperation{Kind: "seal_task_group", Slot: v.Slot})
		case program.FinalizeTaskGroupOperation:
			return json.Marshal(wireFinalizeTaskGroupOperation{Kind: "finalize_task_group", Slot: v.Slot})
		case program.CancelTaskGroupOperation:
			reason, err := encodeExpression(pathField(path, "reason"), v.Reason)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireCancelTaskGroupOperation{Kind: "cancel_task_group", Slot: v.Slot, Reason: reason})
		case program.DrawRandomOperation:
			generator, err := encodeRandomGenerator(pathField(path, "generator"), v.Generator)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireDrawRandomOperation{Kind: "draw_random", Name: v.Name, Generator: generator})
		default:
			return nil, fmt.Errorf("%s: unsupported program.Operation implementation %T", path, value)
		}
	})
}

// decodeOperation decodes data as an program.Operation, or returns a nil interface
// for JSON null or a missing value.
func decodeOperation(path string, data json.RawMessage) (program.Operation, error) {
	return decodeUnion(path, data, func(path, kind string, raw json.RawMessage) (program.Operation, error) {
		switch kind {
		case "let":
			var wire wireLetOperation
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			typeRef, err := decodeTypeReference(pathField(path, "type"), wire.Type)
			if err != nil {
				return nil, err
			}
			value, err := decodeExpression(pathField(path, "value"), wire.Value)
			if err != nil {
				return nil, err
			}
			return program.LetOperation{Name: wire.Name, Type: typeRef, Value: value}, nil
		case "set":
			var wire wireSetOperation
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
			return program.SetOperation{Target: target, Value: value}, nil
		case "list_append":
			var wire wireListAppendOperation
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
			return program.ListAppendOperation{Target: target, Value: value}, nil
		case "list_insert":
			var wire wireListInsertOperation
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			target, err := decodeAssignmentTarget(pathField(path, "target"), wire.Target)
			if err != nil {
				return nil, err
			}
			index, err := decodeExpression(pathField(path, "index"), wire.Index)
			if err != nil {
				return nil, err
			}
			value, err := decodeExpression(pathField(path, "value"), wire.Value)
			if err != nil {
				return nil, err
			}
			return program.ListInsertOperation{Target: target, Index: index, Value: value}, nil
		case "list_remove_at":
			var wire wireListRemoveAtOperation
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			target, err := decodeAssignmentTarget(pathField(path, "target"), wire.Target)
			if err != nil {
				return nil, err
			}
			index, err := decodeExpression(pathField(path, "index"), wire.Index)
			if err != nil {
				return nil, err
			}
			return program.ListRemoveAtOperation{Target: target, Index: index}, nil
		case "map_put":
			var wire wireMapPutOperation
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			target, err := decodeAssignmentTarget(pathField(path, "target"), wire.Target)
			if err != nil {
				return nil, err
			}
			key, err := decodeExpression(pathField(path, "key"), wire.Key)
			if err != nil {
				return nil, err
			}
			value, err := decodeExpression(pathField(path, "value"), wire.Value)
			if err != nil {
				return nil, err
			}
			return program.MapPutOperation{Target: target, Key: key, Value: value}, nil
		case "map_delete":
			var wire wireMapDeleteOperation
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			target, err := decodeAssignmentTarget(pathField(path, "target"), wire.Target)
			if err != nil {
				return nil, err
			}
			key, err := decodeExpression(pathField(path, "key"), wire.Key)
			if err != nil {
				return nil, err
			}
			return program.MapDeleteOperation{Target: target, Key: key}, nil
		case "if":
			var wire wireIfOperation
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			condition, err := decodeExpression(pathField(path, "condition"), wire.Condition)
			if err != nil {
				return nil, err
			}
			then, err := decodeBlock(pathField(path, "then"), wire.Then)
			if err != nil {
				return nil, err
			}
			elseBlock, err := decodeBlock(pathField(path, "else"), wire.Else)
			if err != nil {
				return nil, err
			}
			return program.IfOperation{Condition: condition, Then: then, Else: elseBlock}, nil
		case "for_each":
			var wire wireForEachOperation
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			collection, err := decodeExpression(pathField(path, "collection"), wire.Collection)
			if err != nil {
				return nil, err
			}
			body, err := decodeBlock(pathField(path, "body"), wire.Body)
			if err != nil {
				return nil, err
			}
			return program.ForEachOperation{Collection: collection, ItemName: wire.ItemName, IndexName: wire.IndexName, Body: body}, nil
		case "match":
			var wire wireMatchOperation
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			matchValue, err := decodeExpression(pathField(path, "value"), wire.Value)
			if err != nil {
				return nil, err
			}
			cases, err := decodeMatchOperationCases(pathField(path, "cases"), wire.Cases)
			if err != nil {
				return nil, err
			}
			return program.MatchOperation{Value: matchValue, Cases: cases}, nil
		case "open_question":
			var wire wireOpenQuestionOperation
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			recipient, err := decodeExpression(pathField(path, "recipient"), wire.Recipient)
			if err != nil {
				return nil, err
			}
			arguments, err := decodeCallArguments(pathField(path, "arguments"), wire.Arguments)
			if err != nil {
				return nil, err
			}
			return program.OpenQuestionOperation{Slot: wire.Slot, Recipient: recipient, Arguments: arguments}, nil
		case "close_question":
			var wire wireCloseQuestionOperation
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return program.CloseQuestionOperation{Slot: wire.Slot}, nil
		case "emit_effect":
			var wire wireEmitEffectOperation
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			recipients, err := decodeExpression(pathField(path, "recipients"), wire.Recipients)
			if err != nil {
				return nil, err
			}
			arguments, err := decodeCallArguments(pathField(path, "arguments"), wire.Arguments)
			if err != nil {
				return nil, err
			}
			return program.EmitEffectOperation{Effect: wire.Effect, Recipients: recipients, Arguments: arguments}, nil
		case "schedule_timer":
			var wire wireScheduleTimerOperation
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			delay, err := decodeExpression(pathField(path, "delay_milliseconds"), wire.DelayMilliseconds)
			if err != nil {
				return nil, err
			}
			return program.ScheduleTimerOperation{Slot: wire.Slot, DelayMilliseconds: delay}, nil
		case "cancel_timer":
			var wire wireCancelTimerOperation
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return program.CancelTimerOperation{Slot: wire.Slot}, nil
		case "spawn_child_workflow":
			var wire wireSpawnChildWorkflowOperation
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			arguments, err := decodeCallArguments(pathField(path, "arguments"), wire.Arguments)
			if err != nil {
				return nil, err
			}
			return program.SpawnChildWorkflowOperation{Slot: wire.Slot, Arguments: arguments}, nil
		case "cancel_child_workflow":
			var wire wireCancelChildWorkflowOperation
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			reason, err := decodeExpression(pathField(path, "reason"), wire.Reason)
			if err != nil {
				return nil, err
			}
			return program.CancelChildWorkflowOperation{Slot: wire.Slot, Reason: reason}, nil
		case "open_ask_group":
			var wire wireOpenAskGroupOperation
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			recipients, err := decodeExpression(pathField(path, "recipients"), wire.Recipients)
			if err != nil {
				return nil, err
			}
			arguments, err := decodeCallArguments(pathField(path, "arguments"), wire.Arguments)
			if err != nil {
				return nil, err
			}
			completion, err := decodeAskGroupCompletionPolicy(pathField(path, "completion"), wire.Completion)
			if err != nil {
				return nil, err
			}
			return program.OpenAskGroupOperation{Slot: wire.Slot, Recipients: recipients, Arguments: arguments, Completion: completion}, nil
		case "finalize_ask_group":
			var wire wireFinalizeAskGroupOperation
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return program.FinalizeAskGroupOperation{Slot: wire.Slot}, nil
		case "cancel_ask_group":
			var wire wireCancelAskGroupOperation
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return program.CancelAskGroupOperation{Slot: wire.Slot}, nil
		case "begin_task_group":
			var wire wireBeginTaskGroupOperation
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			completion, err := decodeTaskGroupCompletionPolicy(pathField(path, "completion"), wire.Completion)
			if err != nil {
				return nil, err
			}
			return program.BeginTaskGroupOperation{Slot: wire.Slot, Completion: completion}, nil
		case "spawn_task_group_child":
			var wire wireSpawnTaskGroupChildOperation
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			key, err := decodeExpression(pathField(path, "key"), wire.Key)
			if err != nil {
				return nil, err
			}
			arguments, err := decodeCallArguments(pathField(path, "arguments"), wire.Arguments)
			if err != nil {
				return nil, err
			}
			return program.SpawnTaskGroupChildOperation{Slot: wire.Slot, Key: key, Arguments: arguments}, nil
		case "seal_task_group":
			var wire wireSealTaskGroupOperation
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return program.SealTaskGroupOperation{Slot: wire.Slot}, nil
		case "finalize_task_group":
			var wire wireFinalizeTaskGroupOperation
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return program.FinalizeTaskGroupOperation{Slot: wire.Slot}, nil
		case "cancel_task_group":
			var wire wireCancelTaskGroupOperation
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			reason, err := decodeExpression(pathField(path, "reason"), wire.Reason)
			if err != nil {
				return nil, err
			}
			return program.CancelTaskGroupOperation{Slot: wire.Slot, Reason: reason}, nil
		case "draw_random":
			var wire wireDrawRandomOperation
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			generator, err := decodeRandomGenerator(pathField(path, "generator"), wire.Generator)
			if err != nil {
				return nil, err
			}
			return program.DrawRandomOperation{Name: wire.Name, Generator: generator}, nil
		default:
			return nil, newDecodeError(path, fmt.Sprintf("unsupported operation kind %q", kind), nil)
		}
	})
}
