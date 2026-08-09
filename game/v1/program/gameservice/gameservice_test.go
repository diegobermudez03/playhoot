package gameservice_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/diegobermudez03/playhoot/game/v1/program"
	"github.com/diegobermudez03/playhoot/game/v1/program/gameservice"
)

func assertGenericJSONEqual(t *testing.T, a, b []byte) {
	t.Helper()
	var ga, gb any
	if err := json.Unmarshal(a, &ga); err != nil {
		t.Fatalf("unmarshal a: %v", err)
	}
	if err := json.Unmarshal(b, &gb); err != nil {
		t.Fatalf("unmarshal b: %v", err)
	}
	if !reflect.DeepEqual(ga, gb) {
		t.Fatalf("JSON values differ:\n  a = %s\n  b = %s", a, b)
	}
}

func TestEncodeDecodeJSON_MinimalDefinition(t *testing.T) {
	original := program.Definition{
		Metadata: program.Metadata{ID: "minimal", Name: "Minimal"},
	}
	data, err := gameservice.EncodeJSON(original)
	if err != nil {
		t.Fatalf("EncodeJSON: %v", err)
	}
	decoded, err := gameservice.DecodeJSON(data)
	if err != nil {
		t.Fatalf("DecodeJSON: %v", err)
	}
	if decoded == nil {
		t.Fatal("expected non-nil definition")
	}
	if decoded.Metadata != original.Metadata {
		t.Fatalf("metadata mismatch: %#v vs %#v", decoded.Metadata, original.Metadata)
	}
}

