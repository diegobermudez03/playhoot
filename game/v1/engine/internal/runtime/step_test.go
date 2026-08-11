package runtime_test

import (
	"testing"

	"github.com/diegobermudez03/playhoot/game/v1/engine"
	"github.com/diegobermudez03/playhoot/game/v1/engine/internal/runtime"
)

// globalCountField is the AssignmentTarget for global.count, reused by
// several tests below.
var globalCountField = engine.FieldTarget{Target: engine.NameTarget{Name: "global"}, Field: "count"}

func counterProgram() engine.Program {
	incrementOp := engine.SetOperation{
		Target: globalCountField,
		Value: engine.BinaryExpression{
			Operator: engine.BinaryOperatorAdd,
			Left:     engine.FieldExpression{Target: engine.ReferenceExpression{Name: "global"}, Field: "count"},
			Right:    engine.NumberLiteralExpression{Value: 1},
		},
	}
	finishGuard := engine.BinaryExpression{
		Operator: engine.BinaryOperatorGreaterOrEqual,
		Left:     engine.FieldExpression{Target: engine.ReferenceExpression{Name: "global"}, Field: "count"},
		Right:    engine.NumberLiteralExpression{Value: 3},
	}

	workflow := engine.Workflow{
		Name:         "Counter",
		ResultType:   engine.NumberType{},
		InitialState: "Running",
		States: []engine.WorkflowState{
			{
				Name: "Running",
				Transitions: []engine.Transition{
					{
						Name:       "Increment",
						Signal:     engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Increment"}},
						Operations: engine.Block{Operations: []engine.Operation{incrementOp}},
						Control:    engine.StayControl{},
					},
					{
						Name:    "Finish",
						Signal:  engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Finish"}},
						Guard:   finishGuard,
						Control: engine.CompleteControl{Result: engine.FieldExpression{Target: engine.ReferenceExpression{Name: "global"}, Field: "count"}},
					},
				},
			},
		},
		GlobalTransitions: []engine.Transition{
			{
				Name:    "Abort",
				Signal:  engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Abort"}},
				Control: engine.CancelControl{Reason: engine.StringLiteralExpression{Value: "aborted"}},
			},
		},
	}

	return engine.Program{
		RootWorkflow: "Counter",
		Workflows:    map[string]engine.Workflow{"Counter": workflow},
	}
}

func counterSnapshot(count float64) engine.Snapshot {
	return engine.Snapshot{
		GlobalState: engine.RecordValue{TypeName: "global", Fields: []engine.FieldValue{{Name: "count", Value: engine.NumberValue{Value: count}}}},
		Root: engine.WorkflowInstance{
			Workflow:   "Counter",
			State:      "Running",
			LocalState: engine.RecordValue{TypeName: "local"},
		},
	}
}

