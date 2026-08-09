package codec

import (
	"encoding/json"

	"github.com/diegobermudez03/playhoot/game/v1/program"
)

type wirePresentationSlotDeclaration struct {
	Name string `json:"name"`
}

// encodePresentationSlotDeclaration encodes value, an ordinary
// (non-interface) struct, as its JSON wire representation.
func encodePresentationSlotDeclaration(path string, value program.PresentationSlotDeclaration) (json.RawMessage, error) {
	return json.Marshal(wirePresentationSlotDeclaration{Name: value.Name})
}

// decodePresentationSlotDeclaration decodes data as a
// program.PresentationSlotDeclaration. JSON null is not a valid encoding and
// produces a path-aware structural error.
func decodePresentationSlotDeclaration(path string, data json.RawMessage) (program.PresentationSlotDeclaration, error) {
	var wire wirePresentationSlotDeclaration
	if err := decodeOrdinaryObject(path, data, &wire); err != nil {
		return program.PresentationSlotDeclaration{}, err
	}
	return program.PresentationSlotDeclaration{Name: wire.Name}, nil
}

func encodePresentationSlotDeclarations(path string, items []program.PresentationSlotDeclaration) ([]json.RawMessage, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]json.RawMessage, len(items))
	for i, item := range items {
		raw, err := encodePresentationSlotDeclaration(pathIndex(path, i), item)
		if err != nil {
			return nil, err
		}
		result[i] = raw
	}
	return result, nil
}

func decodePresentationSlotDeclarations(path string, items []json.RawMessage) ([]program.PresentationSlotDeclaration, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]program.PresentationSlotDeclaration, len(items))
	for i, raw := range items {
		item, err := decodePresentationSlotDeclaration(pathIndex(path, i), raw)
		if err != nil {
			return nil, err
		}
		result[i] = item
	}
	return result, nil
}

// --- program.PresentationDeclaration ---

type wirePresentationDeclaration struct {
	Name                string             `json:"name"`
	Slot                string             `json:"slot"`
	Targets             json.RawMessage    `json:"targets"`
	Projection          string             `json:"projection"`
	ProjectionArguments []wireCallArgument `json:"projection_arguments"`
	View                string             `json:"view"`
}

// encodePresentationDeclaration encodes value, an ordinary (non-interface)
// struct, as its JSON wire representation.
func encodePresentationDeclaration(path string, value program.PresentationDeclaration) (json.RawMessage, error) {
	targets, err := encodeExpression(pathField(path, "targets"), value.Targets)
	if err != nil {
		return nil, err
	}
	arguments, err := encodeCallArguments(pathField(path, "projection_arguments"), value.ProjectionArguments)
	if err != nil {
		return nil, err
	}
	return json.Marshal(wirePresentationDeclaration{
		Name:                value.Name,
		Slot:                value.Slot,
		Targets:             targets,
		Projection:          value.Projection,
		ProjectionArguments: arguments,
		View:                value.View,
	})
}

// decodePresentationDeclaration decodes data as a program.PresentationDeclaration.
// JSON null is not a valid encoding and produces a path-aware structural
// error.
func decodePresentationDeclaration(path string, data json.RawMessage) (program.PresentationDeclaration, error) {
	var wire wirePresentationDeclaration
	if err := decodeOrdinaryObject(path, data, &wire); err != nil {
		return program.PresentationDeclaration{}, err
	}
	targets, err := decodeExpression(pathField(path, "targets"), wire.Targets)
	if err != nil {
		return program.PresentationDeclaration{}, err
	}
	arguments, err := decodeCallArguments(pathField(path, "projection_arguments"), wire.ProjectionArguments)
	if err != nil {
		return program.PresentationDeclaration{}, err
	}
	return program.PresentationDeclaration{
		Name:                wire.Name,
		Slot:                wire.Slot,
		Targets:             targets,
		Projection:          wire.Projection,
		ProjectionArguments: arguments,
		View:                wire.View,
	}, nil
}

func encodePresentationDeclarations(path string, items []program.PresentationDeclaration) ([]json.RawMessage, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]json.RawMessage, len(items))
	for i, item := range items {
		raw, err := encodePresentationDeclaration(pathIndex(path, i), item)
		if err != nil {
			return nil, err
		}
		result[i] = raw
	}
	return result, nil
}

func decodePresentationDeclarations(path string, items []json.RawMessage) ([]program.PresentationDeclaration, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]program.PresentationDeclaration, len(items))
	for i, raw := range items {
		item, err := decodePresentationDeclaration(pathIndex(path, i), raw)
		if err != nil {
			return nil, err
		}
		result[i] = item
	}
	return result, nil
}

// --- program.QuestionPresentationDeclaration (pointer helper) ---

type wireQuestionPresentationDeclaration struct {
	Slot                string             `json:"slot"`
	Projection          string             `json:"projection"`
	ProjectionArguments []wireCallArgument `json:"projection_arguments"`
	View                string             `json:"view"`
}

// encodeQuestionPresentationDeclaration encodes value as its JSON wire
// representation, or as JSON null when value is a nil pointer. Unlike the
// closed-interface codecs elsewhere in this package, this helper takes a
// concrete pointer type directly because program.QuestionSlotDeclaration and
// program.AskGroupSlotDeclaration both use *program.QuestionPresentationDeclaration to
// represent "no authored presentation" with nil, not a closed interface.
func encodeQuestionPresentationDeclaration(path string, value *program.QuestionPresentationDeclaration) (json.RawMessage, error) {
	if value == nil {
		return json.RawMessage("null"), nil
	}
	arguments, err := encodeCallArguments(pathField(path, "projection_arguments"), value.ProjectionArguments)
	if err != nil {
		return nil, err
	}
	return json.Marshal(wireQuestionPresentationDeclaration{
		Slot:                value.Slot,
		Projection:          value.Projection,
		ProjectionArguments: arguments,
		View:                value.View,
	})
}

// decodeQuestionPresentationDeclaration decodes data as a
// *program.QuestionPresentationDeclaration. Unlike other ordinary-object decoders
// in this package, JSON null is valid here and decodes to a nil pointer,
// matching the "headless question" semantics of a nil Presentation field.
func decodeQuestionPresentationDeclaration(path string, data json.RawMessage) (*program.QuestionPresentationDeclaration, error) {
	if isEmptyOrNull(data) {
		return nil, nil
	}
	raw, err := decodeTopLevelValue(path, data)
	if err != nil {
		return nil, err
	}
	var wire wireQuestionPresentationDeclaration
	if err := strictDecodeInto(path, raw, &wire); err != nil {
		return nil, err
	}
	arguments, err := decodeCallArguments(pathField(path, "projection_arguments"), wire.ProjectionArguments)
	if err != nil {
		return nil, err
	}
	return &program.QuestionPresentationDeclaration{
		Slot:                wire.Slot,
		Projection:          wire.Projection,
		ProjectionArguments: arguments,
		View:                wire.View,
	}, nil
}
