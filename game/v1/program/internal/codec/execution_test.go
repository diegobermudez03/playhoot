package codec

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/diegobermudez03/playhoot/game/v1/program"
)

// --- assignment target ---

func TestAssignmentTarget_RoundTrip_DeeplyNested(t *testing.T) {
	original := program.FieldTarget{
		Target: program.IndexTarget{
			Target: program.FieldTarget{
				Target: program.NameTarget{Name: "global"},
				Field:  "participants",
			},
			Index: program.ReferenceExpression{Name: "currentUser"},
		},
		Field: "score",
	}

	raw, err := encodeAssignmentTarget("$", original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeAssignmentTarget("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", original, decoded)
	}
}

func TestAssignmentTarget_AllVariants(t *testing.T) {
	cases := []program.AssignmentTarget{
		program.NameTarget{Name: "global"},
		program.FieldTarget{Target: program.NameTarget{Name: "global"}, Field: "participants"},
		program.IndexTarget{Target: program.NameTarget{Name: "turnOrder"}, Index: program.NumberLiteralExpression{Value: "0"}},
	}
	for _, original := range cases {
		raw, err := encodeAssignmentTarget("$", original)
		if err != nil {
			t.Fatalf("encode %T: %v", original, err)
		}
		decoded, err := decodeAssignmentTarget("$", raw)
		if err != nil {
			t.Fatalf("decode %T: %v", original, err)
		}
		if !reflect.DeepEqual(original, decoded) {
			t.Fatalf("round trip mismatch for %T:\n  original = %#v\n  decoded  = %#v", original, original, decoded)
		}
	}
}

// --- block ---

func TestBlock_RoundTrip_NilVsEmptyOperations(t *testing.T) {
	nilBlock := program.Block{Operations: nil}
	raw, err := encodeBlock("$", nilBlock)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["operations"] != nil {
		t.Fatalf("expected null operations, got %#v", obj["operations"])
	}
	decoded, err := decodeBlock("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Operations != nil {
		t.Fatalf("expected nil operations preserved, got %#v", decoded.Operations)
	}

	emptyBlock := program.Block{Operations: []program.Operation{}}
	raw, err = encodeBlock("$", emptyBlock)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	obj = nil
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	ops, ok := obj["operations"].([]any)
	if !ok || len(ops) != 0 {
		t.Fatalf("expected empty array operations, got %#v", obj["operations"])
	}
	decoded, err = decodeBlock("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Operations == nil || len(decoded.Operations) != 0 {
		t.Fatalf("expected non-nil empty operations preserved, got %#v", decoded.Operations)
	}
}

