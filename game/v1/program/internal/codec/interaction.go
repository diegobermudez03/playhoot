package codec

import (
	"encoding/json"

	"github.com/diegobermudez03/playhoot/game/v1/program"
)

// --- program.UserIntentDeclaration ---

type wireUserIntentDeclaration struct {
	Name       string                 `json:"name"`
	Parameters []wireFieldDeclaration `json:"parameters"`
}

// encodeUserIntentDeclaration encodes value, an ordinary (non-interface)
// struct, as its JSON wire representation.
func encodeUserIntentDeclaration(path string, value program.UserIntentDeclaration) (json.RawMessage, error) {
	parameters, err := encodeFieldDeclarations(pathField(path, "parameters"), value.Parameters)
	if err != nil {
		return nil, err
	}
	return json.Marshal(wireUserIntentDeclaration{Name: value.Name, Parameters: parameters})
}

// decodeUserIntentDeclaration decodes data as a program.UserIntentDeclaration.
// JSON null is not a valid encoding and produces a path-aware structural
// error.
func decodeUserIntentDeclaration(path string, data json.RawMessage) (program.UserIntentDeclaration, error) {
	var wire wireUserIntentDeclaration
	if err := decodeOrdinaryObject(path, data, &wire); err != nil {
		return program.UserIntentDeclaration{}, err
	}
	parameters, err := decodeFieldDeclarations(pathField(path, "parameters"), wire.Parameters)
	if err != nil {
		return program.UserIntentDeclaration{}, err
	}
	return program.UserIntentDeclaration{Name: wire.Name, Parameters: parameters}, nil
}

func encodeUserIntentDeclarations(path string, items []program.UserIntentDeclaration) ([]json.RawMessage, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]json.RawMessage, len(items))
	for i, item := range items {
		raw, err := encodeUserIntentDeclaration(pathIndex(path, i), item)
		if err != nil {
			return nil, err
		}
		result[i] = raw
	}
	return result, nil
}

func decodeUserIntentDeclarations(path string, items []json.RawMessage) ([]program.UserIntentDeclaration, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]program.UserIntentDeclaration, len(items))
	for i, raw := range items {
		item, err := decodeUserIntentDeclaration(pathIndex(path, i), raw)
		if err != nil {
			return nil, err
		}
		result[i] = item
	}
	return result, nil
}

// --- program.QuestionDeclaration ---

type wireQuestionDeclaration struct {
	Name         string                 `json:"name"`
	Parameters   []wireFieldDeclaration `json:"parameters"`
	ResponseType json.RawMessage        `json:"response_type"`
	Validation   json.RawMessage        `json:"validation"`
}

// encodeQuestionDeclaration encodes value, an ordinary (non-interface)
// struct, as its JSON wire representation.
func encodeQuestionDeclaration(path string, value program.QuestionDeclaration) (json.RawMessage, error) {
	parameters, err := encodeFieldDeclarations(pathField(path, "parameters"), value.Parameters)
	if err != nil {
		return nil, err
	}
	responseType, err := encodeTypeReference(pathField(path, "response_type"), value.ResponseType)
	if err != nil {
		return nil, err
	}
	validation, err := encodeExpression(pathField(path, "validation"), value.Validation)
	if err != nil {
		return nil, err
	}
	return json.Marshal(wireQuestionDeclaration{
		Name:         value.Name,
		Parameters:   parameters,
		ResponseType: responseType,
		Validation:   validation,
	})
}

// decodeQuestionDeclaration decodes data as a program.QuestionDeclaration. JSON
// null is not a valid encoding and produces a path-aware structural error.
func decodeQuestionDeclaration(path string, data json.RawMessage) (program.QuestionDeclaration, error) {
	var wire wireQuestionDeclaration
	if err := decodeOrdinaryObject(path, data, &wire); err != nil {
		return program.QuestionDeclaration{}, err
	}
	parameters, err := decodeFieldDeclarations(pathField(path, "parameters"), wire.Parameters)
	if err != nil {
		return program.QuestionDeclaration{}, err
	}
	responseType, err := decodeTypeReference(pathField(path, "response_type"), wire.ResponseType)
	if err != nil {
		return program.QuestionDeclaration{}, err
	}
	validation, err := decodeExpression(pathField(path, "validation"), wire.Validation)
	if err != nil {
		return program.QuestionDeclaration{}, err
	}
	return program.QuestionDeclaration{
		Name:         wire.Name,
		Parameters:   parameters,
		ResponseType: responseType,
		Validation:   validation,
	}, nil
}

func encodeQuestionDeclarations(path string, items []program.QuestionDeclaration) ([]json.RawMessage, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]json.RawMessage, len(items))
	for i, item := range items {
		raw, err := encodeQuestionDeclaration(pathIndex(path, i), item)
		if err != nil {
			return nil, err
		}
		result[i] = raw
	}
	return result, nil
}

func decodeQuestionDeclarations(path string, items []json.RawMessage) ([]program.QuestionDeclaration, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]program.QuestionDeclaration, len(items))
	for i, raw := range items {
		item, err := decodeQuestionDeclaration(pathIndex(path, i), raw)
		if err != nil {
			return nil, err
		}
		result[i] = item
	}
	return result, nil
}

// --- program.EffectDeclaration ---

type wireEffectDeclaration struct {
	Name       string                 `json:"name"`
	Parameters []wireFieldDeclaration `json:"parameters"`
}

// encodeEffectDeclaration encodes value, an ordinary (non-interface)
// struct, as its JSON wire representation.
func encodeEffectDeclaration(path string, value program.EffectDeclaration) (json.RawMessage, error) {
	parameters, err := encodeFieldDeclarations(pathField(path, "parameters"), value.Parameters)
	if err != nil {
		return nil, err
	}
	return json.Marshal(wireEffectDeclaration{Name: value.Name, Parameters: parameters})
}

// decodeEffectDeclaration decodes data as an program.EffectDeclaration. JSON null
// is not a valid encoding and produces a path-aware structural error.
func decodeEffectDeclaration(path string, data json.RawMessage) (program.EffectDeclaration, error) {
	var wire wireEffectDeclaration
	if err := decodeOrdinaryObject(path, data, &wire); err != nil {
		return program.EffectDeclaration{}, err
	}
	parameters, err := decodeFieldDeclarations(pathField(path, "parameters"), wire.Parameters)
	if err != nil {
		return program.EffectDeclaration{}, err
	}
	return program.EffectDeclaration{Name: wire.Name, Parameters: parameters}, nil
}

func encodeEffectDeclarations(path string, items []program.EffectDeclaration) ([]json.RawMessage, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]json.RawMessage, len(items))
	for i, item := range items {
		raw, err := encodeEffectDeclaration(pathIndex(path, i), item)
		if err != nil {
			return nil, err
		}
		result[i] = raw
	}
	return result, nil
}

func decodeEffectDeclarations(path string, items []json.RawMessage) ([]program.EffectDeclaration, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]program.EffectDeclaration, len(items))
	for i, raw := range items {
		item, err := decodeEffectDeclaration(pathIndex(path, i), raw)
		if err != nil {
			return nil, err
		}
		result[i] = item
	}
	return result, nil
}
