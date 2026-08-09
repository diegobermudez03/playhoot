package program

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

// --- transition ---

func TestTransitionDeclaration_RoundTrip_Full(t *testing.T) {
	original := TransitionDeclaration{
		Name: "handle_move",
		Signal: SignalPattern{
			Source: QuestionAnsweredSignalSource{Slot: "moveRequest"},
			Bindings: []SignalBinding{
				{Field: "respondent", Name: "user"},
				{Field: "answer", Name: "selectedMove"},
			},
		},
		Guard: BinaryExpression{
			Operator: BinaryOperatorEqual,
			Left:     ReferenceExpression{Name: "user"},
			Right:    FieldExpression{Target: ReferenceExpression{Name: "local"}, Field: "participant"},
		},
		Operations: Block{
			Operations: []Operation{
				SetOperation{
					Target: FieldTarget{Target: NameTarget{Name: "local"}, Field: "selectedMove"},
					Value:  ReferenceExpression{Name: "selectedMove"},
				},
			},
		},
		Control: GotoControl{State: "ResolveMove"},
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
	cases := []TransitionDeclaration{
		{Name: "", Signal: SignalPattern{Source: NamedSignalSource{Name: "WorkflowStarted"}}, Guard: nil, Control: nil},
		{
			Name:       "empty_ops",
			Signal:     SignalPattern{Source: NamedSignalSource{Name: "WorkflowStarted"}},
			Operations: Block{Operations: []Operation{}},
			Control:    StayControl{},
		},
		{
			Name: "duplicate_bindings",
			Signal: SignalPattern{
				Source: UserIntentSignalSource{Intent: "PlayCard"},
				Bindings: []SignalBinding{
					{Field: "card", Name: "a"},
					{Field: "card", Name: "a"},
				},
			},
			Control: StayControl{},
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
	raw, err := encodeTransitionDeclaration("$", TransitionDeclaration{
		Name:    "handle_move",
		Signal:  SignalPattern{Source: QuestionAnsweredSignalSource{Slot: "moveRequest"}},
		Control: GotoControl{State: "ResolveMove"},
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
	original := WorkflowStateDeclaration{
		Name: "WaitingForMove",
		Presentations: []PresentationDeclaration{
			{Name: "MoveStatus", Slot: "hud", Targets: ReferenceExpression{Name: "roomUsers"}, Projection: "MoveStatusProjection", View: "MoveStatusView"},
			{Name: "MoveStatus", Slot: "hud", Targets: ReferenceExpression{Name: "roomUsers"}, Projection: "MoveStatusProjection", View: "MoveStatusView"},
		},
		Transitions: []TransitionDeclaration{
			{
				Name:    "handle_move",
				Signal:  SignalPattern{Source: QuestionAnsweredSignalSource{Slot: "moveRequest"}},
				Control: GotoControl{State: "ResolveMove"},
			},
			{
				Name:    "duplicate_same_signal",
				Signal:  SignalPattern{Source: QuestionAnsweredSignalSource{Slot: "moveRequest"}},
				Control: StayControl{},
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
	nilCase := WorkflowStateDeclaration{Name: "S", Presentations: nil, Transitions: nil}
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

	emptyCase := WorkflowStateDeclaration{Name: "S", Presentations: []PresentationDeclaration{}, Transitions: []TransitionDeclaration{}}
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
	original := WorkflowDeclaration{
		Name:       "PlayTurn",
		Parameters: []FieldDeclaration{{Name: "participant", Type: NamedTypeReference{Name: "ParticipantId"}}},
		ResultType: NamedTypeReference{Name: "TurnResult"},
		LocalState: StateDeclaration{
			Fields: []StateFieldDeclaration{
				{Name: "consecutivePairs", Type: BuiltinTypeReference{Type: BuiltinTypeNumber}, Initializer: NumberLiteralExpression{Value: "0"}},
			},
		},
		QuestionSlots: []QuestionSlotDeclaration{
			{
				Name:     "moveRequest",
				Question: "ChooseMove",
				Presentation: &QuestionPresentationDeclaration{
					Slot:                "primaryInteraction",
					Projection:          "ChooseMoveProjection",
					ProjectionArguments: []CallArgument{{Name: "plans", Value: ReferenceExpression{Name: "plans"}}},
					View:                "ChooseMoveView",
				},
			},
		},
		AskGroupSlots: []AskGroupSlotDeclaration{
			{Name: "votes", Question: "ChooseCard"},
		},
		TimerSlots: []TimerSlotDeclaration{{Name: "moveDeadline"}},
		ChildSlots: []ChildWorkflowSlotDeclaration{{Name: "activeTurn", Workflow: "PlayTurn"}},
		TaskGroupSlots: []TaskGroupSlotDeclaration{
			{Name: "teamSelections", Workflow: "TeamChooseCard", KeyType: NamedTypeReference{Name: "TeamId"}},
		},
		Presentations: []PresentationDeclaration{
			{Name: "MatchBoard", Slot: "main", Targets: ReferenceExpression{Name: "roomUsers"}, Projection: "BoardProjection", View: "BoardView"},
		},
		InitialState: "Start",
		GlobalTransitions: []TransitionDeclaration{
			{Name: "cancel_on_session", Signal: SignalPattern{Source: NamedSignalSource{Name: "SessionCancelled"}}, Control: CancelControl{Reason: StringLiteralExpression{Value: "session cancelled"}}},
		},
		States: []WorkflowStateDeclaration{
			{Name: "Start", Transitions: []TransitionDeclaration{{Name: "start", Signal: SignalPattern{Source: NamedSignalSource{Name: "WorkflowStarted"}}, Control: GotoControl{State: "Waiting"}}}},
			{Name: "Waiting", Transitions: []TransitionDeclaration{}},
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
	original := WorkflowDeclaration{
		Name:       "MatchRound",
		ResultType: NamedTypeReference{Name: "RoundResult"},
		QuestionSlots: []QuestionSlotDeclaration{
			{Name: "confirmation", Question: "ConfirmAction"},
		},
		AskGroupSlots: []AskGroupSlotDeclaration{
			{Name: "votes", Question: "ChooseCard"},
		},
		TimerSlots: []TimerSlotDeclaration{{Name: "deadline"}},
		ChildSlots: []ChildWorkflowSlotDeclaration{{Name: "activeTurn", Workflow: "PlayTurn"}},
		TaskGroupSlots: []TaskGroupSlotDeclaration{
			{Name: "teamSelections", Workflow: "TeamChooseCard", KeyType: NamedTypeReference{Name: "TeamId"}},
		},
		Presentations: []PresentationDeclaration{
			{Name: "MatchBoard", Slot: "main", Targets: ReferenceExpression{Name: "roomUsers"}, Projection: "BoardProjection", View: "BoardView"},
		},
		InitialState: "Start",
		GlobalTransitions: []TransitionDeclaration{
			{
				Name:    "session_cancelled",
				Signal:  SignalPattern{Source: NamedSignalSource{Name: "SessionCancelled"}},
				Control: CancelControl{Reason: StringLiteralExpression{Value: "session ended"}},
			},
		},
		States: []WorkflowStateDeclaration{
			{
				Name: "Start",
				Transitions: []TransitionDeclaration{
					{
						Name:   "workflow_started",
						Signal: SignalPattern{Source: NamedSignalSource{Name: "WorkflowStarted"}},
						Operations: Block{
							Operations: []Operation{
								BeginTaskGroupOperation{Slot: "teamSelections", Completion: TaskGroupAllTerminalPolicy{}},
								ForEachOperation{
									Collection: FieldExpression{Target: ReferenceExpression{Name: "global"}, Field: "teams"},
									ItemName:   "team",
									Body: Block{
										Operations: []Operation{
											SpawnTaskGroupChildOperation{
												Slot: "teamSelections",
												Key:  FieldExpression{Target: ReferenceExpression{Name: "team"}, Field: "id"},
												Arguments: []CallArgument{
													{Name: "team", Value: ReferenceExpression{Name: "team"}},
												},
											},
										},
									},
								},
								SealTaskGroupOperation{Slot: "teamSelections"},
								ScheduleTimerOperation{Slot: "deadline", DelayMilliseconds: NumberLiteralExpression{Value: "30000"}},
							},
						},
						Control: GotoControl{State: "Waiting"},
					},
				},
			},
			{
				Name: "Waiting",
				Transitions: []TransitionDeclaration{
					{
						Name:   "task_group_completed",
						Signal: SignalPattern{Source: TaskGroupCompletedSignalSource{Slot: "teamSelections"}, Bindings: []SignalBinding{{Field: "results", Name: "results"}}},
						Operations: Block{
							Operations: []Operation{
								CancelTimerOperation{Slot: "deadline"},
							},
						},
						Control: CompleteControl{Result: ReferenceExpression{Name: "results"}},
					},
					{
						Name:   "deadline_expired",
						Signal: SignalPattern{Source: TimerExpiredSignalSource{Slot: "deadline"}},
						Operations: Block{
							Operations: []Operation{FinalizeTaskGroupOperation{Slot: "teamSelections"}},
						},
						Control: StayControl{},
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
	raw, err := encodeWorkflowDeclaration("$", WorkflowDeclaration{
		Name:         "PlayTurn",
		ResultType:   NamedTypeReference{Name: "TurnResult"},
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