func buildCompleteDefinition() program.Definition {
	return program.Definition{
		Metadata: program.Metadata{
			ID: "card-picker", Name: "Card Picker", Description: "A tiny card-selection game",
			Version: "0.1.0", LanguageVersion: "1",
		},
		Types: []program.TypeDeclaration{
			program.EnumTypeDeclaration{Name: "Phase", Values: []program.EnumValueDeclaration{{Name: "PLAYING"}, {Name: "FINISHED"}}},
			program.RecordTypeDeclaration{Name: "CardRef", Fields: []program.FieldDeclaration{{Name: "id", Type: program.BuiltinTypeReference{Type: program.BuiltinTypeString}}}},
			program.UnionTypeDeclaration{
				Name: "TurnResult",
				Variants: []program.UnionVariantDeclaration{
					{Name: "Won", Fields: []program.FieldDeclaration{{Name: "participant", Type: program.NamedTypeReference{Name: "ParticipantId"}}}},
					{Name: "Ended", Fields: []program.FieldDeclaration{}},
				},
			},
			program.NewTypeDeclaration{Name: "ParticipantId", Underlying: program.BuiltinTypeReference{Type: program.BuiltinTypeString}},
			nil,
		},
		Resources: []program.ResourceDeclaration{
			{Name: "WinningScore", Type: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}, Value: program.NumberLiteralExpression{Value: "10"}},
		},
		GlobalState: program.StateDeclaration{
			Fields: []program.StateFieldDeclaration{
				{Name: "phase", Type: program.NamedTypeReference{Name: "Phase"}, Initializer: program.EnumValueExpression{TypeName: "Phase", ValueName: "PLAYING"}},
				{Name: "scores", Type: program.MapTypeReference{Key: program.NamedTypeReference{Name: "ParticipantId"}, Value: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}}},
			},
		},
		Functions: []program.FunctionDeclaration{
			{
				Name:       "HasWon",
				Parameters: []program.FieldDeclaration{{Name: "score", Type: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}}},
				ResultType: program.BuiltinTypeReference{Type: program.BuiltinTypeBool},
				Body: program.BinaryExpression{
					Operator: program.BinaryOperatorGreaterOrEqual,
					Left:     program.ReferenceExpression{Name: "score"},
					Right:    program.FieldExpression{Target: program.ReferenceExpression{Name: "resources"}, Field: "WinningScore"},
				},
			},
		},
		Invariants: []program.InvariantDeclaration{
			{
				Name: "NoNegativeScores",
				Condition: program.ListAllExpression{
					Collection: program.CallExpression{Function: "values", Arguments: []program.CallArgument{{Name: "map", Value: program.FieldExpression{Target: program.ReferenceExpression{Name: "global"}, Field: "scores"}}}},
					ItemName:   "score",
					Predicate:  program.BinaryExpression{Operator: program.BinaryOperatorGreaterOrEqual, Left: program.ReferenceExpression{Name: "score"}, Right: program.NumberLiteralExpression{Value: "0"}},
				},
			},
		},
		Projections: []program.ProjectionDeclaration{
			{
				Name:       "MyHand",
				ResultType: program.ListTypeReference{Element: program.NamedTypeReference{Name: "CardRef"}},
				Body: program.IndexExpression{
					Target: program.FieldExpression{Target: program.ReferenceExpression{Name: "global"}, Field: "hands"},
					Index:  program.ReferenceExpression{Name: "viewer"},
				},
			},
		},
		Views: []program.ViewDeclaration{
			{
				Name:      "HandView",
				ModelType: program.ListTypeReference{Element: program.NamedTypeReference{Name: "CardRef"}},
				Root: program.ContainerElement{
					Layout: program.StackLayout{},
					Children: []program.UIElement{
						program.TextElement{Value: program.StringLiteralExpression{Value: "Your hand"}},
					},
				},
			},
		},
		PresentationSlots: []program.PresentationSlotDeclaration{{Name: "main"}, {Name: "hud"}},
		UserIntents: []program.UserIntentDeclaration{
			{Name: "PlayCard", Parameters: []program.FieldDeclaration{{Name: "card", Type: program.NamedTypeReference{Name: "CardRef"}}}},
		},
		Questions: []program.QuestionDeclaration{
			{
				Name:         "ChooseCard",
				Parameters:   []program.FieldDeclaration{{Name: "options", Type: program.ListTypeReference{Element: program.NamedTypeReference{Name: "CardRef"}}}},
				ResponseType: program.NamedTypeReference{Name: "CardRef"},
				Validation: program.BinaryExpression{
					Operator: program.BinaryOperatorIn,
					Left:     program.ReferenceExpression{Name: "answer"},
					Right:    program.ReferenceExpression{Name: "options"},
				},
			},
		},
		Effects: []program.EffectDeclaration{
			{Name: "CardPlayed", Parameters: []program.FieldDeclaration{{Name: "card", Type: program.NamedTypeReference{Name: "CardRef"}}}},
		},
		RootWorkflow: "PlayRound",
		Workflows: []program.WorkflowDeclaration{
			{
				Name:         "PlayRound",
				ResultType:   program.NamedTypeReference{Name: "TurnResult"},
				ChildSlots:   []program.ChildWorkflowSlotDeclaration{{Name: "activeTurn", Workflow: "PlayTurn"}},
				InitialState: "Start",
				States: []program.WorkflowStateDeclaration{
					{
						Name: "Start",
						Transitions: []program.TransitionDeclaration{
							{
								Name:   "start",
								Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
								Operations: program.Block{
									Operations: []program.Operation{
										program.DrawRandomOperation{
											Name:      "firstCard",
											Generator: program.RandomElementGenerator{Collection: program.FieldExpression{Target: program.ReferenceExpression{Name: "global"}, Field: "deck"}},
										},
										program.MatchOperation{
											Value: program.EnumValueExpression{TypeName: "Phase", ValueName: "PLAYING"},
											Cases: []program.MatchOperationCase{
												{
													Pattern: program.WildcardMatchPattern{},
													Body: program.Block{
														Operations: []program.Operation{
															program.SpawnChildWorkflowOperation{Slot: "activeTurn", Arguments: []program.CallArgument{}},
														},
													},
												},
											},
										},
									},
								},
								Control: program.GotoControl{State: "Waiting"},
							},
						},
					},
					{
						Name: "Waiting",
						Transitions: []program.TransitionDeclaration{
							{
								Name:    "child_completed",
								Signal:  program.SignalPattern{Source: program.ChildCompletedSignalSource{Slot: "activeTurn"}, Bindings: []program.SignalBinding{{Field: "result", Name: "turnResult"}}},
								Control: program.CompleteControl{Result: program.ReferenceExpression{Name: "turnResult"}},
							},
						},
					},
				},
			},
			{
				Name:       "PlayTurn",
				ResultType: program.NamedTypeReference{Name: "TurnResult"},
				QuestionSlots: []program.QuestionSlotDeclaration{
					{
						Name:     "moveRequest",
						Question: "ChooseCard",
						Presentation: &program.QuestionPresentationDeclaration{
							Slot: "primaryInteraction", Projection: "MyHand", View: "HandView",
						},
					},
				},
				TimerSlots:   []program.TimerSlotDeclaration{{Name: "moveDeadline"}},
				InitialState: "RequestMove",
				States: []program.WorkflowStateDeclaration{
					{
						Name: "RequestMove",
						Transitions: []program.TransitionDeclaration{
							{
								Name:   "workflow_started",
								Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
								Operations: program.Block{
									Operations: []program.Operation{
										program.OpenQuestionOperation{Slot: "moveRequest", Recipient: program.ReferenceExpression{Name: "participant"}, Arguments: []program.CallArgument{}},
										program.ScheduleTimerOperation{Slot: "moveDeadline", DelayMilliseconds: program.NumberLiteralExpression{Value: "30000"}},
									},
								},
								Control: program.GotoControl{State: "WaitingForMove"},
							},
						},
					},
					{
						Name: "WaitingForMove",
						Transitions: []program.TransitionDeclaration{
							{
								Name:   "answered",
								Signal: program.SignalPattern{Source: program.QuestionAnsweredSignalSource{Slot: "moveRequest"}, Bindings: []program.SignalBinding{{Field: "answer", Name: "selectedCard"}}},
								Operations: program.Block{
									Operations: []program.Operation{program.CancelTimerOperation{Slot: "moveDeadline"}},
								},
								Control: program.CompleteControl{Result: program.UnionExpression{TypeName: "TurnResult", VariantName: "Ended"}},
							},
						},
					},
				},
			},
		},
	}
}

