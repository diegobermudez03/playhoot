package codec

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/diegobermudez03/playhoot/game/language/v1/program"
)

// --- transition ---

func TestTransitionDeclaration_RoundTrip_Full(t *testing.T) {
	original := program.TransitionDeclaration{
		Name: "handle_move",
		Signal: program.SignalPattern{
			Source: program.QuestionAnsweredSignalSource{Slot: "moveRequest"},
			Bindings: []program.SignalBinding{
				{Field: "respondent", Name: "user"},
				{Field: "answer", Name: "selectedMove"},
			},
		},
		Guard: program.BinaryExpression{
			Operator: program.BinaryOperatorEqual,
			Left:     program.ReferenceExpression{Name: "user"},
			Right:    program.FieldExpression{Target: program.ReferenceExpression{Name: "local"}, Field: "participant"},
		},
		Operations: program.Block{
			Operations: []program.Operation{
				program.SetOperation{
					Target: program.FieldTarget{Target: program.NameTarget{Name: "local"}, Field: "selectedMove"},
					Value:  program.ReferenceExpression{Name: "selectedMove"},
				},
			},
		},
		Control: program.GotoControl{State: "ResolveMove"},
	}
	raw, err := encodeTransitionDeclaration("$", original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeTransitionDeclaration("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", original, decoded)
	}
}

func TestTransitionDeclaration_RoundTrip_NilAndEmptyVariants(t *testing.T) {
	cases := []program.TransitionDeclaration{
		{Name: "", Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}}, Guard: nil, Control: nil},
		{
			Name:       "empty_ops",
			Signal:     program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
			Operations: program.Block{Operations: []program.Operation{}},
			Control:    program.StayControl{},
		},
		{
			Name: "duplicate_bindings",
			Signal: program.SignalPattern{
				Source: program.UserIntentSignalSource{Intent: "PlayCard"},
				Bindings: []program.SignalBinding{
					{Field: "card", Name: "a"},
					{Field: "card", Name: "a"},
				},
			},
			Control: program.StayControl{},
		},
	}
	for i, original := range cases {
		raw, err := encodeTransitionDeclaration("$", original)
		if err != nil {
			t.Fatalf("case %d: encode: %v", i, err)
		}
		decoded, err := decodeTransitionDeclaration("$", raw)
		if err != nil {
			t.Fatalf("case %d: decode: %v", i, err)
		}
		if !reflect.DeepEqual(original, decoded) {
			t.Fatalf("case %d: round trip mismatch:\n  original = %#v\n  decoded  = %#v", i, original, decoded)
		}
	}
}

