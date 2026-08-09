package program

import "encoding/json"

// --- QuestionSlotDeclaration ---

type wireQuestionSlotDeclaration struct {
	Name         string          `json:"name"`
	Question     string          `json:"question"`
	Presentation json.RawMessage `json:"presentation"`
}

// encodeQuestionSlotDeclaration encodes value, an ordinary (non-interface)
// struct, as its JSON wire representation. Presentation reuses the
// existing question-presentation pointer helper: a nil Presentation
// encodes as JSON null.
func encodeQuestionSlotDeclaration(path string, value QuestionSlotDeclaration) (json.RawMessage, error) {
	presentation, err := encodeQuestionPresentationDeclaration(pathField(path, "presentation"), value.Presentation)
	if err != nil {
		return nil, err
	}
	return json.Marshal(wireQuestionSlotDeclaration{Name: value.Name, Question: value.Question, Presentation: presentation})
}

// decodeQuestionSlotDeclaration decodes data as a QuestionSlotDeclaration.
// JSON null for the declaration itself is not a valid encoding and
// produces a path-aware structural error; JSON null for Presentation is
// valid and decodes to a nil pointer.
func decodeQuestionSlotDeclaration(path string, data json.RawMessage) (QuestionSlotDeclaration, error) {
	var wire wireQuestionSlotDeclaration
	if err := decodeOrdinaryObject(path, data, &wire); err != nil {
		return QuestionSlotDeclaration{}, err
	}
	presentation, err := decodeQuestionPresentationDeclaration(pathField(path, "presentation"), wire.Presentation)
	if err != nil {
		return QuestionSlotDeclaration{}, err
	}
	return QuestionSlotDeclaration{Name: wire.Name, Question: wire.Question, Presentation: presentation}, nil
}

func encodeQuestionSlotDeclarations(path string, items []QuestionSlotDeclaration) ([]json.RawMessage, error) {
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

func decodeQuestionSlotDeclarations(path string, items []json.RawMessage) ([]QuestionSlotDeclaration, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]QuestionSlotDeclaration, len(items))
	for i, raw := range items {
		item, err := decodeQuestionSlotDeclaration(pathIndex(path, i), raw)
		if err != nil {
			return nil, err
		}
		result[i] = item
	}
	return result, nil
}

// --- AskGroupSlotDeclaration ---

type wireAskGroupSlotDeclaration struct {
	Name         string          `json:"name"`
	Question     string          `json:"question"`
	Presentation json.RawMessage `json:"presentation"`
}

// encodeAskGroupSlotDeclaration encodes value, an ordinary (non-interface)
// struct, as its JSON wire representation. Presentation reuses the same
// question-presentation pointer helper as QuestionSlotDeclaration — there
// is no second presentation wire format.
func encodeAskGroupSlotDeclaration(path string, value AskGroupSlotDeclaration) (json.RawMessage, error) {
	presentation, err := encodeQuestionPresentationDeclaration(pathField(path, "presentation"), value.Presentation)
	if err != nil {
		return nil, err
	}
	return json.Marshal(wireAskGroupSlotDeclaration{Name: value.Name, Question: value.Question, Presentation: presentation})
}

// decodeAskGroupSlotDeclaration decodes data as an AskGroupSlotDeclaration.
// JSON null for the declaration itself is not a valid encoding and
// produces a path-aware structural error; JSON null for Presentation is
// valid and decodes to a nil pointer.
func decodeAskGroupSlotDeclaration(path string, data json.RawMessage) (AskGroupSlotDeclaration, error) {
	var wire wireAskGroupSlotDeclaration
	if err := decodeOrdinaryObject(path, data, &wire); err != nil {
		return AskGroupSlotDeclaration{}, err
	}
	presentation, err := decodeQuestionPresentationDeclaration(pathField(path, "presentation"), wire.Presentation)
	if err != nil {
		return AskGroupSlotDeclaration{}, err
	}
	return AskGroupSlotDeclaration{Name: wire.Name, Question: wire.Question, Presentation: presentation}, nil
}

func encodeAskGroupSlotDeclarations(path string, items []AskGroupSlotDeclaration) ([]json.RawMessage, error) {
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

func decodeAskGroupSlotDeclarations(path string, items []json.RawMessage) ([]AskGroupSlotDeclaration, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]AskGroupSlotDeclaration, len(items))
	for i, raw := range items {
		item, err := decodeAskGroupSlotDeclaration(pathIndex(path, i), raw)
		if err != nil {
			return nil, err
		}
		result[i] = item
	}
	return result, nil
}

// --- TimerSlotDeclaration ---

type wireTimerSlotDeclaration struct {
	Name string `json:"name"`
}

// encodeTimerSlotDeclaration encodes value, an ordinary (non-interface)
// struct, as its JSON wire representation.
func encodeTimerSlotDeclaration(path string, value TimerSlotDeclaration) (json.RawMessage, error) {
	return json.Marshal(wireTimerSlotDeclaration{Name: value.Name})
}

