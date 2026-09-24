package runtime_test

import (
	"testing"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/internal/runtime"
)

func findInstanceKeyedAskGroupSlot(instance engine.WorkflowInstance, name string) (engine.KeyedAskGroupSlotInstance, bool) {
	for _, s := range instance.KeyedAskGroupSlots {
		if s.Name == name {
			return s, true
		}
	}
	return engine.KeyedAskGroupSlotInstance{}, false
}

func findKeyedAskGroupPendingByKey(pending []engine.KeyedPendingAskGroup, key string) (engine.KeyedPendingAskGroup, bool) {
	for _, p := range pending {
		if p.Key.(engine.StringValue).Value == key {
			return p, true
		}
	}
	return engine.KeyedPendingAskGroup{}, false
}

// keyedAskGroupProgram builds a hand-assembled engine.Program (bypassing
// compiler.Compile, mirroring ask_group_exec_test.go's style): a "Main"
// workflow taking a list<user> parameter "recipients", with one keyed
// ask-group slot "Team" (keyed by string) backed by a bool-answering
// question "Confirm". Open/Finalize/Cancel each take their target Key
// from a UserIntentSignalSource's bound "key" field, exactly like
// keyed_interaction_exec_test.go's pattern, so each delivery can target
// a different (slot, key) occurrence. The Join transition binds
// "key"/"responses"/"respondents"/"missing" and completes with the
// bound key, so the test can observe both which occurrence completed
// and its outcome data.
func keyedAskGroupProgram(completion engine.AskGroupCompletionPolicy) engine.Program {
	keyRef := engine.ReferenceExpression{Name: "key"}
	openOp := engine.OpenKeyedAskGroupOperation{Slot: "Team", Key: keyRef, Recipients: engine.ReferenceExpression{Name: "recipients"}, Completion: completion}
	finalizeOp := engine.FinalizeKeyedAskGroupOperation{Slot: "Team", Key: keyRef}
	cancelOp := engine.CancelKeyedAskGroupOperation{Slot: "Team", Key: keyRef}

	intentSignal := func(intent string) engine.SignalPattern {
		return engine.SignalPattern{Source: engine.UserIntentSignalSource{Intent: intent}, Bindings: []engine.SignalBinding{{Field: "key", Name: "key"}}}
	}

	main := engine.Workflow{
		Name:               "Main",
		Parameters:         []engine.FieldType{{Name: "recipients", Type: engine.ListType{Element: engine.UserType{}}}},
		ResultType:         engine.StringType{},
		KeyedAskGroupSlots: []engine.KeyedAskGroupSlot{{Name: "Team", Question: "Confirm", KeyType: engine.StringType{}}},
		InitialState:       "S",
		States: []engine.WorkflowState{
			{
				Name: "S",
				Transitions: []engine.Transition{
					{Name: "Open", Signal: intentSignal("Open"), Operations: engine.Block{Operations: []engine.Operation{openOp}}, Control: engine.StayControl{}},
					{Name: "Finalize", Signal: intentSignal("Finalize"), Operations: engine.Block{Operations: []engine.Operation{finalizeOp}}, Control: engine.StayControl{}},
					{Name: "CancelAsk", Signal: intentSignal("CancelAsk"), Operations: engine.Block{Operations: []engine.Operation{cancelOp}}, Control: engine.StayControl{}},
					{
						Name: "Join",
						Signal: engine.SignalPattern{
							Source: engine.KeyedAskGroupCompletedSignalSource{Slot: "Team"},
							Bindings: []engine.SignalBinding{
								{Field: "key", Name: "k"},
								{Field: "missing", Name: "m"},
							},
						},
						Control: engine.CompleteControl{Result: engine.ReferenceExpression{Name: "k"}},
					},
				},
			},
		},
	}

	return engine.Program{
		RootWorkflow: "Main",
		Workflows:    map[string]engine.Workflow{"Main": main},
		Questions: map[string]engine.Question{
			"Confirm": {Name: "Confirm", ResponseType: engine.BoolType{}},
		},
	}
}

func keyedAskGroupSnapshot(recipients []engine.UserID) engine.Snapshot {
	elements := make([]engine.Value, len(recipients))
	for i, r := range recipients {
		elements[i] = engine.UserValue{ID: r}
	}
	return engine.Snapshot{
		GlobalState: engine.RecordValue{TypeName: "global"},
		Root: engine.WorkflowInstance{
			Workflow:           "Main",
			State:              "S",
			Parameters:         []engine.FieldValue{{Name: "recipients", Value: engine.ListValue{ElementType: engine.UserType{}, Elements: elements}}},
			LocalState:         engine.RecordValue{TypeName: "local"},
			KeyedAskGroupSlots: []engine.KeyedAskGroupSlotInstance{{Name: "Team"}},
		},
	}
}