func TestEncodeDecodeJSON_CompleteDefinition_RoundTrip(t *testing.T) {
	original := buildCompleteDefinition()
	data, err := gameservice.EncodeJSON(original)
	if err != nil {
		t.Fatalf("EncodeJSON: %v", err)
	}
	decoded, err := gameservice.DecodeJSON(data)
	if err != nil {
		t.Fatalf("DecodeJSON: %v", err)
	}
	if !reflect.DeepEqual(original, *decoded) {
		t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", original, *decoded)
	}
}

func TestEncodeDecodeJSON_ReEncodingStability(t *testing.T) {
	original := buildCompleteDefinition()

	firstEncode, err := gameservice.EncodeJSON(original)
	if err != nil {
		t.Fatalf("EncodeJSON: %v", err)
	}
	decoded, err := gameservice.DecodeJSON(firstEncode)
	if err != nil {
		t.Fatalf("DecodeJSON: %v", err)
	}
	secondEncode, err := gameservice.EncodeJSON(*decoded)
	if err != nil {
		t.Fatalf("EncodeJSON (re-encode): %v", err)
	}
	assertGenericJSONEqual(t, firstEncode, secondEncode)

	thirdEncode, err := gameservice.EncodeJSON(original)
	if err != nil {
		t.Fatalf("EncodeJSON (direct repeat): %v", err)
	}
	if string(firstEncode) != string(thirdEncode) {
		t.Fatalf("expected identical bytes for repeated direct encoding:\n  first = %s\n  third = %s", firstEncode, thirdEncode)
	}
}