// decodeTimerSlotDeclaration decodes data as a TimerSlotDeclaration. JSON
// null is not a valid encoding and produces a path-aware structural
// error.
func decodeTimerSlotDeclaration(path string, data json.RawMessage) (TimerSlotDeclaration, error) {
	var wire wireTimerSlotDeclaration
	if err := decodeOrdinaryObject(path, data, &wire); err != nil {
		return TimerSlotDeclaration{}, err
	}
	return TimerSlotDeclaration{Name: wire.Name}, nil
}

func encodeTimerSlotDeclarations(path string, items []TimerSlotDeclaration) ([]json.RawMessage, error) {
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

func decodeTimerSlotDeclarations(path string, items []json.RawMessage) ([]TimerSlotDeclaration, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]TimerSlotDeclaration, len(items))
	for i, raw := range items {
		item, err := decodeTimerSlotDeclaration(pathIndex(path, i), raw)
		if err != nil {
			return nil, err
		}
		result[i] = item
	}
	return result, nil
}

// --- ChildWorkflowSlotDeclaration ---

type wireChildWorkflowSlotDeclaration struct {
	Name     string `json:"name"`
	Workflow string `json:"workflow"`
}

// encodeChildWorkflowSlotDeclaration encodes value, an ordinary
// (non-interface) struct, as its JSON wire representation.
func encodeChildWorkflowSlotDeclaration(path string, value ChildWorkflowSlotDeclaration) (json.RawMessage, error) {
	return json.Marshal(wireChildWorkflowSlotDeclaration{Name: value.Name, Workflow: value.Workflow})
}

// decodeChildWorkflowSlotDeclaration decodes data as a
// ChildWorkflowSlotDeclaration. JSON null is not a valid encoding and
// produces a path-aware structural error.
func decodeChildWorkflowSlotDeclaration(path string, data json.RawMessage) (ChildWorkflowSlotDeclaration, error) {
	var wire wireChildWorkflowSlotDeclaration
	if err := decodeOrdinaryObject(path, data, &wire); err != nil {
		return ChildWorkflowSlotDeclaration{}, err
	}
	return ChildWorkflowSlotDeclaration{Name: wire.Name, Workflow: wire.Workflow}, nil
}

func encodeChildWorkflowSlotDeclarations(path string, items []ChildWorkflowSlotDeclaration) ([]json.RawMessage, error) {
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

func decodeChildWorkflowSlotDeclarations(path string, items []json.RawMessage) ([]ChildWorkflowSlotDeclaration, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]ChildWorkflowSlotDeclaration, len(items))
	for i, raw := range items {
		item, err := decodeChildWorkflowSlotDeclaration(pathIndex(path, i), raw)
		if err != nil {
			return nil, err
		}
		result[i] = item
	}
	return result, nil
}

// --- TaskGroupSlotDeclaration ---

type wireTaskGroupSlotDeclaration struct {
	Name     string          `json:"name"`
	Workflow string          `json:"workflow"`
	KeyType  json.RawMessage `json:"key_type"`
}

// encodeTaskGroupSlotDeclaration encodes value, an ordinary
// (non-interface) struct, as its JSON wire representation.
func encodeTaskGroupSlotDeclaration(path string, value TaskGroupSlotDeclaration) (json.RawMessage, error) {
	keyType, err := encodeTypeReference(pathField(path, "key_type"), value.KeyType)
	if err != nil {
		return nil, err
	}
	return json.Marshal(wireTaskGroupSlotDeclaration{Name: value.Name, Workflow: value.Workflow, KeyType: keyType})
}

// decodeTaskGroupSlotDeclaration decodes data as a
// TaskGroupSlotDeclaration. JSON null for the declaration itself is not a
// valid encoding and produces a path-aware structural error; KeyType may
// independently be JSON null.
func decodeTaskGroupSlotDeclaration(path string, data json.RawMessage) (TaskGroupSlotDeclaration, error) {
	var wire wireTaskGroupSlotDeclaration
	if err := decodeOrdinaryObject(path, data, &wire); err != nil {
		return TaskGroupSlotDeclaration{}, err
	}
	keyType, err := decodeTypeReference(pathField(path, "key_type"), wire.KeyType)
	if err != nil {
		return TaskGroupSlotDeclaration{}, err
	}
	return TaskGroupSlotDeclaration{Name: wire.Name, Workflow: wire.Workflow, KeyType: keyType}, nil
}

func encodeTaskGroupSlotDeclarations(path string, items []TaskGroupSlotDeclaration) ([]json.RawMessage, error) {
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

func decodeTaskGroupSlotDeclarations(path string, items []json.RawMessage) ([]TaskGroupSlotDeclaration, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]TaskGroupSlotDeclaration, len(items))
	for i, raw := range items {
		item, err := decodeTaskGroupSlotDeclaration(pathIndex(path, i), raw)
		if err != nil {
			return nil, err
		}
		result[i] = item
	}
	return result, nil
}