func openKeyedAskGroup(p engine.Program, snap engine.Snapshot, key string) (engine.Commit, error) {
	return runtime.Step(p, snap, engine.Signal{Kind: engine.SignalKindIntent, Intent: "Open", Fields: map[string]engine.Value{"key": engine.StringValue{Value: key}}}, engine.DefaultLimits())
}

// keyedAskGroupInteractionID looks up the InteractionID currently
// assigned to snap's "Team" slot occurrence at key, or the zero value
// (never a real assignment) if that key was never opened.
func keyedAskGroupInteractionID(snap engine.Snapshot, key string) engine.InteractionID {
	slot, _ := findInstanceKeyedAskGroupSlot(snap.Root, "Team")
	entry, _ := findKeyedAskGroupPendingByKey(slot.Pending, key)
	return entry.InteractionID
}

func answerKeyedAskGroup(p engine.Program, snap engine.Snapshot, key string, respondent engine.UserID, answer bool) (engine.Commit, error) {
	return runtime.Step(p, snap, engine.Signal{
		Kind:          engine.SignalKindInteractionAnswered,
		InteractionID: keyedAskGroupInteractionID(snap, key),
		Respondent:    respondent,
		Answer:        engine.BoolValue{Value: answer},
	}, engine.DefaultLimits())
}

func TestExec_OpenKeyedAskGroup_DifferentKeysAreIndependentlyCollecting(t *testing.T) {
	p := keyedAskGroupProgram(engine.AskGroupAllResponsesPolicy{})
	snap := keyedAskGroupSnapshot([]engine.UserID{askAlice, askBob})

	commit, err := openKeyedAskGroup(p, snap, "team_a")
	if err != nil {
		t.Fatalf("unexpected error opening team_a: %v", err)
	}
	commit, err = openKeyedAskGroup(p, commit.Snapshot, "team_b")
	if err != nil {
		t.Fatalf("unexpected error opening team_b (must not contend with team_a): %v", err)
	}

	slot, ok := findInstanceKeyedAskGroupSlot(commit.Snapshot.Root, "Team")
	if !ok || len(slot.Pending) != 2 {
		t.Fatalf("expected both keys simultaneously collecting, got %+v", slot)
	}
}

