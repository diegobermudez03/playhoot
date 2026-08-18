package codec

import (
	"encoding/json"

	"github.com/diegobermudez03/playhoot/game/language/v1/program"
)

// --- program.TransitionDeclaration ---

type wireTransitionDeclaration struct {
	Name       string          `json:"name"`
	Signal     json.RawMessage `json:"signal"`
	Guard      json.RawMessage `json:"guard"`
	Operations json.RawMessage `json:"operations"`
	Control    json.RawMessage `json:"control"`
}

// encodeTransitionDeclaration encodes value, an ordinary (non-interface)
// struct, as its JSON wire representation. Signal and Operations always
// encode as ordinary objects, never JSON null; Guard and Control may
// independently be JSON null.
func encodeTransitionDeclaration(path string, value program.TransitionDeclaration) (json.RawMessage, error) {
	signal, err := encodeSignalPattern(pathField(path, "signal"), value.Signal)
	if err != nil {
		return nil, err
	}
	guard, err := encodeExpression(pathField(path, "guard"), value.Guard)
	if err != nil {
		return nil, err
	}
	operations, err := encodeBlock(pathField(path, "operations"), value.Operations)
	if err != nil {
		return nil, err
	}
	control, err := encodeWorkflowControl(pathField(path, "control"), value.Control)
	if err != nil {
		return nil, err
	}
	return json.Marshal(wireTransitionDeclaration{
		Name:       value.Name,
		Signal:     signal,
		Guard:      guard,
		Operations: operations,
		Control:    control,
	})
}

// decodeTransitionDeclaration decodes data as a program.TransitionDeclaration.
// JSON null for the declaration itself is not a valid encoding and
// produces a path-aware structural error; Signal and Operations must be
// ordinary objects (never null), while Guard and Control may
// independently be JSON null.
func decodeTransitionDeclaration(path string, data json.RawMessage) (program.TransitionDeclaration, error) {
	var wire wireTransitionDeclaration
	if err := decodeOrdinaryObject(path, data, &wire); err != nil {
		return program.TransitionDeclaration{}, err
	}
	signal, err := decodeSignalPattern(pathField(path, "signal"), wire.Signal)
	if err != nil {
		return program.TransitionDeclaration{}, err
	}
	guard, err := decodeExpression(pathField(path, "guard"), wire.Guard)
	if err != nil {
		return program.TransitionDeclaration{}, err
	}
	operations, err := decodeBlock(pathField(path, "operations"), wire.Operations)
	if err != nil {
		return program.TransitionDeclaration{}, err
	}
	control, err := decodeWorkflowControl(pathField(path, "control"), wire.Control)
	if err != nil {
		return program.TransitionDeclaration{}, err
	}
	return program.TransitionDeclaration{
		Name:       wire.Name,
		Signal:     signal,
		Guard:      guard,
		Operations: operations,
		Control:    control,
	}, nil
}

func encodeTransitionDeclarations(path string, items []program.TransitionDeclaration) ([]json.RawMessage, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]json.RawMessage, len(items))
	for i, item := range items {
		raw, err := encodeTransitionDeclaration(pathIndex(path, i), item)
		if err != nil {
			return nil, err
		}
		result[i] = raw
	}
	return result, nil
}

func decodeTransitionDeclarations(path string, items []json.RawMessage) ([]program.TransitionDeclaration, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]program.TransitionDeclaration, len(items))
	for i, raw := range items {
		item, err := decodeTransitionDeclaration(pathIndex(path, i), raw)
		if err != nil {
			return nil, err
		}
		result[i] = item
	}
	return result, nil
}

// --- program.WorkflowStateDeclaration ---

type wireWorkflowStateDeclaration struct {
	Name          string            `json:"name"`
	Presentations []json.RawMessage `json:"presentations"`
	Transitions   []json.RawMessage `json:"transitions"`
}

// encodeWorkflowStateDeclaration encodes value, an ordinary (non-interface)
// struct, as its JSON wire representation. This package adds no on_enter,
// on_exit, mount, or unmount fields — the state model remains declarative
// and signal-driven.
func encodeWorkflowStateDeclaration(path string, value program.WorkflowStateDeclaration) (json.RawMessage, error) {
	presentations, err := encodePresentationDeclarations(pathField(path, "presentations"), value.Presentations)
	if err != nil {
		return nil, err
	}
	transitions, err := encodeTransitionDeclarations(pathField(path, "transitions"), value.Transitions)
	if err != nil {
		return nil, err
	}
	return json.Marshal(wireWorkflowStateDeclaration{Name: value.Name, Presentations: presentations, Transitions: transitions})
}

