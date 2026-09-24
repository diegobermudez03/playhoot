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

// --- program.KeyedQuestionSlotDeclaration ---

type wireKeyedQuestionSlotDeclaration struct {
	Name         string          `json:"name"`
	Question     string          `json:"question"`
	KeyType      json.RawMessage `json:"key_type"`
	Presentation json.RawMessage `json:"presentation"`
}

func encodeKeyedQuestionSlotDeclaration(path string, value program.KeyedQuestionSlotDeclaration) (json.RawMessage, error) {
	keyType, err := encodeTypeReference(pathField(path, "key_type"), value.KeyType)
	if err != nil {
		return nil, err
	}
	presentation, err := encodeQuestionPresentationDeclaration(pathField(path, "presentation"), value.Presentation)
	if err != nil {
		return nil, err
	}
	return json.Marshal(wireKeyedQuestionSlotDeclaration{Name: value.Name, Question: value.Question, KeyType: keyType, Presentation: presentation})
}

func decodeKeyedQuestionSlotDeclaration(path string, data json.RawMessage) (program.KeyedQuestionSlotDeclaration, error) {
	var wire wireKeyedQuestionSlotDeclaration
	if err := decodeOrdinaryObject(path, data, &wire); err != nil {
		return program.KeyedQuestionSlotDeclaration{}, err
	}
	keyType, err := decodeTypeReference(pathField(path, "key_type"), wire.KeyType)
	if err != nil {
		return program.KeyedQuestionSlotDeclaration{}, err
	}
	presentation, err := decodeQuestionPresentationDeclaration(pathField(path, "presentation"), wire.Presentation)
	if err != nil {
		return program.KeyedQuestionSlotDeclaration{}, err
	}
	return program.KeyedQuestionSlotDeclaration{Name: wire.Name, Question: wire.Question, KeyType: keyType, Presentation: presentation}, nil
}

func encodeKeyedQuestionSlotDeclarations(path string, items []program.KeyedQuestionSlotDeclaration) ([]json.RawMessage, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]json.RawMessage, len(items))
	for i, item := range items {
		raw, err := encodeKeyedQuestionSlotDeclaration(pathIndex(path, i), item)
		if err != nil {
			return nil, err
		}
		result[i] = raw
	}
	return result, nil
}

func decodeKeyedQuestionSlotDeclarations(path string, items []json.RawMessage) ([]program.KeyedQuestionSlotDeclaration, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]program.KeyedQuestionSlotDeclaration, len(items))
	for i, raw := range items {
		item, err := decodeKeyedQuestionSlotDeclaration(pathIndex(path, i), raw)
		if err != nil {
			return nil, err
		}
		result[i] = item
	}
	return result, nil
}

// --- program.KeyedAskGroupSlotDeclaration ---

type wireKeyedAskGroupSlotDeclaration struct {
	Name         string          `json:"name"`
	Question     string          `json:"question"`
	KeyType      json.RawMessage `json:"key_type"`
	Presentation json.RawMessage `json:"presentation"`
}

func encodeKeyedAskGroupSlotDeclaration(path string, value program.KeyedAskGroupSlotDeclaration) (json.RawMessage, error) {
	keyType, err := encodeTypeReference(pathField(path, "key_type"), value.KeyType)
	if err != nil {
		return nil, err
	}
	presentation, err := encodeQuestionPresentationDeclaration(pathField(path, "presentation"), value.Presentation)
	if err != nil {
		return nil, err
	}
	return json.Marshal(wireKeyedAskGroupSlotDeclaration{Name: value.Name, Question: value.Question, KeyType: keyType, Presentation: presentation})
}

func decodeKeyedAskGroupSlotDeclaration(path string, data json.RawMessage) (program.KeyedAskGroupSlotDeclaration, error) {
	var wire wireKeyedAskGroupSlotDeclaration
	if err := decodeOrdinaryObject(path, data, &wire); err != nil {
		return program.KeyedAskGroupSlotDeclaration{}, err
	}
	keyType, err := decodeTypeReference(pathField(path, "key_type"), wire.KeyType)
	if err != nil {
		return program.KeyedAskGroupSlotDeclaration{}, err
	}
	presentation, err := decodeQuestionPresentationDeclaration(pathField(path, "presentation"), wire.Presentation)
	if err != nil {
		return program.KeyedAskGroupSlotDeclaration{}, err
	}
	return program.KeyedAskGroupSlotDeclaration{Name: wire.Name, Question: wire.Question, KeyType: keyType, Presentation: presentation}, nil
}

func encodeKeyedAskGroupSlotDeclarations(path string, items []program.KeyedAskGroupSlotDeclaration) ([]json.RawMessage, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]json.RawMessage, len(items))
	for i, item := range items {
		raw, err := encodeKeyedAskGroupSlotDeclaration(pathIndex(path, i), item)
		if err != nil {
			return nil, err
		}
		result[i] = raw
	}
	return result, nil
}

func decodeKeyedAskGroupSlotDeclarations(path string, items []json.RawMessage) ([]program.KeyedAskGroupSlotDeclaration, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]program.KeyedAskGroupSlotDeclaration, len(items))
	for i, raw := range items {
		item, err := decodeKeyedAskGroupSlotDeclaration(pathIndex(path, i), raw)
		if err != nil {
			return nil, err
		}
		result[i] = item
	}
	return result, nil
}

// --- program.KeyedTimerSlotDeclaration ---

type wireKeyedTimerSlotDeclaration struct {
	Name    string          `json:"name"`
	KeyType json.RawMessage `json:"key_type"`
}

func encodeKeyedTimerSlotDeclaration(path string, value program.KeyedTimerSlotDeclaration) (json.RawMessage, error) {
	keyType, err := encodeTypeReference(pathField(path, "key_type"), value.KeyType)
	if err != nil {
		return nil, err
	}
	return json.Marshal(wireKeyedTimerSlotDeclaration{Name: value.Name, KeyType: keyType})
}

func decodeKeyedTimerSlotDeclaration(path string, data json.RawMessage) (program.KeyedTimerSlotDeclaration, error) {
	var wire wireKeyedTimerSlotDeclaration
	if err := decodeOrdinaryObject(path, data, &wire); err != nil {
		return program.KeyedTimerSlotDeclaration{}, err
	}
	keyType, err := decodeTypeReference(pathField(path, "key_type"), wire.KeyType)
	if err != nil {
		return program.KeyedTimerSlotDeclaration{}, err
	}
	return program.KeyedTimerSlotDeclaration{Name: wire.Name, KeyType: keyType}, nil
}

func encodeKeyedTimerSlotDeclarations(path string, items []program.KeyedTimerSlotDeclaration) ([]json.RawMessage, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]json.RawMessage, len(items))
	for i, item := range items {
		raw, err := encodeKeyedTimerSlotDeclaration(pathIndex(path, i), item)
		if err != nil {
			return nil, err
		}
		result[i] = raw
	}
	return result, nil
}

func decodeKeyedTimerSlotDeclarations(path string, items []json.RawMessage) ([]program.KeyedTimerSlotDeclaration, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]program.KeyedTimerSlotDeclaration, len(items))
	for i, raw := range items {
		item, err := decodeKeyedTimerSlotDeclaration(pathIndex(path, i), raw)
		if err != nil {
			return nil, err
		}
		result[i] = item
	}
	return result, nil
}
