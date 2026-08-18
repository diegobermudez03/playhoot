package codec

import (
	"encoding/json"

	"github.com/diegobermudez03/playhoot/game/language/v1/program"
)

// --- program.QuestionSlotDeclaration ---

type wireQuestionSlotDeclaration struct {
	Name         string          `json:"name"`
	Question     string          `json:"question"`
	Presentation json.RawMessage `json:"presentation"`
}

// encodeQuestionSlotDeclaration encodes value, an ordinary (non-interface)
// struct, as its JSON wire representation. Presentation reuses the
// existing question-presentation pointer helper: a nil Presentation
// encodes as JSON null.
func encodeQuestionSlotDeclaration(path string, value program.QuestionSlotDeclaration) (json.RawMessage, error) {
	presentation, err := encodeQuestionPresentationDeclaration(pathField(path, "presentation"), value.Presentation)
	if err != nil {
		return nil, err
	}
	return json.Marshal(wireQuestionSlotDeclaration{Name: value.Name, Question: value.Question, Presentation: presentation})
}

// decodeQuestionSlotDeclaration decodes data as a program.QuestionSlotDeclaration.
// JSON null for the declaration itself is not a valid encoding and
// produces a path-aware structural error; JSON null for Presentation is
// valid and decodes to a nil pointer.
func decodeQuestionSlotDeclaration(path string, data json.RawMessage) (program.QuestionSlotDeclaration, error) {
	var wire wireQuestionSlotDeclaration
	if err := decodeOrdinaryObject(path, data, &wire); err != nil {
		return program.QuestionSlotDeclaration{}, err
	}
	presentation, err := decodeQuestionPresentationDeclaration(pathField(path, "presentation"), wire.Presentation)
	if err != nil {
		return program.QuestionSlotDeclaration{}, err
	}
	return program.QuestionSlotDeclaration{Name: wire.Name, Question: wire.Question, Presentation: presentation}, nil
}

func encodeQuestionSlotDeclarations(path string, items []program.QuestionSlotDeclaration) ([]json.RawMessage, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]json.RawMessage, len(items))
	for i, item := range items {
		raw, err := encodeQuestionSlotDeclaration(pathIndex(path, i), item)
		if err != nil {
			return nil, err
		}
		result[i] = raw
	}
	return result, nil
}

func decodeQuestionSlotDeclarations(path string, items []json.RawMessage) ([]program.QuestionSlotDeclaration, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]program.QuestionSlotDeclaration, len(items))
	for i, raw := range items {
		item, err := decodeQuestionSlotDeclaration(pathIndex(path, i), raw)
		if err != nil {
			return nil, err
		}
		result[i] = item
	}
	return result, nil
}

// --- program.AskGroupSlotDeclaration ---

type wireAskGroupSlotDeclaration struct {
	Name         string          `json:"name"`
	Question     string          `json:"question"`
	Presentation json.RawMessage `json:"presentation"`
}

// encodeAskGroupSlotDeclaration encodes value, an ordinary (non-interface)
// struct, as its JSON wire representation. Presentation reuses the same
// question-presentation pointer helper as program.QuestionSlotDeclaration — there
// is no second presentation wire format.
func encodeAskGroupSlotDeclaration(path string, value program.AskGroupSlotDeclaration) (json.RawMessage, error) {
	presentation, err := encodeQuestionPresentationDeclaration(pathField(path, "presentation"), value.Presentation)
	if err != nil {
		return nil, err
	}
	return json.Marshal(wireAskGroupSlotDeclaration{Name: value.Name, Question: value.Question, Presentation: presentation})
}

// decodeAskGroupSlotDeclaration decodes data as an program.AskGroupSlotDeclaration.
// JSON null for the declaration itself is not a valid encoding and
// produces a path-aware structural error; JSON null for Presentation is
// valid and decodes to a nil pointer.
func decodeAskGroupSlotDeclaration(path string, data json.RawMessage) (program.AskGroupSlotDeclaration, error) {
	var wire wireAskGroupSlotDeclaration
	if err := decodeOrdinaryObject(path, data, &wire); err != nil {
		return program.AskGroupSlotDeclaration{}, err
	}
	presentation, err := decodeQuestionPresentationDeclaration(pathField(path, "presentation"), wire.Presentation)
	if err != nil {
		return program.AskGroupSlotDeclaration{}, err
	}
	return program.AskGroupSlotDeclaration{Name: wire.Name, Question: wire.Question, Presentation: presentation}, nil
}

func encodeAskGroupSlotDeclarations(path string, items []program.AskGroupSlotDeclaration) ([]json.RawMessage, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]json.RawMessage, len(items))
	for i, item := range items {
		raw, err := encodeAskGroupSlotDeclaration(pathIndex(path, i), item)
		if err != nil {
			return nil, err
		}
		result[i] = raw
	}
	return result, nil
}

func decodeAskGroupSlotDeclarations(path string, items []json.RawMessage) ([]program.AskGroupSlotDeclaration, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]program.AskGroupSlotDeclaration, len(items))
	for i, raw := range items {
		item, err := decodeAskGroupSlotDeclaration(pathIndex(path, i), raw)
		if err != nil {
			return nil, err
		}
		result[i] = item
	}
	return result, nil
}

// --- program.TimerSlotDeclaration ---

type wireTimerSlotDeclaration struct {
	Name string `json:"name"`
}

// encodeTimerSlotDeclaration encodes value, an ordinary (non-interface)
// struct, as its JSON wire representation.
func encodeTimerSlotDeclaration(path string, value program.TimerSlotDeclaration) (json.RawMessage, error) {
	return json.Marshal(wireTimerSlotDeclaration{Name: value.Name})
}

