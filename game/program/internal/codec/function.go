package codec

import "github.com/diegobermudez03/playhoot/game/program"

import "encoding/json"

type wireFunctionDeclaration struct {
	Name       string                 `json:"name"`
	Parameters []wireFieldDeclaration `json:"parameters"`
	ResultType json.RawMessage        `json:"result_type"`
	Body       json.RawMessage        `json:"body"`
}

// encodeFunctionDeclaration encodes value, an ordinary (non-interface)
// struct, as its JSON wire representation.
func encodeFunctionDeclaration(path string, value program.FunctionDeclaration) (json.RawMessage, error) {
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
	return json.Marshal(wireFunctionDeclaration{Name: value.Name, Parameters: parameters, ResultType: resultType, Body: body})
}

// decodeFunctionDeclaration decodes data as a program.FunctionDeclaration. JSON
// null is not a valid encoding and produces a path-aware structural error.
func decodeFunctionDeclaration(path string, data json.RawMessage) (program.FunctionDeclaration, error) {
	var wire wireFunctionDeclaration
	if err := decodeOrdinaryObject(path, data, &wire); err != nil {
		return program.FunctionDeclaration{}, err
	}
	parameters, err := decodeFieldDeclarations(pathField(path, "parameters"), wire.Parameters)
	if err != nil {
		return program.FunctionDeclaration{}, err
	}
	resultType, err := decodeTypeReference(pathField(path, "result_type"), wire.ResultType)
	if err != nil {
		return program.FunctionDeclaration{}, err
	}
	body, err := decodeExpression(pathField(path, "body"), wire.Body)
	if err != nil {
		return program.FunctionDeclaration{}, err
	}
	return program.FunctionDeclaration{Name: wire.Name, Parameters: parameters, ResultType: resultType, Body: body}, nil
}

func encodeFunctionDeclarations(path string, items []program.FunctionDeclaration) ([]json.RawMessage, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]json.RawMessage, len(items))
	for i, item := range items {
		raw, err := encodeFunctionDeclaration(pathIndex(path, i), item)
		if err != nil {
			return nil, err
		}
		result[i] = raw
	}
	return result, nil
}

func decodeFunctionDeclarations(path string, items []json.RawMessage) ([]program.FunctionDeclaration, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]program.FunctionDeclaration, len(items))
	for i, raw := range items {
		item, err := decodeFunctionDeclaration(pathIndex(path, i), raw)
		if err != nil {
			return nil, err
		}
		result[i] = item
	}
	return result, nil
}

// --- program.InvariantDeclaration ---

type wireInvariantDeclaration struct {
	Name      string          `json:"name"`
	Condition json.RawMessage `json:"condition"`
}

// encodeInvariantDeclaration encodes value, an ordinary (non-interface)
// struct, as its JSON wire representation.
func encodeInvariantDeclaration(path string, value program.InvariantDeclaration) (json.RawMessage, error) {
	condition, err := encodeExpression(pathField(path, "condition"), value.Condition)
	if err != nil {
		return nil, err
	}
	return json.Marshal(wireInvariantDeclaration{Name: value.Name, Condition: condition})
}

// decodeInvariantDeclaration decodes data as an program.InvariantDeclaration. JSON
// null is not a valid encoding and produces a path-aware structural error.
func decodeInvariantDeclaration(path string, data json.RawMessage) (program.InvariantDeclaration, error) {
	var wire wireInvariantDeclaration
	if err := decodeOrdinaryObject(path, data, &wire); err != nil {
		return program.InvariantDeclaration{}, err
	}
	condition, err := decodeExpression(pathField(path, "condition"), wire.Condition)
	if err != nil {
		return program.InvariantDeclaration{}, err
	}
	return program.InvariantDeclaration{Name: wire.Name, Condition: condition}, nil
}

func encodeInvariantDeclarations(path string, items []program.InvariantDeclaration) ([]json.RawMessage, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]json.RawMessage, len(items))
	for i, item := range items {
		raw, err := encodeInvariantDeclaration(pathIndex(path, i), item)
		if err != nil {
			return nil, err
		}
		result[i] = raw
	}
	return result, nil
}

func decodeInvariantDeclarations(path string, items []json.RawMessage) ([]program.InvariantDeclaration, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]program.InvariantDeclaration, len(items))
	for i, raw := range items {
		item, err := decodeInvariantDeclaration(pathIndex(path, i), raw)
		if err != nil {
			return nil, err
		}
		result[i] = item
	}
	return result, nil
}
