package program

import "encoding/json"

// --- UserIntentDeclaration ---

type wireUserIntentDeclaration struct {
	Name       string                 `json:"name"`
	Parameters []wireFieldDeclaration `json:"parameters"`
}

// encodeUserIntentDeclaration encodes value, an ordinary (non-interface)
// struct, as its JSON wire representation.
func encodeUserIntentDeclaration(path string, value UserIntentDeclaration) (json.RawMessage, error) {
	parameters, err := encodeFieldDeclarations(pathField(path, "parameters"), value.Parameters)
	if err != nil {
		return nil, err
	}
	return json.Marshal(wireUserIntentDeclaration{Name: value.Name, Parameters: parameters})
}

// decodeUserIntentDeclaration decodes data as a UserIntentDeclaration.
// JSON null is not a valid encoding and produces a path-aware structural
// error.
func decodeUserIntentDeclaration(path string, data json.RawMessage) (UserIntentDeclaration, error) {
	var wire wireUserIntentDeclaration
	if err := decodeOrdinaryObject(path, data, &wire); err != nil {
		return UserIntentDeclaration{}, err
	}
	parameters, err := decodeFieldDeclarations(pathField(path, "parameters"), wire.Parameters)
	if err != nil {
		return UserIntentDeclaration{}, err
	}
	return UserIntentDeclaration{Name: wire.Name, Parameters: parameters}, nil
}

func encodeUserIntentDeclarations(path string, items []UserIntentDeclaration) ([]json.RawMessage, error) {
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

func decodeUserIntentDeclarations(path string, items []json.RawMessage) ([]UserIntentDeclaration, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]UserIntentDeclaration, len(items))
	for i, raw := range items {
		item, err := decodeUserIntentDeclaration(pathIndex(path, i), raw)
		if err != nil {
			return nil, err
		}
		result[i] = item
	}
	return result, nil
}

// --- QuestionDeclaration ---

type wireQuestionDeclaration struct {
	Name         string                 `json:"name"`
	Parameters   []wireFieldDeclaration `json:"parameters"`
	ResponseType json.RawMessage        `json:"response_type"`
	Validation   json.RawMessage        `json:"validation"`
}

// encodeQuestionDeclaration encodes value, an ordinary (non-interface)
// struct, as its JSON wire representation.
func encodeQuestionDeclaration(path string, value QuestionDeclaration) (json.RawMessage, error) {
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

// decodeQuestionDeclaration decodes data as a QuestionDeclaration. JSON
// null is not a valid encoding and produces a path-aware structural error.
func decodeQuestionDeclaration(path string, data json.RawMessage) (QuestionDeclaration, error) {
	var wire wireQuestionDeclaration
	if err := decodeOrdinaryObject(path, data, &wire); err != nil {
		return QuestionDeclaration{}, err
	}
	parameters, err := decodeFieldDeclarations(pathField(path, "parameters"), wire.Parameters)
	if err != nil {
		return QuestionDeclaration{}, err
	}
	responseType, err := decodeTypeReference(pathField(path, "response_type"), wire.ResponseType)
	if err != nil {
		return QuestionDeclaration{}, err
	}
	validation, err := decodeExpression(pathField(path, "validation"), wire.Validation)
	if err != nil {
		return QuestionDeclaration{}, err
	}
	return QuestionDeclaration{
		Name:         wire.Name,
		Parameters:   parameters,
		ResponseType: responseType,
		Validation:   validation,
	}, nil
}

func encodeQuestionDeclarations(path string, items []QuestionDeclaration) ([]json.RawMessage, error) {
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

func decodeQuestionDeclarations(path string, items []json.RawMessage) ([]QuestionDeclaration, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]QuestionDeclaration, len(items))
	for i, raw := range items {
		item, err := decodeQuestionDeclaration(pathIndex(path, i), raw)
		if err != nil {
			return nil, err
		}
		result[i] = item
	}
	return result, nil
}

// --- EffectDeclaration ---

type wireEffectDeclaration struct {
	Name       string                 `json:"name"`
	Parameters []wireFieldDeclaration `json:"parameters"`
}

// encodeEffectDeclaration encodes value, an ordinary (non-interface)
// struct, as its JSON wire representation.
func encodeEffectDeclaration(path string, value EffectDeclaration) (json.RawMessage, error) {
	parameters, err := encodeFieldDeclarations(pathField(path, "parameters"), value.Parameters)
	if err != nil {
		return nil, err
	}
	return json.Marshal(wireEffectDeclaration{Name: value.Name, Parameters: parameters})
}

// decodeEffectDeclaration decodes data as an EffectDeclaration. JSON null
// is not a valid encoding and produces a path-aware structural error.
func decodeEffectDeclaration(path string, data json.RawMessage) (EffectDeclaration, error) {
	var wire wireEffectDeclaration
	if err := decodeOrdinaryObject(path, data, &wire); err != nil {
		return EffectDeclaration{}, err
	}
	parameters, err := decodeFieldDeclarations(pathField(path, "parameters"), wire.Parameters)
	if err != nil {
		return EffectDeclaration{}, err
	}
	return EffectDeclaration{Name: wire.Name, Parameters: parameters}, nil
}

func encodeEffectDeclarations(path string, items []EffectDeclaration) ([]json.RawMessage, error) {
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

func decodeEffectDeclarations(path string, items []json.RawMessage) ([]EffectDeclaration, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]EffectDeclaration, len(items))
	for i, raw := range items {
		item, err := decodeEffectDeclaration(pathIndex(path, i), raw)
		if err != nil {
			return nil, err
		}
		result[i] = item
	}
	return result, nil
}