func TestExactJSON_RootShape(t *testing.T) {
	data, err := gameservice.EncodeJSON(buildCompleteDefinition())
	if err != nil {
		t.Fatalf("EncodeJSON: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	expected := []string{
		"metadata", "types", "resources", "global_state", "functions", "invariants",
		"projections", "views", "presentation_slots", "user_intents", "questions",
		"effects", "root_workflow", "workflows",
	}
	if len(obj) != len(expected) {
		t.Fatalf("expected %d root fields, got %d: %#v", len(expected), len(obj), obj)
	}
	for _, key := range expected {
		if _, ok := obj[key]; !ok {
			t.Fatalf("missing expected root field %q", key)
		}
	}
}

func TestEncodeDecodeJSON_NilVsEmptyTopLevelSlices(t *testing.T) {
	nilDef := program.Definition{Metadata: program.Metadata{ID: "x"}}
	data, err := gameservice.EncodeJSON(nilDef)
	if err != nil {
		t.Fatalf("EncodeJSON: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"types", "resources", "functions", "invariants", "projections", "views", "presentation_slots", "user_intents", "questions", "effects", "workflows"} {
		if obj[key] != nil {
			t.Fatalf("expected field %q to be null for nil slice, got %#v", key, obj[key])
		}
	}

	emptyDef := program.Definition{
		Metadata:          program.Metadata{ID: "x"},
		Types:             []program.TypeDeclaration{},
		Resources:         []program.ResourceDeclaration{},
		Functions:         []program.FunctionDeclaration{},
		Invariants:        []program.InvariantDeclaration{},
		Projections:       []program.ProjectionDeclaration{},
		Views:             []program.ViewDeclaration{},
		PresentationSlots: []program.PresentationSlotDeclaration{},
		UserIntents:       []program.UserIntentDeclaration{},
		Questions:         []program.QuestionDeclaration{},
		Effects:           []program.EffectDeclaration{},
		Workflows:         []program.WorkflowDeclaration{},
	}
	data, err = gameservice.EncodeJSON(emptyDef)
	if err != nil {
		t.Fatalf("EncodeJSON: %v", err)
	}
	obj = nil
	if err := json.Unmarshal(data, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"types", "resources", "functions", "invariants", "projections", "views", "presentation_slots", "user_intents", "questions", "effects", "workflows"} {
		arr, ok := obj[key].([]any)
		if !ok || len(arr) != 0 {
			t.Fatalf("expected field %q to be an empty array, got %#v", key, obj[key])
		}
	}

	decoded, err := gameservice.DecodeJSON(data)
	if err != nil {
		t.Fatalf("DecodeJSON: %v", err)
	}
	if decoded.Types == nil || decoded.Workflows == nil {
		t.Fatalf("expected empty non-nil slices preserved, got %#v", decoded)
	}
}

func TestEncodeDecodeJSON_NilInterfaceElementsInTypes(t *testing.T) {
	original := program.Definition{
		Metadata: program.Metadata{ID: "x"},
		Types:    []program.TypeDeclaration{nil, program.EnumTypeDeclaration{Name: "E"}, nil},
	}
	data, err := gameservice.EncodeJSON(original)
	if err != nil {
		t.Fatalf("EncodeJSON: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	types, ok := obj["types"].([]any)
	if !ok || len(types) != 3 || types[0] != nil || types[2] != nil {
		t.Fatalf("expected [null, {...}, null], got %#v", obj["types"])
	}
	decoded, err := gameservice.DecodeJSON(data)
	if err != nil {
		t.Fatalf("DecodeJSON: %v", err)
	}
	if decoded.Types[0] != nil || decoded.Types[2] != nil {
		t.Fatalf("expected nil TypeDeclaration entries preserved, got %#v", decoded.Types)
	}
	if _, ok := decoded.Types[1].(program.EnumTypeDeclaration); !ok {
		t.Fatalf("expected EnumTypeDeclaration at index 1, got %T", decoded.Types[1])
	}
}

func TestEncodeDecodeJSON_DuplicateOrdering(t *testing.T) {
	original := program.Definition{
		Metadata: program.Metadata{ID: "x"},
		Types: []program.TypeDeclaration{
			program.EnumTypeDeclaration{Name: "Dup"},
			program.EnumTypeDeclaration{Name: "Dup"},
		},
		Resources: []program.ResourceDeclaration{
			{Name: "Dup", Type: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}, Value: program.NumberLiteralExpression{Value: "1"}},
			{Name: "Dup", Type: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}, Value: program.NumberLiteralExpression{Value: "2"}},
		},
		Workflows: []program.WorkflowDeclaration{
			{Name: "Dup", InitialState: "S", States: []program.WorkflowStateDeclaration{{Name: "Dup"}, {Name: "Dup"}}},
			{Name: "Dup", InitialState: "S"},
		},
	}
	data, err := gameservice.EncodeJSON(original)
	if err != nil {
		t.Fatalf("EncodeJSON: %v", err)
	}
	decoded, err := gameservice.DecodeJSON(data)
	if err != nil {
		t.Fatalf("DecodeJSON: %v", err)
	}
	if !reflect.DeepEqual(original, *decoded) {
		t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", original, *decoded)
	}
}

func TestDecodeJSON_TopLevelNull(t *testing.T) {
	_, err := gameservice.DecodeJSON([]byte("null"))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *gameservice.DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$" {
		t.Fatalf("expected path $, got %q", decodeErr.Path)
	}
}

func TestDecodeJSON_TopLevelNonObject(t *testing.T) {
	cases := [][]byte{[]byte(`[]`), []byte(`"game"`), []byte(`42`), []byte(`true`)}
	for _, data := range cases {
		_, err := gameservice.DecodeJSON(data)
		if err == nil {
			t.Fatalf("expected an error for %s", data)
		}
		var decodeErr *gameservice.DecodeError
		if !errors.As(err, &decodeErr) {
			t.Fatalf("expected *DecodeError for %s, got %T: %v", data, err, err)
		}
		if decodeErr.Path != "$" {
			t.Fatalf("expected path $ for %s, got %q", data, decodeErr.Path)
		}
	}
}

func TestDecodeJSON_MalformedJSON(t *testing.T) {
	_, err := gameservice.DecodeJSON([]byte(`{"metadata":`))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *gameservice.DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$" {
		t.Fatalf("expected path $, got %q", decodeErr.Path)
	}
}