// decodeTimerSlotDeclaration decodes data as a program.TimerSlotDeclaration. JSON
// null is not a valid encoding and produces a path-aware structural
// error.
func decodeTimerSlotDeclaration(path string, data json.RawMessage) (program.TimerSlotDeclaration, error) {
	var wire wireTimerSlotDeclaration
	if err := decodeOrdinaryObject(path, data, &wire); err != nil {
		return program.TimerSlotDeclaration{}, err
	}
	return program.TimerSlotDeclaration{Name: wire.Name}, nil
}

func encodeTimerSlotDeclarations(path string, items []program.TimerSlotDeclaration) ([]json.RawMessage, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]json.RawMessage, len(items))
	for i, item := range items {
		raw, err := encodeTimerSlotDeclaration(pathIndex(path, i), item)
		if err != nil {
			return nil, err
		}
		result[i] = raw
	}
	return result, nil
}

func decodeTimerSlotDeclarations(path string, items []json.RawMessage) ([]program.TimerSlotDeclaration, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]program.TimerSlotDeclaration, len(items))
	for i, raw := range items {
		item, err := decodeTimerSlotDeclaration(pathIndex(path, i), raw)
		if err != nil {
			return nil, err
		}
		result[i] = item
	}
	return result, nil
}

// --- program.ChildWorkflowSlotDeclaration ---

type wireChildWorkflowSlotDeclaration struct {
	Name     string `json:"name"`
	Workflow string `json:"workflow"`
}

// encodeChildWorkflowSlotDeclaration encodes value, an ordinary
// (non-interface) struct, as its JSON wire representation.
func encodeChildWorkflowSlotDeclaration(path string, value program.ChildWorkflowSlotDeclaration) (json.RawMessage, error) {
	return json.Marshal(wireChildWorkflowSlotDeclaration{Name: value.Name, Workflow: value.Workflow})
}

// decodeChildWorkflowSlotDeclaration decodes data as a
// program.ChildWorkflowSlotDeclaration. JSON null is not a valid encoding and
// produces a path-aware structural error.
func decodeChildWorkflowSlotDeclaration(path string, data json.RawMessage) (program.ChildWorkflowSlotDeclaration, error) {
	var wire wireChildWorkflowSlotDeclaration
	if err := decodeOrdinaryObject(path, data, &wire); err != nil {
		return program.ChildWorkflowSlotDeclaration{}, err
	}
	return program.ChildWorkflowSlotDeclaration{Name: wire.Name, Workflow: wire.Workflow}, nil
}

func encodeChildWorkflowSlotDeclarations(path string, items []program.ChildWorkflowSlotDeclaration) ([]json.RawMessage, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]json.RawMessage, len(items))
	for i, item := range items {
		raw, err := encodeChildWorkflowSlotDeclaration(pathIndex(path, i), item)
		if err != nil {
			return nil, err
		}
		result[i] = raw
	}
	return result, nil
}

func decodeChildWorkflowSlotDeclarations(path string, items []json.RawMessage) ([]program.ChildWorkflowSlotDeclaration, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]program.ChildWorkflowSlotDeclaration, len(items))
	for i, raw := range items {
		item, err := decodeChildWorkflowSlotDeclaration(pathIndex(path, i), raw)
		if err != nil {
			return nil, err
		}
		result[i] = item
	}
	return result, nil
}

// --- program.TaskGroupSlotDeclaration ---

type wireTaskGroupSlotDeclaration struct {
	Name     string          `json:"name"`
	Workflow string          `json:"workflow"`
	KeyType  json.RawMessage `json:"key_type"`
}

// encodeTaskGroupSlotDeclaration encodes value, an ordinary
// (non-interface) struct, as its JSON wire representation.
func encodeTaskGroupSlotDeclaration(path string, value program.TaskGroupSlotDeclaration) (json.RawMessage, error) {
	keyType, err := encodeTypeReference(pathField(path, "key_type"), value.KeyType)
	if err != nil {
		return nil, err
	}
	return json.Marshal(wireTaskGroupSlotDeclaration{Name: value.Name, Workflow: value.Workflow, KeyType: keyType})
}

// decodeTaskGroupSlotDeclaration decodes data as a
// program.TaskGroupSlotDeclaration. JSON null for the declaration itself is not a
// valid encoding and produces a path-aware structural error; KeyType may
// independently be JSON null.
func decodeTaskGroupSlotDeclaration(path string, data json.RawMessage) (program.TaskGroupSlotDeclaration, error) {
	var wire wireTaskGroupSlotDeclaration
	if err := decodeOrdinaryObject(path, data, &wire); err != nil {
		return program.TaskGroupSlotDeclaration{}, err
	}
	keyType, err := decodeTypeReference(pathField(path, "key_type"), wire.KeyType)
	if err != nil {
		return program.TaskGroupSlotDeclaration{}, err
	}
	return program.TaskGroupSlotDeclaration{Name: wire.Name, Workflow: wire.Workflow, KeyType: keyType}, nil
}

func encodeTaskGroupSlotDeclarations(path string, items []program.TaskGroupSlotDeclaration) ([]json.RawMessage, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]json.RawMessage, len(items))
	for i, item := range items {
		raw, err := encodeTaskGroupSlotDeclaration(pathIndex(path, i), item)
		if err != nil {
			return nil, err
		}
		result[i] = raw
	}
	return result, nil
}

func decodeTaskGroupSlotDeclarations(path string, items []json.RawMessage) ([]program.TaskGroupSlotDeclaration, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]program.TaskGroupSlotDeclaration, len(items))
	for i, raw := range items {
		item, err := decodeTaskGroupSlotDeclaration(pathIndex(path, i), raw)
		if err != nil {
			return nil, err
		}
		result[i] = item
	}
	return result, nil
}