// decodeWorkflowStateDeclaration decodes data as a
// program.WorkflowStateDeclaration. JSON null is not a valid encoding and produces
// a path-aware structural error.
func decodeWorkflowStateDeclaration(path string, data json.RawMessage) (program.WorkflowStateDeclaration, error) {
	var wire wireWorkflowStateDeclaration
	if err := decodeOrdinaryObject(path, data, &wire); err != nil {
		return program.WorkflowStateDeclaration{}, err
	}
	presentations, err := decodePresentationDeclarations(pathField(path, "presentations"), wire.Presentations)
	if err != nil {
		return program.WorkflowStateDeclaration{}, err
	}
	transitions, err := decodeTransitionDeclarations(pathField(path, "transitions"), wire.Transitions)
	if err != nil {
		return program.WorkflowStateDeclaration{}, err
	}
	return program.WorkflowStateDeclaration{Name: wire.Name, Presentations: presentations, Transitions: transitions}, nil
}

func encodeWorkflowStateDeclarations(path string, items []program.WorkflowStateDeclaration) ([]json.RawMessage, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]json.RawMessage, len(items))
	for i, item := range items {
		raw, err := encodeWorkflowStateDeclaration(pathIndex(path, i), item)
		if err != nil {
			return nil, err
		}
		result[i] = raw
	}
	return result, nil
}

func decodeWorkflowStateDeclarations(path string, items []json.RawMessage) ([]program.WorkflowStateDeclaration, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]program.WorkflowStateDeclaration, len(items))
	for i, raw := range items {
		item, err := decodeWorkflowStateDeclaration(pathIndex(path, i), raw)
		if err != nil {
			return nil, err
		}
		result[i] = item
	}
	return result, nil
}

// --- program.WorkflowDeclaration ---

type wireWorkflowDeclaration struct {
	Name              string                 `json:"name"`
	Parameters        []wireFieldDeclaration `json:"parameters"`
	ResultType        json.RawMessage        `json:"result_type"`
	LocalState        json.RawMessage        `json:"local_state"`
	QuestionSlots     []json.RawMessage      `json:"question_slots"`
	AskGroupSlots     []json.RawMessage      `json:"ask_group_slots"`
	TimerSlots        []json.RawMessage      `json:"timer_slots"`
	ChildSlots        []json.RawMessage      `json:"child_slots"`
	TaskGroupSlots    []json.RawMessage      `json:"task_group_slots"`
	Presentations     []json.RawMessage      `json:"presentations"`
	InitialState      string                 `json:"initial_state"`
	GlobalTransitions []json.RawMessage      `json:"global_transitions"`
	States            []json.RawMessage      `json:"states"`
}

// encodeWorkflowDeclaration encodes value, an ordinary (non-interface)
// struct, as its JSON wire representation, using the canonical field order
// name, parameters, result_type, local_state, question_slots,
// ask_group_slots, timer_slots, child_slots, task_group_slots,
// presentations, initial_state, global_transitions, states.
func encodeWorkflowDeclaration(path string, value program.WorkflowDeclaration) (json.RawMessage, error) {
	parameters, err := encodeFieldDeclarations(pathField(path, "parameters"), value.Parameters)
	if err != nil {
		return nil, err
	}
	resultType, err := encodeTypeReference(pathField(path, "result_type"), value.ResultType)
	if err != nil {
		return nil, err
	}
	localState, err := encodeStateDeclaration(pathField(path, "local_state"), value.LocalState)
	if err != nil {
		return nil, err
	}
	questionSlots, err := encodeQuestionSlotDeclarations(pathField(path, "question_slots"), value.QuestionSlots)
	if err != nil {
		return nil, err
	}
	askGroupSlots, err := encodeAskGroupSlotDeclarations(pathField(path, "ask_group_slots"), value.AskGroupSlots)
	if err != nil {
		return nil, err
	}
	timerSlots, err := encodeTimerSlotDeclarations(pathField(path, "timer_slots"), value.TimerSlots)
	if err != nil {
		return nil, err
	}
	childSlots, err := encodeChildWorkflowSlotDeclarations(pathField(path, "child_slots"), value.ChildSlots)
	if err != nil {
		return nil, err
	}
	taskGroupSlots, err := encodeTaskGroupSlotDeclarations(pathField(path, "task_group_slots"), value.TaskGroupSlots)
	if err != nil {
		return nil, err
	}
	presentations, err := encodePresentationDeclarations(pathField(path, "presentations"), value.Presentations)
	if err != nil {
		return nil, err
	}
	globalTransitions, err := encodeTransitionDeclarations(pathField(path, "global_transitions"), value.GlobalTransitions)
	if err != nil {
		return nil, err
	}
	states, err := encodeWorkflowStateDeclarations(pathField(path, "states"), value.States)
	if err != nil {
		return nil, err
	}
	return json.Marshal(wireWorkflowDeclaration{
		Name:              value.Name,
		Parameters:        parameters,
		ResultType:        resultType,
		LocalState:        localState,
		QuestionSlots:     questionSlots,
		AskGroupSlots:     askGroupSlots,
		TimerSlots:        timerSlots,
		ChildSlots:        childSlots,
		TaskGroupSlots:    taskGroupSlots,
		Presentations:     presentations,
		InitialState:      value.InitialState,
		GlobalTransitions: globalTransitions,
		States:            states,
	})
}