func TestDecodeJSON_TrailingData(t *testing.T) {
	valid, err := gameservice.EncodeJSON(program.Definition{Metadata: program.Metadata{ID: "x"}})
	if err != nil {
		t.Fatalf("EncodeJSON: %v", err)
	}

	trailing := append(append([]byte{}, valid...), []byte(` {"metadata":{}}`)...)
	if _, err := gameservice.DecodeJSON(trailing); err == nil {
		t.Fatal("expected an error for trailing JSON value")
	}

	trailingGarbage := append(append([]byte{}, valid...), []byte(`garbage`)...)
	if _, err := gameservice.DecodeJSON(trailingGarbage); err == nil {
		t.Fatal("expected an error for trailing non-whitespace data")
	}

	trailingWhitespace := append(append([]byte{}, valid...), []byte("  \n\t")...)
	if _, err := gameservice.DecodeJSON(trailingWhitespace); err != nil {
		t.Fatalf("expected trailing whitespace to be allowed, got %v", err)
	}
}

func TestDecodeJSON_UnknownRootField(t *testing.T) {
	_, err := gameservice.DecodeJSON([]byte(`{
		"metadata": {"id":"","name":"","description":"","version":"","language_version":""},
		"types": [], "resources": [], "global_state": {"fields": []},
		"functions": [], "invariants": [], "projections": [], "views": [],
		"presentation_slots": [], "user_intents": [], "questions": [], "effects": [],
		"root_workflow": "", "workflows": [],
		"unexpected": true
	}`))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *gameservice.DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$" {
		t.Fatalf("expected path $, got %q", decodeErr.Path)
	}
}