func TestStep_IncrementsGlobalStateAndStays(t *testing.T) {
	p := counterProgram()
	snap := counterSnapshot(0)

	commit, err := runtime.Step(p, snap, engine.Signal{Name: "Increment"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	count, _ := commit.Snapshot.GlobalState.FieldByName("count")
	if count.Value.(engine.NumberValue).Value != 1 {
		t.Fatalf("got %v, want 1", count.Value)
	}
	if commit.Snapshot.Root.State != "Running" {
		t.Fatalf("expected StayControl to keep state Running, got %q", commit.Snapshot.Root.State)
	}
	if commit.Snapshot.Sequence != 1 {
		t.Fatalf("got sequence %d, want 1", commit.Snapshot.Sequence)
	}
	if commit.ConsumedSignal.Name != "Increment" {
		t.Fatalf("got %+v", commit.ConsumedSignal)
	}

	// Original snapshot must remain unchanged (atomicity / no mutation
	// in place).
	origCount, _ := snap.GlobalState.FieldByName("count")
	if origCount.Value.(engine.NumberValue).Value != 0 {
		t.Fatalf("original snapshot was mutated: %v", origCount.Value)
	}
}

func TestStep_SequentialStepsCanStopAndResume(t *testing.T) {
	p := counterProgram()
	snap := counterSnapshot(0)

	for i := 0; i < 3; i++ {
		commit, err := runtime.Step(p, snap, engine.Signal{Name: "Increment"}, engine.DefaultLimits())
		if err != nil {
			t.Fatalf("step %d: unexpected error: %v", i, err)
		}
		snap = commit.Snapshot // simulate persisting and resuming from the returned snapshot
	}

	commit, err := runtime.Step(p, snap, engine.Signal{Name: "Finish"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if commit.Snapshot.Root.Outcome == nil || commit.Snapshot.Root.Outcome.Kind != engine.WorkflowOutcomeCompleted {
		t.Fatalf("got %+v", commit.Snapshot.Root.Outcome)
	}
	if commit.Snapshot.Root.Outcome.Result.(engine.NumberValue).Value != 3 {
		t.Fatalf("got result %v, want 3", commit.Snapshot.Root.Outcome.Result)
	}

	var completed *engine.WorkflowCompletedOutput
	for _, o := range commit.Outputs {
		if c, ok := o.(engine.WorkflowCompletedOutput); ok {
			completed = &c
		}
	}
	if completed == nil {
		t.Fatalf("expected a WorkflowCompletedOutput, got %+v", commit.Outputs)
	}
	if len(completed.Path) != 0 || completed.Workflow != "Counter" || completed.Outcome.Kind != engine.WorkflowOutcomeCompleted {
		t.Fatalf("got %+v", completed)
	}
}

func TestStep_GuardFalseRejectsSignal(t *testing.T) {
	p := counterProgram()
	snap := counterSnapshot(0) // count is 0, Finish's guard requires >= 3

	_, err := runtime.Step(p, snap, engine.Signal{Name: "Finish"}, engine.DefaultLimits())
	if err != runtime.ErrSignalRejected {
		t.Fatalf("expected runtime.ErrSignalRejected, got %v", err)
	}
}

func TestStep_NoMatchingTransitionRejectsSignal(t *testing.T) {
	p := counterProgram()
	snap := counterSnapshot(0)

	_, err := runtime.Step(p, snap, engine.Signal{Name: "Nonexistent"}, engine.DefaultLimits())
	if err != runtime.ErrSignalRejected {
		t.Fatalf("expected runtime.ErrSignalRejected, got %v", err)
	}
}

func TestStep_GlobalTransitionFallback(t *testing.T) {
	p := counterProgram()
	snap := counterSnapshot(0)

	commit, err := runtime.Step(p, snap, engine.Signal{Name: "Abort"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if commit.Snapshot.Root.Outcome == nil || commit.Snapshot.Root.Outcome.Kind != engine.WorkflowOutcomeCancelled {
		t.Fatalf("got %+v", commit.Snapshot.Root.Outcome)
	}
	if commit.Snapshot.Root.Outcome.Reason != "aborted" {
		t.Fatalf("got reason %q", commit.Snapshot.Root.Outcome.Reason)
	}
}

func TestStep_StateLocalTransitionTakesPrecedenceOverGlobal(t *testing.T) {
	// Declare a state-local transition for the same signal name ("Abort")
	// that the global transition also handles, with a different,
	// distinguishable outcome (Fail instead of Cancel). The state-local
	// one must win.
	p := counterProgram()
	wf := p.Workflows["Counter"]
	wf.States[0].Transitions = append(wf.States[0].Transitions, engine.Transition{
		Name:    "LocalAbort",
		Signal:  engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Abort"}},
		Control: engine.FailControl{Error: engine.StringLiteralExpression{Value: "local abort"}},
	})
	p.Workflows["Counter"] = wf

	commit, err := runtime.Step(p, counterSnapshot(0), engine.Signal{Name: "Abort"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if commit.Snapshot.Root.Outcome == nil || commit.Snapshot.Root.Outcome.Kind != engine.WorkflowOutcomeFailed {
		t.Fatalf("expected the state-local Fail to win over the global Cancel, got %+v", commit.Snapshot.Root.Outcome)
	}
}

func TestStep_TerminatedInstanceRejectsFurtherSignals(t *testing.T) {
	p := counterProgram()
	snap := counterSnapshot(3)

	commit, err := runtime.Step(p, snap, engine.Signal{Name: "Finish"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = runtime.Step(p, commit.Snapshot, engine.Signal{Name: "Increment"}, engine.DefaultLimits())
	if err != runtime.ErrSignalRejected {
		t.Fatalf("expected a terminated instance to reject further signals, got %v", err)
	}
}

func TestStep_SnapshotProgramMismatch(t *testing.T) {
	p := counterProgram()
	snap := counterSnapshot(0)
	snap.Root.Workflow = "SomeOtherWorkflow"

	_, err := runtime.Step(p, snap, engine.Signal{Name: "Increment"}, engine.DefaultLimits())
	execErr, ok := err.(*runtime.ExecutionError)
	if !ok || execErr.Code != runtime.ExecutionErrorSnapshotProgramMismatch {
		t.Fatalf("expected runtime.ExecutionErrorSnapshotProgramMismatch, got %v", err)
	}
}

func TestStep_InvariantViolationAfterTransitionRejectsAtomically(t *testing.T) {
	p := counterProgram()
	p.Invariants = []engine.Invariant{
		{
			Name: "CountBelowTwo",
			Condition: engine.BinaryExpression{
				Operator: engine.BinaryOperatorLess,
				Left:     engine.FieldExpression{Target: engine.ReferenceExpression{Name: "global"}, Field: "count"},
				Right:    engine.NumberLiteralExpression{Value: 2},
			},
		},
	}
	snap := counterSnapshot(1) // one more increment pushes count to 2, violating the invariant

	_, err := runtime.Step(p, snap, engine.Signal{Name: "Increment"}, engine.DefaultLimits())
	execErr, ok := err.(*runtime.ExecutionError)
	if !ok || execErr.Code != runtime.ExecutionErrorInvariantViolation {
		t.Fatalf("expected runtime.ExecutionErrorInvariantViolation, got %v", err)
	}
	// Atomicity: the original snapshot must be unaffected.
	count, _ := snap.GlobalState.FieldByName("count")
	if count.Value.(engine.NumberValue).Value != 1 {
		t.Fatalf("original snapshot was mutated: %v", count.Value)
	}
}

func TestStep_LetBindingVisibleToControl(t *testing.T) {
	workflow := engine.Workflow{
		Name:         "LetDemo",
		ResultType:   engine.NumberType{},
		InitialState: "S",
		States: []engine.WorkflowState{
			{
				Name: "S",
				Transitions: []engine.Transition{
					{
						Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Go"}},
						Operations: engine.Block{Operations: []engine.Operation{
							engine.LetOperation{
								Name: "doubled",
								Value: engine.BinaryExpression{
									Operator: engine.BinaryOperatorMultiply,
									Left:     engine.FieldExpression{Target: engine.ReferenceExpression{Name: "global"}, Field: "count"},
									Right:    engine.NumberLiteralExpression{Value: 2},
								},
							},
						}},
						Control: engine.CompleteControl{Result: engine.ReferenceExpression{Name: "doubled"}},
					},
				},
			},
		},
	}
	p := engine.Program{RootWorkflow: "LetDemo", Workflows: map[string]engine.Workflow{"LetDemo": workflow}}
	snap := engine.Snapshot{
		GlobalState: engine.RecordValue{TypeName: "global", Fields: []engine.FieldValue{{Name: "count", Value: engine.NumberValue{Value: 5}}}},
		Root:        engine.WorkflowInstance{Workflow: "LetDemo", State: "S", LocalState: engine.RecordValue{TypeName: "local"}},
	}

	commit, err := runtime.Step(p, snap, engine.Signal{Name: "Go"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if commit.Snapshot.Root.Outcome.Result.(engine.NumberValue).Value != 10 {
		t.Fatalf("got %v, want 10", commit.Snapshot.Root.Outcome.Result)
	}
}

func TestStep_ListAndMapMutationsAndControlFlow(t *testing.T) {
	// global: { items: list<number>, scores: map<string, number> }
	// On "Go": append 4, insert 99 at index 0, remove element at index 2,
	// put "alice"->10 into scores, then delete "bob", using an If and a
	// ForEach for good measure.
	appendOp := engine.ListAppendOperation{
		Target: engine.FieldTarget{Target: engine.NameTarget{Name: "global"}, Field: "items"},
		Value:  engine.NumberLiteralExpression{Value: 4},
	}
	insertOp := engine.ListInsertOperation{
		Target: engine.FieldTarget{Target: engine.NameTarget{Name: "global"}, Field: "items"},
		Index:  engine.NumberLiteralExpression{Value: 0},
		Value:  engine.NumberLiteralExpression{Value: 99},
	}
	removeOp := engine.ListRemoveAtOperation{
		Target: engine.FieldTarget{Target: engine.NameTarget{Name: "global"}, Field: "items"},
		Index:  engine.NumberLiteralExpression{Value: 2},
	}
	putOp := engine.MapPutOperation{
		Target: engine.FieldTarget{Target: engine.NameTarget{Name: "global"}, Field: "scores"},
		Key:    engine.StringLiteralExpression{Value: "alice"},
		Value:  engine.NumberLiteralExpression{Value: 10},
	}
	deleteOp := engine.MapDeleteOperation{
		Target: engine.FieldTarget{Target: engine.NameTarget{Name: "global"}, Field: "scores"},
		Key:    engine.StringLiteralExpression{Value: "bob"},
	}
	ifOp := engine.IfOperation{
		Condition: engine.BoolLiteralExpression{Value: true},
		Then:      engine.Block{Operations: []engine.Operation{putOp}},
		Else:      engine.Block{Operations: []engine.Operation{deleteOp}},
	}
	forEachOp := engine.ForEachOperation{
		Collection: engine.ListExpression{ElementType: engine.NumberType{}, Elements: []engine.Expression{
			engine.NumberLiteralExpression{Value: 1}, engine.NumberLiteralExpression{Value: 2},
		}},
		ItemName: "n",
		Body: engine.Block{Operations: []engine.Operation{
			engine.ListAppendOperation{
				Target: engine.FieldTarget{Target: engine.NameTarget{Name: "global"}, Field: "items"},
				Value:  engine.ReferenceExpression{Name: "n"},
			},
		}},
	}

	workflow := engine.Workflow{
		Name:         "Mutate",
		ResultType:   engine.UnitType{},
		InitialState: "S",
		States: []engine.WorkflowState{
			{
				Name: "S",
				Transitions: []engine.Transition{
					{
						Signal:     engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Go"}},
						Operations: engine.Block{Operations: []engine.Operation{appendOp, insertOp, removeOp, ifOp, deleteOp, forEachOp}},
						Control:    engine.CompleteControl{Result: engine.UnitLiteralExpression{}},
					},
				},
			},
		},
	}
	p := engine.Program{RootWorkflow: "Mutate", Workflows: map[string]engine.Workflow{"Mutate": workflow}}
	snap := engine.Snapshot{
		GlobalState: engine.RecordValue{TypeName: "global", Fields: []engine.FieldValue{
			{Name: "items", Value: engine.ListValue{ElementType: engine.NumberType{}, Elements: []engine.Value{
				engine.NumberValue{Value: 1}, engine.NumberValue{Value: 2}, engine.NumberValue{Value: 3},
			}}},
			{Name: "scores", Value: engine.MapValue{KeyType: engine.StringType{}, ValueType: engine.NumberType{}, Entries: []engine.MapEntry{
				{Key: engine.StringValue{Value: "bob"}, Value: engine.NumberValue{Value: 5}},
			}}},
		}},
		Root: engine.WorkflowInstance{Workflow: "Mutate", State: "S", LocalState: engine.RecordValue{TypeName: "local"}},
	}

	commit, err := runtime.Step(p, snap, engine.Signal{Name: "Go"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	itemsField, _ := commit.Snapshot.GlobalState.FieldByName("items")
	items := itemsField.Value.(engine.ListValue).Elements
	// start [1,2,3] -> append 4 -> [1,2,3,4] -> insert 99@0 -> [99,1,2,3,4]
	// -> removeAt 2 -> [99,1,3,4] -> forEach appends 1,2 -> [99,1,3,4,1,2]
	want := []float64{99, 1, 3, 4, 1, 2}
	if len(items) != len(want) {
		t.Fatalf("got %v, want %v", items, want)
	}
	for i, w := range want {
		if items[i].(engine.NumberValue).Value != w {
			t.Fatalf("got %v, want %v", items, want)
		}
	}

	scoresField, _ := commit.Snapshot.GlobalState.FieldByName("scores")
	scores := scoresField.Value.(engine.MapValue).Entries
	// ifOp (condition true) put alice->10; scores now {bob:5, alice:10};
	// the deleteOp *after* ifOp then removes bob unconditionally.
	if len(scores) != 1 || scores[0].Key.(engine.StringValue).Value != "alice" || scores[0].Value.(engine.NumberValue).Value != 10 {
		t.Fatalf("got %+v", scores)
	}

	// Original snapshot's collections must remain untouched.
	origItemsField, _ := snap.GlobalState.FieldByName("items")
	if len(origItemsField.Value.(engine.ListValue).Elements) != 3 {
		t.Fatal("original snapshot's items slice was mutated")
	}
}

func TestStep_MatchControlSelectsCorrectBranch(t *testing.T) {
	workflow := engine.Workflow{
		Name:         "MatchDemo",
		ResultType:   engine.NumberType{},
		InitialState: "S",
		States: []engine.WorkflowState{
			{
				Name: "S",
				Transitions: []engine.Transition{
					{
						Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Go"}},
						Control: engine.MatchControl{
							Value: engine.EnumValueExpression{TypeName: "Outcome", ValueName: "Win"},
							Cases: []engine.MatchControlCase{
								{Pattern: engine.EnumValueMatchPattern{TypeName: "Outcome", ValueName: "Win"}, Control: engine.CompleteControl{Result: engine.NumberLiteralExpression{Value: 1}}},
								{Pattern: engine.WildcardMatchPattern{}, Control: engine.CompleteControl{Result: engine.NumberLiteralExpression{Value: 0}}},
							},
						},
					},
				},
			},
		},
	}
	p := engine.Program{RootWorkflow: "MatchDemo", Workflows: map[string]engine.Workflow{"MatchDemo": workflow}}
	snap := engine.Snapshot{
		GlobalState: engine.RecordValue{TypeName: "global"},
		Root:        engine.WorkflowInstance{Workflow: "MatchDemo", State: "S", LocalState: engine.RecordValue{TypeName: "local"}},
	}

	commit, err := runtime.Step(p, snap, engine.Signal{Name: "Go"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if commit.Snapshot.Root.Outcome.Result.(engine.NumberValue).Value != 1 {
		t.Fatalf("expected the Win case to be selected, got %v", commit.Snapshot.Root.Outcome.Result)
	}
}

func TestStep_SignalBindingFeedsGuardAndControl(t *testing.T) {
	workflow := engine.Workflow{
		Name:         "BindDemo",
		ResultType:   engine.NumberType{},
		InitialState: "S",
		States: []engine.WorkflowState{
			{
				Name: "S",
				Transitions: []engine.Transition{
					{
						Signal: engine.SignalPattern{
							Source:   engine.NamedSignalSource{Name: "Answer"},
							Bindings: []engine.SignalBinding{{Field: "value", Name: "v"}},
						},
						Guard:   engine.BinaryExpression{Operator: engine.BinaryOperatorGreater, Left: engine.ReferenceExpression{Name: "v"}, Right: engine.NumberLiteralExpression{Value: 0}},
						Control: engine.CompleteControl{Result: engine.ReferenceExpression{Name: "v"}},
					},
				},
			},
		},
	}
	p := engine.Program{RootWorkflow: "BindDemo", Workflows: map[string]engine.Workflow{"BindDemo": workflow}}
	snap := engine.Snapshot{
		GlobalState: engine.RecordValue{TypeName: "global"},
		Root:        engine.WorkflowInstance{Workflow: "BindDemo", State: "S", LocalState: engine.RecordValue{TypeName: "local"}},
	}

	commit, err := runtime.Step(p, snap, engine.Signal{Name: "Answer", Fields: map[string]engine.Value{"value": engine.NumberValue{Value: 7}}}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if commit.Snapshot.Root.Outcome.Result.(engine.NumberValue).Value != 7 {
		t.Fatalf("got %v, want 7", commit.Snapshot.Root.Outcome.Result)
	}

	_, err = runtime.Step(p, snap, engine.Signal{Name: "Answer", Fields: map[string]engine.Value{"value": engine.NumberValue{Value: -1}}}, engine.DefaultLimits())
	if err != runtime.ErrSignalRejected {
		t.Fatalf("expected guard to reject a non-positive bound value, got %v", err)
	}
}
