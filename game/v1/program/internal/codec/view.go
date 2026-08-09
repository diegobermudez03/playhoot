package codec

import (
	"encoding/json"

	"github.com/diegobermudez03/playhoot/game/v1/program"
)

type wireViewDeclaration struct {
	Name       string          `json:"name"`
	ModelType  json.RawMessage `json:"model_type"`
	LocalState json.RawMessage `json:"local_state"`
	Root       json.RawMessage `json:"root"`
}

// encodeViewDeclaration encodes value, an ordinary (non-interface)
// struct, as its JSON wire representation. LocalState is always encoded
// as an ordinary state-declaration object, never JSON null.
func encodeViewDeclaration(path string, value program.ViewDeclaration) (json.RawMessage, error) {
	modelType, err := encodeTypeReference(pathField(path, "model_type"), value.ModelType)
	if err != nil {
		return nil, err
	}
	localState, err := encodeStateDeclaration(pathField(path, "local_state"), value.LocalState)
	if err != nil {
		return nil, err
	}
	root, err := encodeUIElement(pathField(path, "root"), value.Root)
	if err != nil {
		return nil, err
	}
	return json.Marshal(wireViewDeclaration{Name: value.Name, ModelType: modelType, LocalState: localState, Root: root})
}

// decodeViewDeclaration decodes data as a program.ViewDeclaration. JSON null is
// not a valid encoding of the declaration itself, nor of its LocalState
// field, and produces a path-aware structural error in either case;
// ModelType and Root may each independently be JSON null.
func decodeViewDeclaration(path string, data json.RawMessage) (program.ViewDeclaration, error) {
	var wire wireViewDeclaration
	if err := decodeOrdinaryObject(path, data, &wire); err != nil {
		return program.ViewDeclaration{}, err
	}
	modelType, err := decodeTypeReference(pathField(path, "model_type"), wire.ModelType)
	if err != nil {
		return program.ViewDeclaration{}, err
	}
	localState, err := decodeStateDeclaration(pathField(path, "local_state"), wire.LocalState)
	if err != nil {
		return program.ViewDeclaration{}, err
	}
	root, err := decodeUIElement(pathField(path, "root"), wire.Root)
	if err != nil {
		return program.ViewDeclaration{}, err
	}
	return program.ViewDeclaration{Name: wire.Name, ModelType: modelType, LocalState: localState, Root: root}, nil
}

func encodeViewDeclarations(path string, items []program.ViewDeclaration) ([]json.RawMessage, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]json.RawMessage, len(items))
	for i, item := range items {
		raw, err := encodeViewDeclaration(pathIndex(path, i), item)
		if err != nil {
			return nil, err
		}
		result[i] = raw
	}
	return result, nil
}

func decodeViewDeclarations(path string, items []json.RawMessage) ([]program.ViewDeclaration, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]program.ViewDeclaration, len(items))
	for i, raw := range items {
		item, err := decodeViewDeclaration(pathIndex(path, i), raw)
		if err != nil {
			return nil, err
		}
		result[i] = item
	}
	return result, nil
}