func TestDecodeJSON_UnknownNestedField(t *testing.T) {
	_, err := gameservice.DecodeJSON([]byte(`{
		"metadata": {"id":"","name":"","description":"","version":"","language_version":""},
		"types": [], "resources": [], "global_state": {"fields": []},
		"functions": [], "invariants": [], "projections": [], "views": [],
		"presentation_slots": [], "user_intents": [], "questions": [], "effects": [],
		"root_workflow": "", "workflows": [
			{
				"name": "W", "parameters": [], "result_type": null, "local_state": {"fields": []},
				"question_slots": [], "ask_group_slots": [], "timer_slots": [], "child_slots": [],
				"task_group_slots": [], "presentations": [], "initial_state": "S",
				"global_transitions": [],
				"states": [
					{
						"name": "S", "presentations": [],
						"transitions": [
							{
								"name": "t",
								"signal": {"source": {"kind": "named", "name": "WorkflowStarted"}, "bindings": []},
								"guard": null, "operations": {"operations": []}, "control": null,
								"unexpected": true
							}
						]
					}
				]
			}
		]
	}`))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *gameservice.DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$.workflows[0].states[0].transitions[0]" {
		t.Fatalf("expected path $.workflows[0].states[0].transitions[0], got %q", decodeErr.Path)
	}
}

func TestDecodeJSON_DeepNestedInvalidExpression(t *testing.T) {
	_, err := gameservice.DecodeJSON([]byte(`{
		"metadata": {"id":"","name":"","description":"","version":"","language_version":""},
		"types": [], "resources": [], "global_state": {"fields": []},
		"functions": [], "invariants": [], "projections": [], "views": [],
		"presentation_slots": [], "user_intents": [], "questions": [], "effects": [],
		"root_workflow": "", "workflows": [
			{
				"name": "W", "parameters": [], "result_type": null, "local_state": {"fields": []},
				"question_slots": [], "ask_group_slots": [], "timer_slots": [], "child_slots": [],
				"task_group_slots": [], "presentations": [], "initial_state": "S",
				"global_transitions": [],
				"states": [
					{"name": "A", "presentations": [], "transitions": []},
					{
						"name": "B", "presentations": [],
						"transitions": [
							{
								"name": "t",
								"signal": {"source": {"kind": "named", "name": "WorkflowStarted"}, "bindings": []},
								"guard": null,
								"operations": {"operations": [
									{"kind": "close_question", "slot": "a"},
									{"kind": "close_question", "slot": "b"},
									{"kind": "set", "target": {"kind": "name", "name": "x"}, "value": {"kind": "not_a_real_expression"}}
								]},
								"control": null
							}
						]
					}
				]
			}
		]
	}`))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *gameservice.DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$.workflows[0].states[1].transitions[0].operations.operations[2].value" {
		t.Fatalf("expected path $.workflows[0].states[1].transitions[0].operations.operations[2].value, got %q", decodeErr.Path)
	}
}

func TestDecodeJSON_DeepNestedInvalidTaskGroupKeyType(t *testing.T) {
	_, err := gameservice.DecodeJSON([]byte(`{
		"metadata": {"id":"","name":"","description":"","version":"","language_version":""},
		"types": [], "resources": [], "global_state": {"fields": []},
		"functions": [], "invariants": [], "projections": [], "views": [],
		"presentation_slots": [], "user_intents": [], "questions": [], "effects": [],
		"root_workflow": "", "workflows": [
			{
				"name": "W", "parameters": [], "result_type": null, "local_state": {"fields": []},
				"question_slots": [], "ask_group_slots": [], "timer_slots": [], "child_slots": [],
				"task_group_slots": [{"name": "s", "workflow": "w", "key_type": {"kind": "not_a_real_type"}}],
				"presentations": [], "initial_state": "S",
				"global_transitions": [], "states": []
			}
		]
	}`))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *gameservice.DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$.workflows[0].task_group_slots[0].key_type" {
		t.Fatalf("expected path $.workflows[0].task_group_slots[0].key_type, got %q", decodeErr.Path)
	}
}

