package program

import "encoding/json"

type wireProjectionDeclaration struct {
	Name       string                 `json:"name"`
	Parameters []wireFieldDeclaration `json:"parameters"`
	ResultType json.RawMessage        `json:"result_type"`
	Body       json.RawMessage        `json:"body"`
}

// encodeProjectionDeclaration encodes value, an ordinary (non-interface)
// struct, as its JSON wire representation.
func encodeProjectionDeclaration(path string, value ProjectionDeclaration) (json.RawMessage, error) {
	parameters, err := encodeFieldDeclarations(pathField(path, "parameters"), value.Parameters)
	if err != nil {
		return nil, err
	}
	resultType, err := encodeTypeReference(pathField(path, "result_type"), value.ResultType)
	if err != nil {
		return nil, err
	}
	body, err := encodeExpression(pathField(path, "body"), value.Body)
	if err != nil {
		return nil, err
	}
	return json.Marshal(wireProjectionDeclaration{Name: value.Name, Parameters: parameters, ResultType: resultType, Body: body})
}

// decodeProjectionDeclaration decodes data as a ProjectionDeclaration.
// JSON null is not a valid encoding and produces a path-aware structural
// error.
func decodeProjectionDeclaration(path string, data json.RawMessage) (ProjectionDeclaration, error) {
	var wire wireProjectionDeclaration
	if err := decodeOrdinaryObject(path, data, &wire); err != nil {
		return ProjectionDeclaration{}, err
	}
	parameters, err := decodeFieldDeclarations(pathField(path, "parameters"), wire.Parameters)
	if err != nil {
		return ProjectionDeclaration{}, err
	}
	resultType, err := decodeTypeReference(pathField(path, "result_type"), wire.ResultType)
	if err != nil {
		return ProjectionDeclaration{}, err
	}
	body, err := decodeExpression(pathField(path, "body"), wire.Body)
	if err != nil {
		return ProjectionDeclaration{}, err
	}
	return ProjectionDeclaration{Name: wire.Name, Parameters: parameters, ResultType: resultType, Body: body}, nil
}

func encodeProjectionDeclarations(path string, items []ProjectionDeclaration) ([]json.RawMessage, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]json.RawMessage, len(items))
	for i, item := range items {
		raw, err := encodeProjectionDeclaration(pathIndex(path, i), item)
		if err != nil {
			return nil, err
		}
		result[i] = raw
	}
	return result, nil
}

func decodeProjectionDeclarations(path string, items []json.RawMessage) ([]ProjectionDeclaration, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]ProjectionDeclaration, len(items))
	for i, raw := range items {
		item, err := decodeProjectionDeclaration(pathIndex(path, i), raw)
		if err != nil {
			return nil, err
		}
		result[i] = item
	}
	return result, nil
}