func TestBlock_RoundTrip_OrderedOperations(t *testing.T) {
	original := program.Block{
		Operations: []program.Operation{
			program.SetOperation{Target: program.NameTarget{Name: "a"}, Value: program.NumberLiteralExpression{Value: "1"}},
			program.SetOperation{Target: program.NameTarget{Name: "b"}, Value: program.NumberLiteralExpression{Value: "2"}},
			program.SetOperation{Target: program.NameTarget{Name: "c"}, Value: program.NumberLiteralExpression{Value: "3"}},
		},
	}
	raw, err := encodeBlock("$", original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeBlock("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(decoded.Operations) != 3 {
		t.Fatalf("expected 3 operations, got %d", len(decoded.Operations))
	}
	for i, name := range []string{"a", "b", "c"} {
		op := decoded.Operations[i].(program.SetOperation)
		if op.Target.(program.NameTarget).Name != name {
			t.Fatalf("operation %d: expected target %q, got %q", i, name, op.Target.(program.NameTarget).Name)
		}
	}
}

func TestBlock_RoundTrip_DeeplyNested(t *testing.T) {
	original := program.Block{
		Operations: []program.Operation{
			program.IfOperation{
				Condition: program.ReferenceExpression{Name: "condition"},
				Then: program.Block{
					Operations: []program.Operation{
						program.ForEachOperation{
							Collection: program.ReferenceExpression{Name: "participants"},
							ItemName:   "participant",
							Body: program.Block{
								Operations: []program.Operation{
									program.MatchOperation{
										Value: program.ReferenceExpression{Name: "participant"},
										Cases: []program.MatchOperationCase{
											{
												Pattern: program.WildcardMatchPattern{},
												Body: program.Block{
													Operations: []program.Operation{
														program.SetOperation{Target: program.NameTarget{Name: "found"}, Value: program.BoolLiteralExpression{Value: true}},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				Else: program.Block{},
			},
		},
	}
	raw, err := encodeBlock("$", original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeBlock("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", original, decoded)
	}
}

func TestBlock_DecodeNull_IsStructuralError(t *testing.T) {
	_, err := decodeBlock("$", json.RawMessage("null"))
	if err == nil {
		t.Fatal("expected an error decoding null as program.Block")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

// --- operations: exhaustive table ---

func allOperationVariants() []program.Operation {
	return []program.Operation{
		program.LetOperation{
			Name:  "participant",
			Type:  program.NamedTypeReference{Name: "Participant"},
			Value: program.ReferenceExpression{Name: "currentUser"},
		},
		program.SetOperation{
			Target: program.FieldTarget{Target: program.NameTarget{Name: "global"}, Field: "turnNumber"},
			Value:  program.NumberLiteralExpression{Value: "1"},
		},
		program.ListAppendOperation{
			Target: program.FieldTarget{Target: program.NameTarget{Name: "global"}, Field: "turnOrder"},
			Value:  program.ReferenceExpression{Name: "currentUser"},
		},
		program.ListInsertOperation{
			Target: program.FieldTarget{Target: program.NameTarget{Name: "global"}, Field: "turnOrder"},
			Index:  program.NumberLiteralExpression{Value: "0"},
			Value:  program.ReferenceExpression{Name: "currentUser"},
		},
		program.ListRemoveAtOperation{
			Target: program.NameTarget{Name: "turnOrder"},
			Index:  program.NumberLiteralExpression{Value: "2"},
		},
		program.MapPutOperation{
			Target: program.FieldTarget{Target: program.NameTarget{Name: "global"}, Field: "scores"},
			Key:    program.ReferenceExpression{Name: "currentUser"},
			Value:  program.NumberLiteralExpression{Value: "10"},
		},
		program.MapDeleteOperation{
			Target: program.FieldTarget{Target: program.NameTarget{Name: "global"}, Field: "scores"},
			Key:    program.ReferenceExpression{Name: "currentUser"},
		},
		program.IfOperation{
			Condition: program.ReferenceExpression{Name: "condition"},
			Then:      program.Block{Operations: []program.Operation{}},
			Else:      program.Block{Operations: []program.Operation{}},
		},
		program.ForEachOperation{
			Collection: program.ReferenceExpression{Name: "participants"},
			ItemName:   "participant",
			IndexName:  "index",
			Body:       program.Block{Operations: []program.Operation{}},
		},
		program.MatchOperation{
			Value: program.ReferenceExpression{Name: "turnResult"},
			Cases: []program.MatchOperationCase{
				{
					Pattern: program.UnionVariantMatchPattern{
						TypeName:    "TurnResult",
						VariantName: "Won",
						Bindings:    []program.MatchFieldBinding{{Field: "participant", Name: "winner"}},
					},
					Body: program.Block{
						Operations: []program.Operation{
							program.SetOperation{
								Target: program.FieldTarget{Target: program.NameTarget{Name: "global"}, Field: "winner"},
								Value:  program.ReferenceExpression{Name: "winner"},
							},
						},
					},
				},
			},
		},
		program.OpenQuestionOperation{
			Slot:      "moveRequest",
			Recipient: program.ReferenceExpression{Name: "participant"},
			Arguments: []program.CallArgument{
				{Name: "plans", Value: program.FieldExpression{Target: program.ReferenceExpression{Name: "local"}, Field: "availablePlans"}},
			},
		},
		program.CloseQuestionOperation{Slot: "moveRequest"},
		program.EmitEffectOperation{
			Effect:     "DiceRolled",
			Recipients: program.ReferenceExpression{Name: "roomUsers"},
			Arguments: []program.CallArgument{
				{Name: "first", Value: program.ReferenceExpression{Name: "firstDie"}},
				{Name: "second", Value: program.ReferenceExpression{Name: "secondDie"}},
			},
		},
		program.ScheduleTimerOperation{
			Slot:              "moveDeadline",
			DelayMilliseconds: program.NumberLiteralExpression{Value: "30000"},
		},
		program.CancelTimerOperation{Slot: "moveDeadline"},
		program.SpawnChildWorkflowOperation{
			Slot: "activeTurn",
			Arguments: []program.CallArgument{
				{Name: "participant", Value: program.ReferenceExpression{Name: "participantId"}},
			},
		},
		program.CancelChildWorkflowOperation{
			Slot:   "activeTurn",
			Reason: program.StringLiteralExpression{Value: "participant disconnected"},
		},
		program.OpenAskGroupOperation{
			Slot:       "cardVotes",
			Recipients: program.FieldExpression{Target: program.ReferenceExpression{Name: "team"}, Field: "users"},
			Arguments: []program.CallArgument{
				{Name: "options", Value: program.FieldExpression{Target: program.ReferenceExpression{Name: "local"}, Field: "availableCards"}},
			},
			Completion: program.AskGroupQuorumPolicy{Count: program.ReferenceExpression{Name: "requiredVotes"}},
		},
		program.FinalizeAskGroupOperation{Slot: "cardVotes"},
		program.CancelAskGroupOperation{Slot: "cardVotes"},
		program.BeginTaskGroupOperation{
			Slot:       "teamSelections",
			Completion: program.TaskGroupAllTerminalPolicy{},
		},
		program.SpawnTaskGroupChildOperation{
			Slot: "teamSelections",
			Key:  program.FieldExpression{Target: program.ReferenceExpression{Name: "team"}, Field: "id"},
			Arguments: []program.CallArgument{
				{Name: "team", Value: program.ReferenceExpression{Name: "team"}},
			},
		},
		program.SealTaskGroupOperation{Slot: "teamSelections"},
		program.FinalizeTaskGroupOperation{Slot: "teamSelections"},
		program.CancelTaskGroupOperation{
			Slot:   "teamSelections",
			Reason: program.StringLiteralExpression{Value: "round cancelled"},
		},
		program.DrawRandomOperation{
			Name: "firstDie",
			Generator: program.RandomIntegerGenerator{
				Minimum: program.NumberLiteralExpression{Value: "1"},
				Maximum: program.NumberLiteralExpression{Value: "6"},
			},
		},
	}
}

// wantedOperationKinds must match every discriminator declared by this
// codec increment. If a variant is added to program without a matching
// entry here and in allOperationVariants, this test fails loudly instead
// of silently under-covering the codec.
var wantedOperationKinds = []string{
	"let", "set",
	"list_append", "list_insert", "list_remove_at",
	"map_put", "map_delete",
	"if", "for_each", "match",
	"open_question", "close_question", "emit_effect",
	"schedule_timer", "cancel_timer",
	"spawn_child_workflow", "cancel_child_workflow",
	"open_ask_group", "finalize_ask_group", "cancel_ask_group",
	"begin_task_group", "spawn_task_group_child", "seal_task_group", "finalize_task_group", "cancel_task_group",
	"draw_random",
}

func TestOperation_AllVariants_RoundTrip(t *testing.T) {
	variants := allOperationVariants()
	if len(variants) != len(wantedOperationKinds) {
		t.Fatalf("expected %d operation variants covered, got %d — update allOperationVariants to cover every program.Operation kind", len(wantedOperationKinds), len(variants))
	}

	seenKinds := make(map[string]bool)
	for _, original := range variants {
		t.Run(reflect.TypeOf(original).Name(), func(t *testing.T) {
			raw, err := encodeOperation("$", original)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}

			var obj map[string]any
			if err := json.Unmarshal(raw, &obj); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			kind, _ := obj["kind"].(string)
			seenKinds[kind] = true

			decoded, err := decodeOperation("$", raw)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if reflect.TypeOf(decoded) != reflect.TypeOf(original) {
				t.Fatalf("expected decoded type %T, got %T", original, decoded)
			}
			if !reflect.DeepEqual(original, decoded) {
				t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", original, decoded)
			}

			reencoded, err := encodeOperation("$", decoded)
			if err != nil {
				t.Fatalf("re-encode: %v", err)
			}
			assertSemanticJSONEqual(t, raw, reencoded)
		})
	}

	for _, wanted := range wantedOperationKinds {
		if !seenKinds[wanted] {
			t.Errorf("operation kind %q was not exercised by allOperationVariants", wanted)
		}
	}
}

// --- workflow control ---

func TestWorkflowControl_AllVariants_RoundTrip(t *testing.T) {
	variants := []program.WorkflowControl{
		program.GotoControl{State: "WaitingForMove"},
		program.StayControl{},
		program.CompleteControl{Result: program.ReferenceExpression{Name: "selectedMove"}},
		program.FailControl{Error: program.StringLiteralExpression{Value: "unable to resolve turn"}},
		program.CancelControl{Reason: program.StringLiteralExpression{Value: "participant disconnected"}},
		program.ConditionalControl{
			Condition: program.ReferenceExpression{Name: "hasWinner"},
			Then:      program.CompleteControl{Result: program.ReferenceExpression{Name: "winner"}},
			Else:      program.GotoControl{State: "NextRound"},
		},
		program.MatchControl{
			Value: program.ReferenceExpression{Name: "turnResult"},
			Cases: []program.MatchControlCase{
				{
					Pattern: program.UnionVariantMatchPattern{
						TypeName:    "TurnResult",
						VariantName: "Won",
						Bindings:    []program.MatchFieldBinding{{Field: "participant", Name: "winner"}},
					},
					Control: program.CompleteControl{Result: program.ReferenceExpression{Name: "winner"}},
				},
				{
					Pattern: program.WildcardMatchPattern{},
					Control: program.GotoControl{State: "NextTurn"},
				},
			},
		},
	}

	for _, original := range variants {
		t.Run(reflect.TypeOf(original).Name(), func(t *testing.T) {
			raw, err := encodeWorkflowControl("$", original)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			decoded, err := decodeWorkflowControl("$", raw)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if !reflect.DeepEqual(original, decoded) {
				t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", original, decoded)
			}
			reencoded, err := encodeWorkflowControl("$", decoded)
			if err != nil {
				t.Fatalf("re-encode: %v", err)
			}
			assertSemanticJSONEqual(t, raw, reencoded)
		})
	}
}

// --- signal source ---

func TestSignalSource_AllVariants_RoundTrip(t *testing.T) {
	variants := []program.SignalSource{
		program.NamedSignalSource{Name: "WorkflowStarted"},
		program.UserIntentSignalSource{Intent: "PlayCard"},
		program.QuestionAnsweredSignalSource{Slot: "moveRequest"},
		program.TimerExpiredSignalSource{Slot: "moveDeadline"},
		program.ChildCompletedSignalSource{Slot: "activeTurn"},
		program.ChildFailedSignalSource{Slot: "activeTurn"},
		program.ChildCancelledSignalSource{Slot: "activeTurn"},
		program.AskGroupCompletedSignalSource{Slot: "cardVotes"},
		program.TaskGroupCompletedSignalSource{Slot: "teamSelections"},
	}

	for _, original := range variants {
		t.Run(reflect.TypeOf(original).Name(), func(t *testing.T) {
			raw, err := encodeSignalSource("$", original)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			decoded, err := decodeSignalSource("$", raw)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if !reflect.DeepEqual(original, decoded) {
				t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", original, decoded)
			}
		})
	}
}

// --- signal pattern ---

func TestSignalPattern_RoundTrip(t *testing.T) {
	original := program.SignalPattern{
		Source: program.UserIntentSignalSource{Intent: "PlayCard"},
		Bindings: []program.SignalBinding{
			{Field: "actor", Name: "user"},
			{Field: "card", Name: "selectedCard"},
			{Field: "card", Name: "duplicateBinding"},
		},
	}
	raw, err := encodeSignalPattern("$", original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeSignalPattern("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", original, decoded)
	}
}

func TestSignalPattern_NilVsEmptyBindings(t *testing.T) {
	nilBindings := program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}, Bindings: nil}
	raw, err := encodeSignalPattern("$", nilBindings)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeSignalPattern("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Bindings != nil {
		t.Fatalf("expected nil bindings preserved, got %#v", decoded.Bindings)
	}

	emptyBindings := program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}, Bindings: []program.SignalBinding{}}
	raw, err = encodeSignalPattern("$", emptyBindings)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err = decodeSignalPattern("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Bindings == nil || len(decoded.Bindings) != 0 {
		t.Fatalf("expected empty non-nil bindings preserved, got %#v", decoded.Bindings)
	}
}

func TestSignalPattern_DecodeNull_IsStructuralError(t *testing.T) {
	_, err := decodeSignalPattern("$", json.RawMessage("null"))
	if err == nil {
		t.Fatal("expected an error decoding null as program.SignalPattern")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

// --- ask-group and task-group completion policies ---

func TestAskGroupCompletionPolicy_AllVariants_RoundTrip(t *testing.T) {
	variants := []program.AskGroupCompletionPolicy{
		program.AskGroupAllResponsesPolicy{},
		program.AskGroupFirstResponsePolicy{},
		program.AskGroupQuorumPolicy{Count: program.ReferenceExpression{Name: "requiredVotes"}},
	}
	for _, original := range variants {
		raw, err := encodeAskGroupCompletionPolicy("$", original)
		if err != nil {
			t.Fatalf("encode %T: %v", original, err)
		}
		decoded, err := decodeAskGroupCompletionPolicy("$", raw)
		if err != nil {
			t.Fatalf("decode %T: %v", original, err)
		}
		if !reflect.DeepEqual(original, decoded) {
			t.Fatalf("round trip mismatch for %T:\n  original = %#v\n  decoded  = %#v", original, original, decoded)
		}
	}
}

func TestTaskGroupCompletionPolicy_AllVariants_RoundTrip(t *testing.T) {
	variants := []program.TaskGroupCompletionPolicy{
		program.TaskGroupAllTerminalPolicy{},
		program.TaskGroupFirstTerminalPolicy{},
		program.TaskGroupQuorumTerminalPolicy{Count: program.ReferenceExpression{Name: "requiredTasks"}},
	}
	for _, original := range variants {
		raw, err := encodeTaskGroupCompletionPolicy("$", original)
		if err != nil {
			t.Fatalf("encode %T: %v", original, err)
		}
		decoded, err := decodeTaskGroupCompletionPolicy("$", raw)
		if err != nil {
			t.Fatalf("decode %T: %v", original, err)
		}
		if !reflect.DeepEqual(original, decoded) {
			t.Fatalf("round trip mismatch for %T:\n  original = %#v\n  decoded  = %#v", original, original, decoded)
		}
	}
}

// --- representative exact JSON ---

func TestExactJSON_SetOperation(t *testing.T) {
	raw, err := encodeOperation("$", program.SetOperation{
		Target: program.FieldTarget{Target: program.NameTarget{Name: "global"}, Field: "turnNumber"},
		Value:  program.NumberLiteralExpression{Value: "1"},
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["kind"] != "set" {
		t.Fatalf("unexpected shape: %#v", obj)
	}
}

func TestExactJSON_IfOperation(t *testing.T) {
	raw, err := encodeOperation("$", program.IfOperation{
		Condition: program.BoolLiteralExpression{Value: true},
		Then:      program.Block{Operations: []program.Operation{}},
		Else:      program.Block{},
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["kind"] != "if" {
		t.Fatalf("unexpected kind: %#v", obj["kind"])
	}
	then, ok := obj["then"].(map[string]any)
	if !ok {
		t.Fatalf("expected then to be an object, got %#v", obj["then"])
	}
	if ops, ok := then["operations"].([]any); !ok || len(ops) != 0 {
		t.Fatalf("expected then.operations to be an empty array, got %#v", then["operations"])
	}
	elseBlock, ok := obj["else"].(map[string]any)
	if !ok {
		t.Fatalf("expected else to be an object, got %#v", obj["else"])
	}
	if elseBlock["operations"] != nil {
		t.Fatalf("expected else.operations to be null (zero-value program.Block), got %#v", elseBlock["operations"])
	}
}

func TestExactJSON_OpenAskGroupOperation(t *testing.T) {
	raw, err := encodeOperation("$", program.OpenAskGroupOperation{
		Slot:       "cardVotes",
		Recipients: program.FieldExpression{Target: program.ReferenceExpression{Name: "team"}, Field: "users"},
		Arguments: []program.CallArgument{
			{Name: "options", Value: program.FieldExpression{Target: program.ReferenceExpression{Name: "local"}, Field: "availableCards"}},
		},
		Completion: program.AskGroupQuorumPolicy{Count: program.ReferenceExpression{Name: "requiredVotes"}},
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["kind"] != "open_ask_group" || obj["slot"] != "cardVotes" {
		t.Fatalf("unexpected shape: %#v", obj)
	}
	completion, ok := obj["completion"].(map[string]any)
	if !ok || completion["kind"] != "quorum" {
		t.Fatalf("unexpected completion: %#v", obj["completion"])
	}
}

func TestExactJSON_BeginTaskGroupOperation(t *testing.T) {
	raw, err := encodeOperation("$", program.BeginTaskGroupOperation{
		Slot:       "teamSelections",
		Completion: program.TaskGroupAllTerminalPolicy{},
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["kind"] != "begin_task_group" {
		t.Fatalf("unexpected kind: %#v", obj["kind"])
	}
	completion, ok := obj["completion"].(map[string]any)
	if !ok || completion["kind"] != "all_terminal" {
		t.Fatalf("unexpected completion: %#v", obj["completion"])
	}
}

func TestExactJSON_DrawRandomOperation(t *testing.T) {
	raw, err := encodeOperation("$", program.DrawRandomOperation{
		Name: "firstDie",
		Generator: program.RandomIntegerGenerator{
			Minimum: program.NumberLiteralExpression{Value: "1"},
			Maximum: program.NumberLiteralExpression{Value: "6"},
		},
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["kind"] != "draw_random" || obj["name"] != "firstDie" {
		t.Fatalf("unexpected shape: %#v", obj)
	}
}

func TestExactJSON_ConditionalControl(t *testing.T) {
	raw, err := encodeWorkflowControl("$", program.ConditionalControl{
		Condition: program.ReferenceExpression{Name: "hasWinner"},
		Then:      program.CompleteControl{Result: program.ReferenceExpression{Name: "winner"}},
		Else:      program.GotoControl{State: "NextRound"},
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["kind"] != "conditional" {
		t.Fatalf("unexpected kind: %#v", obj["kind"])
	}
}

func TestExactJSON_MatchControl(t *testing.T) {
	raw, err := encodeWorkflowControl("$", program.MatchControl{
		Value: program.ReferenceExpression{Name: "turnResult"},
		Cases: []program.MatchControlCase{
			{Pattern: program.WildcardMatchPattern{}, Control: program.StayControl{}},
		},
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["kind"] != "match" {
		t.Fatalf("unexpected kind: %#v", obj["kind"])
	}
	cases, ok := obj["cases"].([]any)
	if !ok || len(cases) != 1 {
		t.Fatalf("expected 1 case, got %#v", obj["cases"])
	}
}

func TestExactJSON_UserIntentSignalSource(t *testing.T) {
	raw, err := encodeSignalSource("$", program.UserIntentSignalSource{Intent: "PlayCard"})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["kind"] != "user_intent" || obj["intent"] != "PlayCard" {
		t.Fatalf("unexpected shape: %#v", obj)
	}
}

func TestExactJSON_AskGroupQuorumPolicy(t *testing.T) {
	raw, err := encodeAskGroupCompletionPolicy("$", program.AskGroupQuorumPolicy{Count: program.ReferenceExpression{Name: "requiredVotes"}})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["kind"] != "quorum" {
		t.Fatalf("unexpected kind: %#v", obj["kind"])
	}
}

func TestExactJSON_TaskGroupQuorumTerminalPolicy(t *testing.T) {
	raw, err := encodeTaskGroupCompletionPolicy("$", program.TaskGroupQuorumTerminalPolicy{Count: program.ReferenceExpression{Name: "requiredTasks"}})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["kind"] != "quorum_terminal" {
		t.Fatalf("unexpected kind: %#v", obj["kind"])
	}
}

func TestExactJSON_DeeplyNestedAssignmentTarget(t *testing.T) {
	raw, err := encodeAssignmentTarget("$", program.FieldTarget{
		Target: program.IndexTarget{
			Target: program.FieldTarget{Target: program.NameTarget{Name: "global"}, Field: "participants"},
			Index:  program.ReferenceExpression{Name: "currentUser"},
		},
		Field: "score",
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["kind"] != "field" || obj["field"] != "score" {
		t.Fatalf("unexpected shape: %#v", obj)
	}
	target, ok := obj["target"].(map[string]any)
	if !ok || target["kind"] != "index" {
		t.Fatalf("unexpected nested target: %#v", obj["target"])
	}
}

// --- pointer and value equivalence ---

func TestPointerAndValueEquivalence_AssignmentTarget(t *testing.T) {
	value := program.NameTarget{Name: "global"}
	valueJSON, err := encodeAssignmentTarget("$", value)
	if err != nil {
		t.Fatalf("encode value: %v", err)
	}
	pointerJSON, err := encodeAssignmentTarget("$", &value)
	if err != nil {
		t.Fatalf("encode pointer: %v", err)
	}
	assertSemanticJSONEqual(t, valueJSON, pointerJSON)
}

func TestPointerAndValueEquivalence_Operation(t *testing.T) {
	value := program.SetOperation{Target: program.NameTarget{Name: "x"}, Value: program.NumberLiteralExpression{Value: "1"}}
	valueJSON, err := encodeOperation("$", value)
	if err != nil {
		t.Fatalf("encode value: %v", err)
	}
	pointerJSON, err := encodeOperation("$", &value)
	if err != nil {
		t.Fatalf("encode pointer: %v", err)
	}
	assertSemanticJSONEqual(t, valueJSON, pointerJSON)
}

func TestPointerAndValueEquivalence_WorkflowControl(t *testing.T) {
	value := program.GotoControl{State: "NextState"}
	valueJSON, err := encodeWorkflowControl("$", value)
	if err != nil {
		t.Fatalf("encode value: %v", err)
	}
	pointerJSON, err := encodeWorkflowControl("$", &value)
	if err != nil {
		t.Fatalf("encode pointer: %v", err)
	}
	assertSemanticJSONEqual(t, valueJSON, pointerJSON)
}

func TestPointerAndValueEquivalence_SignalSource(t *testing.T) {
	value := program.NamedSignalSource{Name: "WorkflowStarted"}
	valueJSON, err := encodeSignalSource("$", value)
	if err != nil {
		t.Fatalf("encode value: %v", err)
	}
	pointerJSON, err := encodeSignalSource("$", &value)
	if err != nil {
		t.Fatalf("encode pointer: %v", err)
	}
	assertSemanticJSONEqual(t, valueJSON, pointerJSON)
}

func TestPointerAndValueEquivalence_AskGroupPolicy(t *testing.T) {
	value := program.AskGroupAllResponsesPolicy{}
	valueJSON, err := encodeAskGroupCompletionPolicy("$", value)
	if err != nil {
		t.Fatalf("encode value: %v", err)
	}
	pointerJSON, err := encodeAskGroupCompletionPolicy("$", &value)
	if err != nil {
		t.Fatalf("encode pointer: %v", err)
	}
	assertSemanticJSONEqual(t, valueJSON, pointerJSON)
}

func TestPointerAndValueEquivalence_TaskGroupPolicy(t *testing.T) {
	value := program.TaskGroupAllTerminalPolicy{}
	valueJSON, err := encodeTaskGroupCompletionPolicy("$", value)
	if err != nil {
		t.Fatalf("encode value: %v", err)
	}
	pointerJSON, err := encodeTaskGroupCompletionPolicy("$", &value)
	if err != nil {
		t.Fatalf("encode pointer: %v", err)
	}
	assertSemanticJSONEqual(t, valueJSON, pointerJSON)
}

// --- typed nil pointers ---

func TestTypedNilPointers_EncodeToNull(t *testing.T) {
	var target *program.NameTarget
	if raw, err := encodeAssignmentTarget("$", target); err != nil || string(raw) != "null" {
		t.Fatalf("AssignmentTarget: expected null, got %s, err %v", raw, err)
	}

	var operation *program.SetOperation
	if raw, err := encodeOperation("$", operation); err != nil || string(raw) != "null" {
		t.Fatalf("Operation: expected null, got %s, err %v", raw, err)
	}

	var control *program.GotoControl
	if raw, err := encodeWorkflowControl("$", control); err != nil || string(raw) != "null" {
		t.Fatalf("WorkflowControl: expected null, got %s, err %v", raw, err)
	}

	var source *program.NamedSignalSource
	if raw, err := encodeSignalSource("$", source); err != nil || string(raw) != "null" {
		t.Fatalf("SignalSource: expected null, got %s, err %v", raw, err)
	}

	var askPolicy *program.AskGroupAllResponsesPolicy
	if raw, err := encodeAskGroupCompletionPolicy("$", askPolicy); err != nil || string(raw) != "null" {
		t.Fatalf("AskGroupCompletionPolicy: expected null, got %s, err %v", raw, err)
	}

	var taskPolicy *program.TaskGroupAllTerminalPolicy
	if raw, err := encodeTaskGroupCompletionPolicy("$", taskPolicy); err != nil || string(raw) != "null" {
		t.Fatalf("TaskGroupCompletionPolicy: expected null, got %s, err %v", raw, err)
	}
}

// --- null decoding ---

func TestJSONNullDecoding_ExecutionFamilies(t *testing.T) {
	if v, err := decodeAssignmentTarget("$", json.RawMessage("null")); err != nil || v != nil {
		t.Fatalf("AssignmentTarget: expected nil, nil got %#v, %v", v, err)
	}
	if v, err := decodeOperation("$", json.RawMessage("null")); err != nil || v != nil {
		t.Fatalf("Operation: expected nil, nil got %#v, %v", v, err)
	}
	if v, err := decodeWorkflowControl("$", json.RawMessage("null")); err != nil || v != nil {
		t.Fatalf("WorkflowControl: expected nil, nil got %#v, %v", v, err)
	}
	if v, err := decodeSignalSource("$", json.RawMessage("null")); err != nil || v != nil {
		t.Fatalf("SignalSource: expected nil, nil got %#v, %v", v, err)
	}
	if v, err := decodeAskGroupCompletionPolicy("$", json.RawMessage("null")); err != nil || v != nil {
		t.Fatalf("AskGroupCompletionPolicy: expected nil, nil got %#v, %v", v, err)
	}
	if v, err := decodeTaskGroupCompletionPolicy("$", json.RawMessage("null")); err != nil || v != nil {
		t.Fatalf("TaskGroupCompletionPolicy: expected nil, nil got %#v, %v", v, err)
	}
}

// --- unknown / missing discriminator ---

func TestDecode_UnknownDiscriminator_ExecutionFamilies(t *testing.T) {
	cases := []struct {
		name   string
		decode func(string, json.RawMessage) error
	}{
		{"program.AssignmentTarget", func(p string, d json.RawMessage) error { _, err := decodeAssignmentTarget(p, d); return err }},
		{"program.Operation", func(p string, d json.RawMessage) error { _, err := decodeOperation(p, d); return err }},
		{"program.WorkflowControl", func(p string, d json.RawMessage) error { _, err := decodeWorkflowControl(p, d); return err }},
		{"program.SignalSource", func(p string, d json.RawMessage) error { _, err := decodeSignalSource(p, d); return err }},
		{"program.AskGroupCompletionPolicy", func(p string, d json.RawMessage) error { _, err := decodeAskGroupCompletionPolicy(p, d); return err }},
		{"program.TaskGroupCompletionPolicy", func(p string, d json.RawMessage) error { _, err := decodeTaskGroupCompletionPolicy(p, d); return err }},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.decode("$", json.RawMessage(`{"kind":"not_a_real_kind"}`))
			if err == nil {
				t.Fatal("expected an error")
			}
			var decodeErr *DecodeError
			if !errors.As(err, &decodeErr) {
				t.Fatalf("expected *DecodeError, got %T: %v", err, err)
			}
			if decodeErr.Path != "$" {
				t.Fatalf("expected path $, got %q", decodeErr.Path)
			}
			if decodeErr.Message == "" {
				t.Fatal("expected a non-empty message identifying the unsupported kind")
			}
		})
	}
}

func TestDecode_MissingDiscriminator_Operation(t *testing.T) {
	_, err := decodeOperation("$", json.RawMessage(`{"target":null,"value":null}`))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$" {
		t.Fatalf("expected path $, got %q", decodeErr.Path)
	}
}

// --- unknown field ---

func TestDecode_UnknownField_WorkflowControl(t *testing.T) {
	_, err := decodeWorkflowControl("$", json.RawMessage(`{"kind":"stay","unexpected":true}`))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

func TestDecode_UnknownField_Operation(t *testing.T) {
	_, err := decodeOperation("$", json.RawMessage(`{"kind":"close_question","slot":"moveRequest","unexpected":true}`))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

func TestDecode_UnknownField_Block(t *testing.T) {
	_, err := decodeBlock("$", json.RawMessage(`{"operations":[],"unexpected":true}`))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

func TestDecode_UnknownField_SignalPattern(t *testing.T) {
	_, err := decodeSignalPattern("$", json.RawMessage(`{"source":{"kind":"named","name":"WorkflowStarted"},"bindings":[],"unexpected":true}`))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

// --- nested path failures ---

func TestDecode_NestedPathFailure_Target(t *testing.T) {
	data := json.RawMessage(`{
		"kind": "list_remove_at",
		"target": {"kind": "reference"},
		"index": {"kind": "number_literal", "value": "2"}
	}`)
	_, err := decodeOperation("$", data)
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$.target" {
		t.Fatalf("expected path $.target, got %q", decodeErr.Path)
	}
}

func TestDecode_NestedPathFailure_ThenOperationsArray(t *testing.T) {
	data := json.RawMessage(`{
		"kind": "if",
		"condition": {"kind": "bool_literal", "value": true},
		"then": {
			"operations": [
				{"kind": "close_question", "slot": "a"},
				{"kind": "not_a_real_kind"}
			]
		},
		"else": {"operations": []}
	}`)
	_, err := decodeOperation("$", data)
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$.then.operations[1]" {
		t.Fatalf("expected path $.then.operations[1], got %q", decodeErr.Path)
	}
}

func TestDecode_NestedPathFailure_MatchControlCasePattern(t *testing.T) {
	data := json.RawMessage(`{
		"kind": "match",
		"value": {"kind": "reference", "name": "turnResult"},
		"cases": [
			{"pattern": {"kind": "not_a_real_pattern"}, "control": {"kind": "stay"}}
		]
	}`)
	_, err := decodeWorkflowControl("$", data)
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$.cases[0].pattern" {
		t.Fatalf("expected path $.cases[0].pattern, got %q", decodeErr.Path)
	}
}

func TestDecode_NestedPathFailure_MatchControlCaseControl(t *testing.T) {
	data := json.RawMessage(`{
		"kind": "match",
		"value": {"kind": "reference", "name": "turnResult"},
		"cases": [
			{"pattern": {"kind": "wildcard"}, "control": {"kind": "not_a_real_control"}}
		]
	}`)
	_, err := decodeWorkflowControl("$", data)
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$.cases[0].control" {
		t.Fatalf("expected path $.cases[0].control, got %q", decodeErr.Path)
	}
}

func TestDecode_NestedPathFailure_CompletionCount(t *testing.T) {
	data := json.RawMessage(`{
		"kind": "open_ask_group",
		"slot": "cardVotes",
		"recipients": {"kind": "reference", "name": "team"},
		"arguments": [],
		"completion": {"kind": "quorum", "count": {"kind": "not_a_real_expression"}}
	}`)
	_, err := decodeOperation("$", data)
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$.completion.count" {
		t.Fatalf("expected path $.completion.count, got %q", decodeErr.Path)
	}
}

func TestDecode_NestedPathFailure_ArgumentsArrayValue(t *testing.T) {
	data := json.RawMessage(`{
		"kind": "emit_effect",
		"effect": "DiceRolled",
		"recipients": {"kind": "reference", "name": "roomUsers"},
		"arguments": [
			{"name": "first", "value": {"kind": "reference", "name": "firstDie"}},
			{"name": "second", "value": {"kind": "not_a_real_expression"}}
		]
	}`)
	_, err := decodeOperation("$", data)
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$.arguments[1].value" {
		t.Fatalf("expected path $.arguments[1].value, got %q", decodeErr.Path)
	}
}

func TestDecode_NestedPathFailure_GeneratorMaximum(t *testing.T) {
	data := json.RawMessage(`{
		"kind": "draw_random",
		"name": "firstDie",
		"generator": {
			"kind": "random_integer",
			"minimum": {"kind": "number_literal", "value": "1"},
			"maximum": {"kind": "not_a_real_expression"}
		}
	}`)
	_, err := decodeOperation("$", data)
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$.generator.maximum" {
		t.Fatalf("expected path $.generator.maximum, got %q", decodeErr.Path)
	}
}

func TestDecode_NestedPathFailure_SignalPatternSource(t *testing.T) {
	data := json.RawMessage(`{"source": {"kind": "not_a_real_source"}, "bindings": []}`)
	_, err := decodeSignalPattern("$", data)
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$.source" {
		t.Fatalf("expected path $.source, got %q", decodeErr.Path)
	}
}

// --- semantic invalidity remains representable ---

func TestSemanticInvalidity_EmptyGotoState(t *testing.T) {
	decoded, err := decodeWorkflowControl("$", json.RawMessage(`{"kind":"goto","state":""}`))
	if err != nil {
		t.Fatalf("expected structural success, got %v", err)
	}
	if decoded.(program.GotoControl).State != "" {
		t.Fatalf("expected empty state preserved, got %q", decoded.(program.GotoControl).State)
	}
}

func TestSemanticInvalidity_UnknownSlot(t *testing.T) {
	decoded, err := decodeOperation("$", json.RawMessage(`{"kind":"close_question","slot":"unknownSlot"}`))
	if err != nil {
		t.Fatalf("expected structural success, got %v", err)
	}
	if decoded.(program.CloseQuestionOperation).Slot != "unknownSlot" {
		t.Fatalf("expected unknown slot preserved, got %q", decoded.(program.CloseQuestionOperation).Slot)
	}
}

func TestSemanticInvalidity_DuplicateCallArguments(t *testing.T) {
	decoded, err := decodeOperation("$", json.RawMessage(`{
		"kind": "open_question",
		"slot": "moveRequest",
		"recipient": {"kind": "reference", "name": "participant"},
		"arguments": [
			{"name": "plans", "value": {"kind": "number_literal", "value": "1"}},
			{"name": "plans", "value": {"kind": "number_literal", "value": "2"}}
		]
	}`))
	if err != nil {
		t.Fatalf("expected structural success, got %v", err)
	}
	op := decoded.(program.OpenQuestionOperation)
	if len(op.Arguments) != 2 {
		t.Fatalf("expected both duplicate arguments preserved, got %d", len(op.Arguments))
	}
}

func TestSemanticInvalidity_NilTimerDelay(t *testing.T) {
	decoded, err := decodeOperation("$", json.RawMessage(`{
		"kind": "schedule_timer",
		"slot": "moveDeadline",
		"delay_milliseconds": null
	}`))
	if err != nil {
		t.Fatalf("expected structural success, got %v", err)
	}
	if decoded.(program.ScheduleTimerOperation).DelayMilliseconds != nil {
		t.Fatalf("expected nil delay preserved, got %#v", decoded.(program.ScheduleTimerOperation).DelayMilliseconds)
	}
}

func TestSemanticInvalidity_NegativeQuorumCount(t *testing.T) {
	decoded, err := decodeAskGroupCompletionPolicy("$", json.RawMessage(`{
		"kind": "quorum",
		"count": {"kind": "number_literal", "value": "-1"}
	}`))
	if err != nil {
		t.Fatalf("expected structural success, got %v", err)
	}
	count := decoded.(program.AskGroupQuorumPolicy).Count.(program.NumberLiteralExpression)
	if count.Value != "-1" {
		t.Fatalf("expected negative literal preserved, got %q", count.Value)
	}
}

func TestSemanticInvalidity_NilTaskGroupSpawnKey(t *testing.T) {
	decoded, err := decodeOperation("$", json.RawMessage(`{
		"kind": "spawn_task_group_child",
		"slot": "teamSelections",
		"key": null,
		"arguments": []
	}`))
	if err != nil {
		t.Fatalf("expected structural success, got %v", err)
	}
	if decoded.(program.SpawnTaskGroupChildOperation).Key != nil {
		t.Fatalf("expected nil key preserved, got %#v", decoded.(program.SpawnTaskGroupChildOperation).Key)
	}
}

func TestSemanticInvalidity_UnavailableSignalBindingField(t *testing.T) {
	decoded, err := decodeSignalPattern("$", json.RawMessage(`{
		"source": {"kind": "timer_expired", "slot": "moveDeadline"},
		"bindings": [
			{"field": "scheduledAt", "name": "when"}
		]
	}`))
	if err != nil {
		t.Fatalf("expected structural success, got %v", err)
	}
	if len(decoded.Bindings) != 1 || decoded.Bindings[0].Field != "scheduledAt" {
		t.Fatalf("expected unavailable field binding preserved, got %#v", decoded.Bindings)
	}
}

func TestSemanticInvalidity_FailControlWithNumericExpression(t *testing.T) {
	decoded, err := decodeWorkflowControl("$", json.RawMessage(`{
		"kind": "fail",
		"error": {"kind": "number_literal", "value": "404"}
	}`))
	if err != nil {
		t.Fatalf("expected structural success, got %v", err)
	}
	errExpr := decoded.(program.FailControl).Error.(program.NumberLiteralExpression)
	if errExpr.Value != "404" {
		t.Fatalf("expected numeric error expression preserved, got %#v", errExpr)
	}
}

func TestSemanticInvalidity_InvalidTaskGroupLifecycleOrder(t *testing.T) {
	// A block containing "seal" before "begin" is semantically invalid but
	// must still round-trip structurally — lifecycle ordering is an engine
	// concern, not a codec concern.
	original := program.Block{
		Operations: []program.Operation{
			program.SealTaskGroupOperation{Slot: "teamSelections"},
			program.BeginTaskGroupOperation{Slot: "teamSelections", Completion: program.TaskGroupAllTerminalPolicy{}},
		},
	}
	raw, err := encodeBlock("$", original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeBlock("$", raw)
	if err != nil {
		t.Fatalf("expected structural success, got %v", err)
	}
	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", original, decoded)
	}
}

func TestSemanticInvalidity_MatchOperationNoCases(t *testing.T) {
	decoded, err := decodeOperation("$", json.RawMessage(`{
		"kind": "match",
		"value": {"kind": "reference", "name": "x"},
		"cases": []
	}`))
	if err != nil {
		t.Fatalf("expected structural success, got %v", err)
	}
	if len(decoded.(program.MatchOperation).Cases) != 0 {
		t.Fatalf("expected 0 cases preserved, got %d", len(decoded.(program.MatchOperation).Cases))
	}
}

func TestSemanticInvalidity_EmptyAssignmentTargetNames(t *testing.T) {
	decoded, err := decodeAssignmentTarget("$", json.RawMessage(`{"kind":"field","target":{"kind":"name","name":""},"field":""}`))
	if err != nil {
		t.Fatalf("expected structural success, got %v", err)
	}
	target := decoded.(program.FieldTarget)
	if target.Field != "" || target.Target.(program.NameTarget).Name != "" {
		t.Fatalf("expected empty names preserved, got %#v", target)
	}
}

// --- nil vs empty slices ---

func TestNilVsEmptySlices_MatchOperationCases(t *testing.T) {
	nilCase := program.MatchOperation{Value: program.ReferenceExpression{Name: "x"}, Cases: nil}
	raw, err := encodeOperation("$", nilCase)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeOperation("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.(program.MatchOperation).Cases != nil {
		t.Fatalf("expected nil cases preserved, got %#v", decoded.(program.MatchOperation).Cases)
	}

	emptyCase := program.MatchOperation{Value: program.ReferenceExpression{Name: "x"}, Cases: []program.MatchOperationCase{}}
	raw, err = encodeOperation("$", emptyCase)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err = decodeOperation("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	cases := decoded.(program.MatchOperation).Cases
	if cases == nil || len(cases) != 0 {
		t.Fatalf("expected empty non-nil cases preserved, got %#v", cases)
	}
}

func TestNilVsEmptySlices_SignalBindings(t *testing.T) {
	nilBindings := program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}, Bindings: nil}
	raw, err := encodeSignalPattern("$", nilBindings)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["bindings"] != nil {
		t.Fatalf("expected null bindings, got %#v", obj["bindings"])
	}
}

// --- duplicate ordering ---

func TestDuplicateOrdering_SignalBindings(t *testing.T) {
	original := program.SignalPattern{
		Source: program.UserIntentSignalSource{Intent: "PlayCard"},
		Bindings: []program.SignalBinding{
			{Field: "card", Name: "a"},
			{Field: "card", Name: "b"},
		},
	}
	raw, err := encodeSignalPattern("$", original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeSignalPattern("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(decoded.Bindings) != 2 || decoded.Bindings[0].Name != "a" || decoded.Bindings[1].Name != "b" {
		t.Fatalf("binding order not preserved: %#v", decoded.Bindings)
	}
}

func TestDuplicateOrdering_TaskSpawnArguments(t *testing.T) {
	original := program.SpawnTaskGroupChildOperation{
		Slot: "teamSelections",
		Key:  program.ReferenceExpression{Name: "team"},
		Arguments: []program.CallArgument{
			{Name: "value", Value: program.NumberLiteralExpression{Value: "1"}},
			{Name: "value", Value: program.NumberLiteralExpression{Value: "2"}},
		},
	}
	raw, err := encodeOperation("$", original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeOperation("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	op := decoded.(program.SpawnTaskGroupChildOperation)
	if len(op.Arguments) != 2 || op.Arguments[0].Value.(program.NumberLiteralExpression).Value != "1" || op.Arguments[1].Value.(program.NumberLiteralExpression).Value != "2" {
		t.Fatalf("argument order not preserved: %#v", op.Arguments)
	}
}

func TestDuplicateOrdering_MatchCases(t *testing.T) {
	original := program.MatchOperation{
		Value: program.ReferenceExpression{Name: "x"},
		Cases: []program.MatchOperationCase{
			{Pattern: program.EnumValueMatchPattern{TypeName: "T", ValueName: "A"}, Body: program.Block{}},
			{Pattern: program.EnumValueMatchPattern{TypeName: "T", ValueName: "A"}, Body: program.Block{}},
		},
	}
	raw, err := encodeOperation("$", original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeOperation("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(decoded.(program.MatchOperation).Cases) != 2 {
		t.Fatalf("expected 2 duplicate cases preserved, got %d", len(decoded.(program.MatchOperation).Cases))
	}
}

// --- deep integrated structure: signal + block + control ---

func TestIntegratedStructure_TransitionFragment(t *testing.T) {
	signal := program.SignalPattern{Source: program.UserIntentSignalSource{Intent: "RollDice"}}

	block := program.Block{
		Operations: []program.Operation{
			program.DrawRandomOperation{
				Name:      "firstDie",
				Generator: program.RandomIntegerGenerator{Minimum: program.NumberLiteralExpression{Value: "1"}, Maximum: program.NumberLiteralExpression{Value: "6"}},
			},
			program.DrawRandomOperation{
				Name:      "secondDie",
				Generator: program.RandomIntegerGenerator{Minimum: program.NumberLiteralExpression{Value: "1"}, Maximum: program.NumberLiteralExpression{Value: "6"}},
			},
			program.SetOperation{
				Target: program.FieldTarget{Target: program.NameTarget{Name: "global"}, Field: "lastRoll"},
				Value:  program.ReferenceExpression{Name: "firstDie"},
			},
			program.EmitEffectOperation{
				Effect:     "DiceRolled",
				Recipients: program.ReferenceExpression{Name: "roomUsers"},
				Arguments: []program.CallArgument{
					{Name: "first", Value: program.ReferenceExpression{Name: "firstDie"}},
					{Name: "second", Value: program.ReferenceExpression{Name: "secondDie"}},
				},
			},
		},
	}

	control := program.GotoControl{State: "WaitingForMove"}

	signalRaw, err := encodeSignalPattern("$", signal)
	if err != nil {
		t.Fatalf("encode signal: %v", err)
	}
	blockRaw, err := encodeBlock("$", block)
	if err != nil {
		t.Fatalf("encode block: %v", err)
	}
	controlRaw, err := encodeWorkflowControl("$", control)
	if err != nil {
		t.Fatalf("encode control: %v", err)
	}

	decodedSignal, err := decodeSignalPattern("$", signalRaw)
	if err != nil {
		t.Fatalf("decode signal: %v", err)
	}
	decodedBlock, err := decodeBlock("$", blockRaw)
	if err != nil {
		t.Fatalf("decode block: %v", err)
	}
	decodedControl, err := decodeWorkflowControl("$", controlRaw)
	if err != nil {
		t.Fatalf("decode control: %v", err)
	}

	if !reflect.DeepEqual(signal, decodedSignal) {
		t.Fatalf("signal mismatch:\n  original = %#v\n  decoded  = %#v", signal, decodedSignal)
	}
	if !reflect.DeepEqual(block, decodedBlock) {
		t.Fatalf("block mismatch:\n  original = %#v\n  decoded  = %#v", block, decodedBlock)
	}
	if !reflect.DeepEqual(control, decodedControl) {
		t.Fatalf("control mismatch:\n  original = %#v\n  decoded  = %#v", control, decodedControl)
	}

	// Cross-reference: the effect's "first"/"second" arguments reference
	// the same lexical names introduced by the two draw operations.
	firstDraw := decodedBlock.Operations[0].(program.DrawRandomOperation)
	secondDraw := decodedBlock.Operations[1].(program.DrawRandomOperation)
	effect := decodedBlock.Operations[3].(program.EmitEffectOperation)
	if firstDraw.Name != effect.Arguments[0].Value.(program.ReferenceExpression).Name {
		t.Fatalf("expected effect first argument to reference %q, got %q", firstDraw.Name, effect.Arguments[0].Value.(program.ReferenceExpression).Name)
	}
	if secondDraw.Name != effect.Arguments[1].Value.(program.ReferenceExpression).Name {
		t.Fatalf("expected effect second argument to reference %q, got %q", secondDraw.Name, effect.Arguments[1].Value.(program.ReferenceExpression).Name)
	}

	// Cross-reference: the goto control's target state name is preserved
	// verbatim as authored.
	if decodedControl.(program.GotoControl).State != "WaitingForMove" {
		t.Fatalf("expected goto state %q, got %q", "WaitingForMove", decodedControl.(program.GotoControl).State)
	}
}
