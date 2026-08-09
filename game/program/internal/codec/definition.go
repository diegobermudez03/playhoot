package codec

import "github.com/diegobermudez03/playhoot/game/program"

import "encoding/json"

func encodeTypeDeclarationSlice(path string, items []program.TypeDeclaration) ([]json.RawMessage, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]json.RawMessage, len(items))
	for i, item := range items {
		raw, err := encodeTypeDeclaration(pathIndex(path, i), item)
		if err != nil {
			return nil, err
		}
		result[i] = raw
	}
	return result, nil
}

func decodeTypeDeclarationSlice(path string, items []json.RawMessage) ([]program.TypeDeclaration, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]program.TypeDeclaration, len(items))
	for i, raw := range items {
		item, err := decodeTypeDeclaration(pathIndex(path, i), raw)
		if err != nil {
			return nil, err
		}
		result[i] = item
	}
	return result, nil
}

// wireDefinition is the private root wire schema. Types uses
// json.RawMessage because program.TypeDeclaration is a closed interface whose
// entries may be JSON null; every other declaration slice holds ordinary
// value objects, decoded strictly (including rejecting a JSON null
// element) by that declaration's own private decoder.
type wireDefinition struct {
	Metadata          json.RawMessage   `json:"metadata"`
	Types             []json.RawMessage `json:"types"`
	Resources         []json.RawMessage `json:"resources"`
	GlobalState       json.RawMessage   `json:"global_state"`
	Functions         []json.RawMessage `json:"functions"`
	Invariants        []json.RawMessage `json:"invariants"`
	Projections       []json.RawMessage `json:"projections"`
	Views             []json.RawMessage `json:"views"`
	PresentationSlots []json.RawMessage `json:"presentation_slots"`
	UserIntents       []json.RawMessage `json:"user_intents"`
	Questions         []json.RawMessage `json:"questions"`
	Effects           []json.RawMessage `json:"effects"`
	RootWorkflow      string            `json:"root_workflow"`
	Workflows         []json.RawMessage `json:"workflows"`
}

// encodeDefinition encodes value, an ordinary (non-interface) struct, as
// its JSON wire representation, using the canonical field order metadata,
// types, resources, global_state, functions, invariants, projections,
// views, presentation_slots, user_intents, questions, effects,
// root_workflow, workflows. program.Metadata and GlobalState always encode as
// ordinary objects, never JSON null.
func EncodeDefinition(path string, value program.Definition) (json.RawMessage, error) {
	metadata, err := encodeMetadata(pathField(path, "metadata"), value.Metadata)
	if err != nil {
		return nil, err
	}
	types, err := encodeTypeDeclarationSlice(pathField(path, "types"), value.Types)
	if err != nil {
		return nil, err
	}
	resources, err := encodeResourceDeclarations(pathField(path, "resources"), value.Resources)
	if err != nil {
		return nil, err
	}
	globalState, err := encodeStateDeclaration(pathField(path, "global_state"), value.GlobalState)
	if err != nil {
		return nil, err
	}
	functions, err := encodeFunctionDeclarations(pathField(path, "functions"), value.Functions)
	if err != nil {
		return nil, err
	}
	invariants, err := encodeInvariantDeclarations(pathField(path, "invariants"), value.Invariants)
	if err != nil {
		return nil, err
	}
	projections, err := encodeProjectionDeclarations(pathField(path, "projections"), value.Projections)
	if err != nil {
		return nil, err
	}
	views, err := encodeViewDeclarations(pathField(path, "views"), value.Views)
	if err != nil {
		return nil, err
	}
	presentationSlots, err := encodePresentationSlotDeclarations(pathField(path, "presentation_slots"), value.PresentationSlots)
	if err != nil {
		return nil, err
	}
	userIntents, err := encodeUserIntentDeclarations(pathField(path, "user_intents"), value.UserIntents)
	if err != nil {
		return nil, err
	}
	questions, err := encodeQuestionDeclarations(pathField(path, "questions"), value.Questions)
	if err != nil {
		return nil, err
	}
	effects, err := encodeEffectDeclarations(pathField(path, "effects"), value.Effects)
	if err != nil {
		return nil, err
	}
	workflows, err := encodeWorkflowDeclarations(pathField(path, "workflows"), value.Workflows)
	if err != nil {
		return nil, err
	}
	return json.Marshal(wireDefinition{
		Metadata:          metadata,
		Types:             types,
		Resources:         resources,
		GlobalState:       globalState,
		Functions:         functions,
		Invariants:        invariants,
		Projections:       projections,
		Views:             views,
		PresentationSlots: presentationSlots,
		UserIntents:       userIntents,
		Questions:         questions,
		Effects:           effects,
		RootWorkflow:      value.RootWorkflow,
		Workflows:         workflows,
	})
}