func TestDecodeJSON_SemanticInvalidityRemainsDecodable(t *testing.T) {
	data := []byte(`{
		"metadata": {"id":"x","name":"","description":"","version":"","language_version":""},
		"types": [], "resources": [], "global_state": {"fields": []},
		"functions": [
			{"name": "Recursive", "parameters": [], "result_type": {"kind":"builtin","type":"bool"},
			 "body": {"kind": "call", "function": "Recursive", "arguments": []}}
		],
		"invariants": [
			{"name": "NumericGuardLike", "condition": {"kind": "number_literal", "value": "1"}}
		],
		"projections": [], "views": [], "presentation_slots": [],
		"user_intents": [], "questions": [
			{"name": "Q", "parameters": [], "response_type": null, "validation": null}
		],
		"effects": [],
		"root_workflow": "DoesNotExist",
		"workflows": [
			{
				"name": "Duplicate", "parameters": [], "result_type": null, "local_state": {"fields": []},
				"question_slots": [{"name": "unknownQ", "question": "NoSuchQuestion", "presentation": null}],
				"ask_group_slots": [], "timer_slots": [], "child_slots": [],
				"task_group_slots": [], "presentations": [], "initial_state": "UnknownState",
				"global_transitions": [],
				"states": [
					{"name": "S", "presentations": [], "transitions": [
						{
							"name": "t",
							"signal": {"source": {"kind": "named", "name": "WorkflowStarted"}, "bindings": []},
							"guard": {"kind": "number_literal", "value": "1"},
							"operations": {"operations": [
								{"kind": "begin_task_group", "slot": "notSealed", "completion": {"kind": "all_terminal"}}
							]},
							"control": {"kind": "goto", "state": "UnknownTarget"}
						}
					]}
				]
			},
			{"name": "Duplicate", "parameters": [], "result_type": null, "local_state": {"fields": []},
			 "question_slots": [], "ask_group_slots": [], "timer_slots": [], "child_slots": [],
			 "task_group_slots": [], "presentations": [], "initial_state": "S",
			 "global_transitions": [], "states": []}
		]
	}`)
	decoded, err := gameservice.DecodeJSON(data)
	if err != nil {
		t.Fatalf("expected structural success, got %v", err)
	}
	if decoded.RootWorkflow != "DoesNotExist" {
		t.Fatalf("expected unknown root workflow preserved, got %q", decoded.RootWorkflow)
	}
	if len(decoded.Workflows) != 2 || decoded.Workflows[0].Name != decoded.Workflows[1].Name {
		t.Fatalf("expected duplicate workflow names preserved, got %#v", decoded.Workflows)
	}
	if decoded.Workflows[0].InitialState != "UnknownState" {
		t.Fatalf("expected unknown initial state preserved, got %q", decoded.Workflows[0].InitialState)
	}
}

func TestDecodeJSON_ZeroValueDefinition(t *testing.T) {
	data, err := gameservice.EncodeJSON(program.Definition{})
	if err != nil {
		t.Fatalf("EncodeJSON: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"metadata", "types", "resources", "global_state", "functions", "invariants", "projections", "views", "presentation_slots", "user_intents", "questions", "effects", "root_workflow", "workflows"} {
		if _, ok := obj[key]; !ok {
			t.Fatalf("missing expected canonical field %q", key)
		}
	}
	if _, ok := obj["metadata"].(map[string]any); !ok {
		t.Fatalf("expected metadata to encode as an object, got %#v", obj["metadata"])
	}
	if _, ok := obj["global_state"].(map[string]any); !ok {
		t.Fatalf("expected global_state to encode as an object, got %#v", obj["global_state"])
	}

	decoded, err := gameservice.DecodeJSON(data)
	if err != nil {
		t.Fatalf("DecodeJSON: %v", err)
	}
	if decoded == nil {
		t.Fatal("expected a non-nil definition")
	}
}
