package codec

import "github.com/diegobermudez03/playhoot/game/program"

import "encoding/json"

type wireResourceDeclaration struct {
	Name  string          `json:"name"`
	Type  json.RawMessage `json:"type"`
	Value json.RawMessage `json:"value"`
}

// encodeResourceDeclaration encodes value, an ordinary (non-interface)
// struct, as its JSON wire representation.
func encodeResourceDeclaration(path string, value program.ResourceDeclaration) (json.RawMessage, error) {
	typeRef, err := encodeTypeReference(pathField(path, "type"), value.Type)
	if err != nil {
		return nil, err
	}
	val, err := encodeExpression(pathField(path, "value"), value.Value)
	if err != nil {
		return nil, err
	}
	return json.Marshal(wireResourceDeclaration{Name: value.Name, Type: typeRef, Value: val})
}

// decodeResourceDeclaration decodes data as a program.ResourceDeclaration. JSON
// null is not a valid encoding and produces a path-aware structural error.
func decodeResourceDeclaration(path string, data json.RawMessage) (program.ResourceDeclaration, error) {
	var wire wireResourceDeclaration
	if err := decodeOrdinaryObject(path, data, &wire); err != nil {
		return program.ResourceDeclaration{}, err
	}
	typeRef, err := decodeTypeReference(pathField(path, "type"), wire.Type)
	if err != nil {
		return program.ResourceDeclaration{}, err
	}
	val, err := decodeExpression(pathField(path, "value"), wire.Value)
	if err != nil {
		return program.ResourceDeclaration{}, err
	}
	return program.ResourceDeclaration{Name: wire.Name, Type: typeRef, Value: val}, nil
}

func encodeResourceDeclarations(path string, items []program.ResourceDeclaration) ([]json.RawMessage, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]json.RawMessage, len(items))
	for i, item := range items {
		raw, err := encodeResourceDeclaration(pathIndex(path, i), item)
		if err != nil {
			return nil, err
		}
		result[i] = raw
	}
	return result, nil
}

func decodeResourceDeclarations(path string, items []json.RawMessage) ([]program.ResourceDeclaration, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]program.ResourceDeclaration, len(items))
	for i, raw := range items {
		item, err := decodeResourceDeclaration(pathIndex(path, i), raw)
		if err != nil {
			return nil, err
		}
		result[i] = item
	}
	return result, nil
}

// --- program.StateFieldDeclaration / program.StateDeclaration ---

type wireStateFieldDeclaration struct {
	Name        string          `json:"name"`
	Type        json.RawMessage `json:"type"`
	Initializer json.RawMessage `json:"initializer"`
}

func encodeStateFieldDeclarations(path string, fields []program.StateFieldDeclaration) ([]wireStateFieldDeclaration, error) {
	if fields == nil {
		return nil, nil
	}
	result := make([]wireStateFieldDeclaration, len(fields))
	for i, field := range fields {
		itemPath := pathIndex(path, i)
		typeRef, err := encodeTypeReference(pathField(itemPath, "type"), field.Type)
		if err != nil {
			return nil, err
		}
		initializer, err := encodeExpression(pathField(itemPath, "initializer"), field.Initializer)
		if err != nil {
			return nil, err
		}
		result[i] = wireStateFieldDeclaration{Name: field.Name, Type: typeRef, Initializer: initializer}
	}
	return result, nil
}

func decodeStateFieldDeclarations(path string, fields []wireStateFieldDeclaration) ([]program.StateFieldDeclaration, error) {
	if fields == nil {
		return nil, nil
	}
	result := make([]program.StateFieldDeclaration, len(fields))
	for i, field := range fields {
		itemPath := pathIndex(path, i)
		typeRef, err := decodeTypeReference(pathField(itemPath, "type"), field.Type)
		if err != nil {
			return nil, err
		}
		initializer, err := decodeExpression(pathField(itemPath, "initializer"), field.Initializer)
		if err != nil {
			return nil, err
		}
		result[i] = program.StateFieldDeclaration{Name: field.Name, Type: typeRef, Initializer: initializer}
	}
	return result, nil
}

type wireStateDeclaration struct {
	Fields []wireStateFieldDeclaration `json:"fields"`
}

// encodeStateDeclaration encodes value, an ordinary (non-interface)
// struct, as its JSON wire representation.
func encodeStateDeclaration(path string, value program.StateDeclaration) (json.RawMessage, error) {
	fields, err := encodeStateFieldDeclarations(pathField(path, "fields"), value.Fields)
	if err != nil {
		return nil, err
	}
	return json.Marshal(wireStateDeclaration{Fields: fields})
}

// decodeStateDeclaration decodes data as a program.StateDeclaration. JSON null is
// not a valid encoding and produces a path-aware structural error.
func decodeStateDeclaration(path string, data json.RawMessage) (program.StateDeclaration, error) {
	var wire wireStateDeclaration
	if err := decodeOrdinaryObject(path, data, &wire); err != nil {
		return program.StateDeclaration{}, err
	}
	fields, err := decodeStateFieldDeclarations(pathField(path, "fields"), wire.Fields)
	if err != nil {
		return program.StateDeclaration{}, err
	}
	return program.StateDeclaration{Fields: fields}, nil
}