func TestExactJSON_TransitionDeclaration(t *testing.T) {
	raw, err := encodeTransitionDeclaration("$", program.TransitionDeclaration{
		Name:    "handle_move",
		Signal:  program.SignalPattern{Source: program.QuestionAnsweredSignalSource{Slot: "moveRequest"}},
		Control: program.GotoControl{State: "ResolveMove"},
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["name"] != "handle_move" {
		t.Fatalf("unexpected shape: %#v", obj)
	}
	signal, ok := obj["signal"].(map[string]any)
	if !ok {
		t.Fatalf("expected signal object, got %#v", obj["signal"])
	}
	source, ok := signal["source"].(map[string]any)
	if !ok || source["kind"] != "question_answered" {
		t.Fatalf("unexpected signal source: %#v", signal["source"])
	}
}

func TestDecode_TransitionDeclaration_Null(t *testing.T) {
	_, err := decodeTransitionDeclaration("$", json.RawMessage("null"))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

func TestDecode_UnknownField_TransitionDeclaration(t *testing.T) {
	_, err := decodeTransitionDeclaration("$", json.RawMessage(`{
		"name":"x","signal":{"source":null,"bindings":null},"guard":null,
		"operations":{"operations":null},"control":null,"extra":true
	}`))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

func TestDecode_NestedPathFailure_TransitionSignalSource(t *testing.T) {
	data := json.RawMessage(`{
		"name": "x",
		"signal": {"source": {"kind": "not_a_real_source"}, "bindings": []},
		"guard": null,
		"operations": {"operations": []},
		"control": null
	}`)
	_, err := decodeTransitionDeclaration("$", data)
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$.signal.source" {
		t.Fatalf("expected path $.signal.source, got %q", decodeErr.Path)
	}
}

func TestDecode_NestedPathFailure_TransitionOperationsArray(t *testing.T) {
	data := json.RawMessage(`{
		"name": "x",
		"signal": {"source": {"kind": "named", "name": "WorkflowStarted"}, "bindings": []},
		"guard": null,
		"operations": {"operations": [
			{"kind": "close_question", "slot": "a"},
			{"kind": "not_a_real_kind"}
		]},
		"control": null
	}`)
	_, err := decodeTransitionDeclaration("$", data)
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$.operations.operations[1]" {
		t.Fatalf("expected path $.operations.operations[1], got %q", decodeErr.Path)
	}
}

func TestDecode_NestedPathFailure_TransitionControl(t *testing.T) {
	data := json.RawMessage(`{
		"name": "x",
		"signal": {"source": {"kind": "named", "name": "WorkflowStarted"}, "bindings": []},
		"guard": null,
		"operations": {"operations": []},
		"control": {"kind": "not_a_real_control"}
	}`)
	_, err := decodeTransitionDeclaration("$", data)
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$.control" {
		t.Fatalf("expected path $.control, got %q", decodeErr.Path)
	}
}

// --- workflow state ---

func TestWorkflowStateDeclaration_RoundTrip(t *testing.T) {
	original := program.WorkflowStateDeclaration{
		Name: "WaitingForMove",
		Presentations: []program.PresentationDeclaration{
			{Name: "MoveStatus", Slot: "hud", Targets: program.ReferenceExpression{Name: "roomUsers"}, Projection: "MoveStatusProjection", View: "MoveStatusView"},
			{Name: "MoveStatus", Slot: "hud", Targets: program.ReferenceExpression{Name: "roomUsers"}, Projection: "MoveStatusProjection", View: "MoveStatusView"},
		},
		Transitions: []program.TransitionDeclaration{
			{
				Name:    "handle_move",
				Signal:  program.SignalPattern{Source: program.QuestionAnsweredSignalSource{Slot: "moveRequest"}},
				Control: program.GotoControl{State: "ResolveMove"},
			},
			{
				Name:    "duplicate_same_signal",
				Signal:  program.SignalPattern{Source: program.QuestionAnsweredSignalSource{Slot: "moveRequest"}},
				Control: program.StayControl{},
			},
		},
	}
	raw, err := encodeWorkflowStateDeclaration("$", original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeWorkflowStateDeclaration("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", original, decoded)
	}
}

func TestWorkflowStateDeclaration_NilVsEmptySlices(t *testing.T) {
	nilCase := program.WorkflowStateDeclaration{Name: "S", Presentations: nil, Transitions: nil}
	raw, err := encodeWorkflowStateDeclaration("$", nilCase)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeWorkflowStateDeclaration("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Presentations != nil || decoded.Transitions != nil {
		t.Fatalf("expected nil slices preserved, got %#v", decoded)
	}

	emptyCase := program.WorkflowStateDeclaration{Name: "S", Presentations: []program.PresentationDeclaration{}, Transitions: []program.TransitionDeclaration{}}
	raw, err = encodeWorkflowStateDeclaration("$", emptyCase)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err = decodeWorkflowStateDeclaration("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Presentations == nil || decoded.Transitions == nil {
		t.Fatalf("expected empty non-nil slices preserved, got %#v", decoded)
	}
}

func TestDecode_WorkflowStateDeclaration_Null(t *testing.T) {
	_, err := decodeWorkflowStateDeclaration("$", json.RawMessage("null"))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

func TestDecode_UnknownField_WorkflowStateDeclaration(t *testing.T) {
	_, err := decodeWorkflowStateDeclaration("$", json.RawMessage(`{"name":"x","presentations":[],"transitions":[],"extra":true}`))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

func TestDecode_NestedPathFailure_StatePresentationTargets(t *testing.T) {
	data := json.RawMessage(`{
		"name": "x",
		"presentations": [
			{"name": "p", "slot": "s", "targets": {"kind": "not_a_real_expression"}, "projection": "p", "projection_arguments": [], "view": "v"}
		],
		"transitions": []
	}`)
	_, err := decodeWorkflowStateDeclaration("$", data)
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$.presentations[0].targets" {
		t.Fatalf("expected path $.presentations[0].targets, got %q", decodeErr.Path)
	}
}

func TestDecode_NestedPathFailure_StateTransitionGuard(t *testing.T) {
	data := json.RawMessage(`{
		"name": "x",
		"presentations": [],
		"transitions": [
			{
				"name": "t",
				"signal": {"source": {"kind": "named", "name": "WorkflowStarted"}, "bindings": []},
				"guard": {"kind": "not_a_real_expression"},
				"operations": {"operations": []},
				"control": null
			}
		]
	}`)
	_, err := decodeWorkflowStateDeclaration("$", data)
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$.transitions[0].guard" {
		t.Fatalf("expected path $.transitions[0].guard, got %q", decodeErr.Path)
	}
}

// --- workflow declaration ---

func TestWorkflowDeclaration_RoundTrip_AllCategories(t *testing.T) {
	original := program.WorkflowDeclaration{
		Name:       "PlayTurn",
		Parameters: []program.FieldDeclaration{{Name: "participant", Type: program.NamedTypeReference{Name: "ParticipantId"}}},
		ResultType: program.NamedTypeReference{Name: "TurnResult"},
		LocalState: program.StateDeclaration{
			Fields: []program.StateFieldDeclaration{
				{Name: "consecutivePairs", Type: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}, Initializer: program.NumberLiteralExpression{Value: "0"}},
			},
		},
		QuestionSlots: []program.QuestionSlotDeclaration{
			{
				Name:     "moveRequest",
				Question: "ChooseMove",
				Presentation: &program.QuestionPresentationDeclaration{
					Slot:                "primaryInteraction",
					Projection:          "ChooseMoveProjection",
					ProjectionArguments: []program.CallArgument{{Name: "plans", Value: program.ReferenceExpression{Name: "plans"}}},
					View:                "ChooseMoveView",
				},
			},
		},
		AskGroupSlots: []program.AskGroupSlotDeclaration{
			{Name: "votes", Question: "ChooseCard"},
		},
		TimerSlots: []program.TimerSlotDeclaration{{Name: "moveDeadline"}},
		ChildSlots: []program.ChildWorkflowSlotDeclaration{{Name: "activeTurn", Workflow: "PlayTurn"}},
		TaskGroupSlots: []program.TaskGroupSlotDeclaration{
			{Name: "teamSelections", Workflow: "TeamChooseCard", KeyType: program.NamedTypeReference{Name: "TeamId"}},
		},
		Presentations: []program.PresentationDeclaration{
			{Name: "MatchBoard", Slot: "main", Targets: program.ReferenceExpression{Name: "roomUsers"}, Projection: "BoardProjection", View: "BoardView"},
		},
		InitialState: "Start",
		GlobalTransitions: []program.TransitionDeclaration{
			{Name: "cancel_on_session", Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "SessionCancelled"}}, Control: program.CancelControl{Reason: program.StringLiteralExpression{Value: "session cancelled"}}},
		},
		States: []program.WorkflowStateDeclaration{
			{Name: "Start", Transitions: []program.TransitionDeclaration{{Name: "start", Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}}, Control: program.GotoControl{State: "Waiting"}}}},
			{Name: "Waiting", Transitions: []program.TransitionDeclaration{}},
		},
	}
	raw, err := encodeWorkflowDeclaration("$", original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeWorkflowDeclaration("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", original, decoded)
	}
}