func TestExec_OpenKeyedAskGroup_SameKeyOccupiedFailsAtomically(t *testing.T) {
	p := keyedAskGroupProgram(engine.AskGroupAllResponsesPolicy{})
	snap := keyedAskGroupSnapshot([]engine.UserID{askAlice})
	commit, err := openKeyedAskGroup(p, snap, "team_a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = openKeyedAskGroup(p, commit.Snapshot, "team_a")
	if e, ok := err.(*runtime.ExecutionError); !ok || e.Code != runtime.ExecutionErrorSlotOccupied {
		t.Fatalf("expected runtime.ExecutionErrorSlotOccupied opening an already-occupied key, got %v", err)
	}
}

func TestExec_KeyedAskGroupAnswered_RoutesToTheCorrectKeyOnly(t *testing.T) {
	p := keyedAskGroupProgram(engine.AskGroupAllResponsesPolicy{})
	snap := keyedAskGroupSnapshot([]engine.UserID{askAlice, askBob})
	commit, _ := openKeyedAskGroup(p, snap, "team_a")
	commit, err := openKeyedAskGroup(p, commit.Snapshot, "team_b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	commit, err = answerKeyedAskGroup(p, commit.Snapshot, "team_a", askAlice, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	slot, _ := findInstanceKeyedAskGroupSlot(commit.Snapshot.Root, "Team")
	teamA, _ := findKeyedAskGroupPendingByKey(slot.Pending, "team_a")
	if teamA.Completed {
		t.Fatal("expected team_a to still be collecting after one of two answers")
	}
	teamB, _ := findKeyedAskGroupPendingByKey(slot.Pending, "team_b")
	if len(teamB.Responses) != 0 {
		t.Fatalf("expected team_b to be entirely unaffected by team_a's answer, got %+v", teamB)
	}

	commit, err = answerKeyedAskGroup(p, commit.Snapshot, "team_a", askBob, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	slot, _ = findInstanceKeyedAskGroupSlot(commit.Snapshot.Root, "Team")
	teamA, _ = findKeyedAskGroupPendingByKey(slot.Pending, "team_a")
	if !teamA.Completed {
		t.Fatal("expected team_a to complete once both its recipients answered")
	}
	teamB, _ = findKeyedAskGroupPendingByKey(slot.Pending, "team_b")
	if teamB.Completed {
		t.Fatal("expected team_b to remain collecting, unaffected by team_a completing")
	}
}

func TestExec_KeyedAskGroupAnswered_UnknownKeyRejected(t *testing.T) {
	p := keyedAskGroupProgram(engine.AskGroupAllResponsesPolicy{})
	snap := keyedAskGroupSnapshot([]engine.UserID{askAlice})

	_, err := answerKeyedAskGroup(p, snap, "team_a", askAlice, true)
	if err != runtime.ErrInputRejected {
		t.Fatalf("expected runtime.ErrInputRejected answering a key that was never opened, got %v", err)
	}
}

func TestExec_FinalizeKeyedAskGroup_ScopedToOneKey(t *testing.T) {
	p := keyedAskGroupProgram(engine.AskGroupAllResponsesPolicy{})
	snap := keyedAskGroupSnapshot([]engine.UserID{askAlice, askBob})
	commit, _ := openKeyedAskGroup(p, snap, "team_a")
	commit, err := openKeyedAskGroup(p, commit.Snapshot, "team_b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	commit, err = answerKeyedAskGroup(p, commit.Snapshot, "team_a", askAlice, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	commit, err = runtime.Step(p, commit.Snapshot, engine.Signal{
		Kind: engine.SignalKindIntent, Intent: "Finalize", Fields: map[string]engine.Value{"key": engine.StringValue{Value: "team_a"}},
	}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error finalizing team_a: %v", err)
	}
	slot, _ := findInstanceKeyedAskGroupSlot(commit.Snapshot.Root, "Team")
	teamA, _ := findKeyedAskGroupPendingByKey(slot.Pending, "team_a")
	if !teamA.Completed {
		t.Fatal("expected finalize to complete team_a")
	}
	teamB, _ := findKeyedAskGroupPendingByKey(slot.Pending, "team_b")
	if teamB.Completed {
		t.Fatal("expected team_b to remain collecting, untouched by finalizing team_a")
	}
}

func TestExec_CancelKeyedAskGroup_ScopedToOneKey(t *testing.T) {
	p := keyedAskGroupProgram(engine.AskGroupAllResponsesPolicy{})
	snap := keyedAskGroupSnapshot([]engine.UserID{askAlice, askBob})
	commit, _ := openKeyedAskGroup(p, snap, "team_a")
	commit, err := openKeyedAskGroup(p, commit.Snapshot, "team_b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	commit, err = runtime.Step(p, commit.Snapshot, engine.Signal{
		Kind: engine.SignalKindIntent, Intent: "CancelAsk", Fields: map[string]engine.Value{"key": engine.StringValue{Value: "team_a"}},
	}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error cancelling team_a: %v", err)
	}
	slot, _ := findInstanceKeyedAskGroupSlot(commit.Snapshot.Root, "Team")
	if _, ok := findKeyedAskGroupPendingByKey(slot.Pending, "team_a"); ok {
		t.Fatal("expected team_a to be cancelled and cleared")
	}
	if _, ok := findKeyedAskGroupPendingByKey(slot.Pending, "team_b"); !ok {
		t.Fatal("expected team_b to remain collecting, untouched by cancelling team_a")
	}
}

func TestExec_KeyedAskGroupJoin_BindsKeyAndCompletesTheRightOccurrence(t *testing.T) {
	p := keyedAskGroupProgram(engine.AskGroupAllResponsesPolicy{})
	snap := keyedAskGroupSnapshot([]engine.UserID{askAlice})
	commit, _ := openKeyedAskGroup(p, snap, "team_a")
	commit, err := answerKeyedAskGroup(p, commit.Snapshot, "team_a", askAlice, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	commit, err = runtime.Step(p, commit.Snapshot, engine.Signal{Kind: engine.SignalKindInteractionCompleted, InteractionID: keyedAskGroupInteractionID(commit.Snapshot, "team_a")}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error joining: %v", err)
	}
	if commit.Snapshot.Root.Outcome == nil || commit.Snapshot.Root.Outcome.Kind != engine.WorkflowOutcomeCompleted {
		t.Fatalf("expected the Join transition to complete the workflow, got %+v", commit.Snapshot.Root.Outcome)
	}
	if got := commit.Snapshot.Root.Outcome.Result.(engine.StringValue).Value; got != "team_a" {
		t.Fatalf("expected the Join transition's bound key to be %q, got %q", "team_a", got)
	}
}