// decodeDefinition decodes data as a program.Definition. JSON null is not a valid
// encoding of the root definition and produces a path-aware structural
// error at path; program.Metadata and GlobalState must always be ordinary objects
// (never null).
func DecodeDefinition(path string, data json.RawMessage) (program.Definition, error) {
	var wire wireDefinition
	if err := decodeOrdinaryObject(path, data, &wire); err != nil {
		return program.Definition{}, err
	}
	metadata, err := decodeMetadata(pathField(path, "metadata"), wire.Metadata)
	if err != nil {
		return program.Definition{}, err
	}
	types, err := decodeTypeDeclarationSlice(pathField(path, "types"), wire.Types)
	if err != nil {
		return program.Definition{}, err
	}
	resources, err := decodeResourceDeclarations(pathField(path, "resources"), wire.Resources)
	if err != nil {
		return program.Definition{}, err
	}
	globalState, err := decodeStateDeclaration(pathField(path, "global_state"), wire.GlobalState)
	if err != nil {
		return program.Definition{}, err
	}
	functions, err := decodeFunctionDeclarations(pathField(path, "functions"), wire.Functions)
	if err != nil {
		return program.Definition{}, err
	}
	invariants, err := decodeInvariantDeclarations(pathField(path, "invariants"), wire.Invariants)
	if err != nil {
		return program.Definition{}, err
	}
	projections, err := decodeProjectionDeclarations(pathField(path, "projections"), wire.Projections)
	if err != nil {
		return program.Definition{}, err
	}
	views, err := decodeViewDeclarations(pathField(path, "views"), wire.Views)
	if err != nil {
		return program.Definition{}, err
	}
	presentationSlots, err := decodePresentationSlotDeclarations(pathField(path, "presentation_slots"), wire.PresentationSlots)
	if err != nil {
		return program.Definition{}, err
	}
	userIntents, err := decodeUserIntentDeclarations(pathField(path, "user_intents"), wire.UserIntents)
	if err != nil {
		return program.Definition{}, err
	}
	questions, err := decodeQuestionDeclarations(pathField(path, "questions"), wire.Questions)
	if err != nil {
		return program.Definition{}, err
	}
	effects, err := decodeEffectDeclarations(pathField(path, "effects"), wire.Effects)
	if err != nil {
		return program.Definition{}, err
	}
	workflows, err := decodeWorkflowDeclarations(pathField(path, "workflows"), wire.Workflows)
	if err != nil {
		return program.Definition{}, err
	}
	return program.Definition{
		Metadata:          metadata,
		Types:             types,
		Resources:         resources,
		GlobalState:       globalState,
		Functions:         functions,
		Invariants:        invariants,
		Projections:       projections,
		Views:             views,
		PresentationSlots: presentationSlots,
		UserIntents:       userIntents,
		Questions:         questions,
		Effects:           effects,
		RootWorkflow:      wire.RootWorkflow,
		Workflows:         workflows,
	}, nil
}