func TestWorkflowDeclaration_ComplexStructuredConcurrency(t *testing.T) {
	original := program.WorkflowDeclaration{
		Name:       "MatchRound",
		ResultType: program.NamedTypeReference{Name: "RoundResult"},
		QuestionSlots: []program.QuestionSlotDeclaration{
			{Name: "confirmation", Question: "ConfirmAction"},
		},
		AskGroupSlots: []program.AskGroupSlotDeclaration{
			{Name: "votes", Question: "ChooseCard"},
		},
		TimerSlots: []program.TimerSlotDeclaration{{Name: "deadline"}},
		ChildSlots: []program.ChildWorkflowSlotDeclaration{{Name: "activeTurn", Workflow: "PlayTurn"}},
		TaskGroupSlots: []program.TaskGroupSlotDeclaration{
			{Name: "teamSelections", Workflow: "TeamChooseCard", KeyType: program.NamedTypeReference{Name: "TeamId"}},
		},
		Presentations: []program.PresentationDeclaration{
			{Name: "MatchBoard", Slot: "main", Targets: program.ReferenceExpression{Name: "roomUsers"}, Projection: "BoardProjection", View: "BoardView"},
		},
		InitialState: "Start",
		GlobalTransitions: []program.TransitionDeclaration{
			{
				Name:    "session_cancelled",
				Signal:  program.SignalPattern{Source: program.NamedSignalSource{Name: "SessionCancelled"}},
				Control: program.CancelControl{Reason: program.StringLiteralExpression{Value: "session ended"}},
			},
		},
		States: []program.WorkflowStateDeclaration{
			{
				Name: "Start",
				Transitions: []program.TransitionDeclaration{
					{
						Name:   "workflow_started",
						Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
						Operations: program.Block{
							Operations: []program.Operation{
								program.BeginTaskGroupOperation{Slot: "teamSelections", Completion: program.TaskGroupAllTerminalPolicy{}},
								program.ForEachOperation{
									Collection: program.FieldExpression{Target: program.ReferenceExpression{Name: "global"}, Field: "teams"},
									ItemName:   "team",
									Body: program.Block{
										Operations: []program.Operation{
											program.SpawnTaskGroupChildOperation{
												Slot: "teamSelections",
												Key:  program.FieldExpression{Target: program.ReferenceExpression{Name: "team"}, Field: "id"},
												Arguments: []program.CallArgument{
													{Name: "team", Value: program.ReferenceExpression{Name: "team"}},
												},
											},
										},
									},
								},
								program.SealTaskGroupOperation{Slot: "teamSelections"},
								program.ScheduleTimerOperation{Slot: "deadline", DelayMilliseconds: program.NumberLiteralExpression{Value: "30000"}},
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
						Name:   "task_group_completed",
						Signal: program.SignalPattern{Source: program.TaskGroupCompletedSignalSource{Slot: "teamSelections"}, Bindings: []program.SignalBinding{{Field: "results", Name: "results"}}},
						Operations: program.Block{
							Operations: []program.Operation{
								program.CancelTimerOperation{Slot: "deadline"},
							},
						},
						Control: program.CompleteControl{Result: program.ReferenceExpression{Name: "results"}},
					},
					{
						Name:   "deadline_expired",
						Signal: program.SignalPattern{Source: program.TimerExpiredSignalSource{Slot: "deadline"}},
						Operations: program.Block{
							Operations: []program.Operation{program.FinalizeTaskGroupOperation{Slot: "teamSelections"}},
						},
						Control: program.StayControl{},
					},
				},
			},
		},
	}

	raw, err := encodeWorkflowDeclaration("$", original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeWorkflowDeclaration("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", original, decoded)
	}
}

func TestExactJSON_WorkflowDeclaration(t *testing.T) {
	raw, err := encodeWorkflowDeclaration("$", program.WorkflowDeclaration{
		Name:         "PlayTurn",
		ResultType:   program.NamedTypeReference{Name: "TurnResult"},
		InitialState: "Start",
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	expectedKeys := []string{
		"name", "parameters", "result_type", "local_state", "question_slots",
		"ask_group_slots", "timer_slots", "child_slots", "task_group_slots",
		"presentations", "initial_state", "global_transitions", "states",
	}
	if len(obj) != len(expectedKeys) {
		t.Fatalf("expected %d fields, got %d: %#v", len(expectedKeys), len(obj), obj)
	}
	for _, key := range expectedKeys {
		if _, ok := obj[key]; !ok {
			t.Fatalf("missing expected field %q in %#v", key, obj)
		}
	}
}

func TestDecode_WorkflowDeclaration_Null(t *testing.T) {
	_, err := decodeWorkflowDeclaration("$", json.RawMessage("null"))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

func TestDecode_UnknownField_WorkflowDeclaration(t *testing.T) {
	_, err := decodeWorkflowDeclaration("$", json.RawMessage(`{
		"name":"x","parameters":[],"result_type":null,"local_state":{"fields":[]},
		"question_slots":[],"ask_group_slots":[],"timer_slots":[],"child_slots":[],
		"task_group_slots":[],"presentations":[],"initial_state":"S",
		"global_transitions":[],"states":[],"extra":true
	}`))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

func TestDecode_NestedPathFailure_QuestionSlotPresentation(t *testing.T) {
	data := json.RawMessage(`{
		"name":"x","parameters":[],"result_type":null,"local_state":{"fields":[]},
		"question_slots":[{"name":"s","question":"q","presentation":{"slot":"p","projection":"pr","projection_arguments":[],"view":"v","extra":true}}],
		"ask_group_slots":[],"timer_slots":[],"child_slots":[],
		"task_group_slots":[],"presentations":[],"initial_state":"S",
		"global_transitions":[],"states":[]
	}`)
	_, err := decodeWorkflowDeclaration("$", data)
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$.question_slots[0].presentation" {
		t.Fatalf("expected path $.question_slots[0].presentation, got %q", decodeErr.Path)
	}
}

func TestDecode_NestedPathFailure_WorkflowTaskGroupSlotKeyType(t *testing.T) {
	data := json.RawMessage(`{
		"name":"x","parameters":[],"result_type":null,"local_state":{"fields":[]},
		"question_slots":[],"ask_group_slots":[],"timer_slots":[],"child_slots":[],
		"task_group_slots":[{"name":"s","workflow":"w","key_type":{"kind":"not_a_real_type"}}],
		"presentations":[],"initial_state":"S",
		"global_transitions":[],"states":[]
	}`)
	_, err := decodeWorkflowDeclaration("$", data)
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$.task_group_slots[0].key_type" {
		t.Fatalf("expected path $.task_group_slots[0].key_type, got %q", decodeErr.Path)
	}
}

func TestDecode_NestedPathFailure_StatesTransitionControl(t *testing.T) {
	data := json.RawMessage(`{
		"name":"x","parameters":[],"result_type":null,"local_state":{"fields":[]},
		"question_slots":[],"ask_group_slots":[],"timer_slots":[],"child_slots":[],
		"task_group_slots":[],"presentations":[],"initial_state":"S",
		"global_transitions":[],
		"states":[
			{"name":"A","presentations":[],"transitions":[]},
			{"name":"B","presentations":[],"transitions":[
				{"name":"t","signal":{"source":{"kind":"named","name":"WorkflowStarted"},"bindings":[]},"guard":null,"operations":{"operations":[]},"control":{"kind":"not_a_real_control"}}
			]}
		]
	}`)
	_, err := decodeWorkflowDeclaration("$", data)
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$.states[1].transitions[0].control" {
		t.Fatalf("expected path $.states[1].transitions[0].control, got %q", decodeErr.Path)
	}
}