// decodeWorkflowDeclaration decodes data as a program.WorkflowDeclaration. JSON
// null is not a valid encoding and produces a path-aware structural
// error; ResultType may independently be JSON null, and LocalState is
// always an ordinary state-declaration object.
func decodeWorkflowDeclaration(path string, data json.RawMessage) (program.WorkflowDeclaration, error) {
	var wire wireWorkflowDeclaration
	if err := decodeOrdinaryObject(path, data, &wire); err != nil {
		return program.WorkflowDeclaration{}, err
	}
	parameters, err := decodeFieldDeclarations(pathField(path, "parameters"), wire.Parameters)
	if err != nil {
		return program.WorkflowDeclaration{}, err
	}
	resultType, err := decodeTypeReference(pathField(path, "result_type"), wire.ResultType)
	if err != nil {
		return program.WorkflowDeclaration{}, err
	}
	localState, err := decodeStateDeclaration(pathField(path, "local_state"), wire.LocalState)
	if err != nil {
		return program.WorkflowDeclaration{}, err
	}
	questionSlots, err := decodeQuestionSlotDeclarations(pathField(path, "question_slots"), wire.QuestionSlots)
	if err != nil {
		return program.WorkflowDeclaration{}, err
	}
	askGroupSlots, err := decodeAskGroupSlotDeclarations(pathField(path, "ask_group_slots"), wire.AskGroupSlots)
	if err != nil {
		return program.WorkflowDeclaration{}, err
	}
	timerSlots, err := decodeTimerSlotDeclarations(pathField(path, "timer_slots"), wire.TimerSlots)
	if err != nil {
		return program.WorkflowDeclaration{}, err
	}
	childSlots, err := decodeChildWorkflowSlotDeclarations(pathField(path, "child_slots"), wire.ChildSlots)
	if err != nil {
		return program.WorkflowDeclaration{}, err
	}
	taskGroupSlots, err := decodeTaskGroupSlotDeclarations(pathField(path, "task_group_slots"), wire.TaskGroupSlots)
	if err != nil {
		return program.WorkflowDeclaration{}, err
	}
	presentations, err := decodePresentationDeclarations(pathField(path, "presentations"), wire.Presentations)
	if err != nil {
		return program.WorkflowDeclaration{}, err
	}
	globalTransitions, err := decodeTransitionDeclarations(pathField(path, "global_transitions"), wire.GlobalTransitions)
	if err != nil {
		return program.WorkflowDeclaration{}, err
	}
	states, err := decodeWorkflowStateDeclarations(pathField(path, "states"), wire.States)
	if err != nil {
		return program.WorkflowDeclaration{}, err
	}
	return program.WorkflowDeclaration{
		Name:              wire.Name,
		Parameters:        parameters,
		ResultType:        resultType,
		LocalState:        localState,
		QuestionSlots:     questionSlots,
		AskGroupSlots:     askGroupSlots,
		TimerSlots:        timerSlots,
		ChildSlots:        childSlots,
		TaskGroupSlots:    taskGroupSlots,
		Presentations:     presentations,
		InitialState:      wire.InitialState,
		GlobalTransitions: globalTransitions,
		States:            states,
	}, nil
}

func encodeWorkflowDeclarations(path string, items []program.WorkflowDeclaration) ([]json.RawMessage, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]json.RawMessage, len(items))
	for i, item := range items {
		raw, err := encodeWorkflowDeclaration(pathIndex(path, i), item)
		if err != nil {
			return nil, err
		}
		result[i] = raw
	}
	return result, nil
}

func decodeWorkflowDeclarations(path string, items []json.RawMessage) ([]program.WorkflowDeclaration, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]program.WorkflowDeclaration, len(items))
	for i, raw := range items {
		item, err := decodeWorkflowDeclaration(pathIndex(path, i), raw)
		if err != nil {
			return nil, err
		}
		result[i] = item
	}
	return result, nil
}
