package engineservice

import (
	"testing"

	"github.com/diegobermudez03/playhoot/game/v1/engine"
)

func TestStep_DrawRandomIntegerIsDeterministicAndInRange(t *testing.T) {
	workflow := engine.Workflow{
		Name:         "Dice",
		ResultType:   engine.NumberType{},
		InitialState: "S",
		States: []engine.WorkflowState{
			{
				Name: "S",
				Transitions: []engine.Transition{
					{
						Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Roll"}},
						Operations: engine.Block{Operations: []engine.Operation{
							engine.DrawRandomOperation{
								Name:      "roll",
								Generator: engine.RandomIntegerGenerator{Minimum: engine.NumberLiteralExpression{Value: 1}, Maximum: engine.NumberLiteralExpression{Value: 6}},
							},
						}},
						Control: engine.CompleteControl{Result: engine.ReferenceExpression{Name: "roll"}},
					},
				},
			},
		},
	}
	p := engine.Program{RootWorkflow: "Dice", Workflows: map[string]engine.Workflow{"Dice": workflow}}
	snap := engine.Snapshot{
		GlobalState: engine.RecordValue{TypeName: "global"},
		Root:        engine.WorkflowInstance{Workflow: "Dice", State: "S", LocalState: engine.RecordValue{TypeName: "local"}},
		Random:      engine.RandomState{State: 12345},
	}

	commit1, err := Step(p, snap, engine.Signal{Name: "Roll"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	roll1 := commit1.Snapshot.Root.Outcome.Result.(engine.NumberValue).Value
	if roll1 < 1 || roll1 > 6 {
		t.Fatalf("roll %v out of range [1,6]", roll1)
	}

	// Same input snapshot + signal + limits must reproduce the same roll.
	commit2, err := Step(p, snap, engine.Signal{Name: "Roll"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	roll2 := commit2.Snapshot.Root.Outcome.Result.(engine.NumberValue).Value
	if roll1 != roll2 {
		t.Fatalf("draws are not deterministic: %v vs %v", roll1, roll2)
	}

	// The committed RandomState must have advanced from the original.
	if commit1.Snapshot.Random.State == snap.Random.State {
		t.Fatal("random state did not advance after a draw")
	}
}

func TestStep_DrawRandomElement(t *testing.T) {
	workflow := engine.Workflow{
		Name:         "Pick",
		ResultType:   engine.StringType{},
		InitialState: "S",
		States: []engine.WorkflowState{
			{
				Name: "S",
				Transitions: []engine.Transition{
					{
						Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Go"}},
						Operations: engine.Block{Operations: []engine.Operation{
							engine.DrawRandomOperation{
								Name: "picked",
								Generator: engine.RandomElementGenerator{Collection: engine.ListExpression{
									ElementType: engine.StringType{},
									Elements: []engine.Expression{
										engine.StringLiteralExpression{Value: "a"},
										engine.StringLiteralExpression{Value: "b"},
										engine.StringLiteralExpression{Value: "c"},
									},
								}},
							},
						}},
						Control: engine.CompleteControl{Result: engine.ReferenceExpression{Name: "picked"}},
					},
				},
			},
		},
	}
	p := engine.Program{RootWorkflow: "Pick", Workflows: map[string]engine.Workflow{"Pick": workflow}}
	snap := engine.Snapshot{
		GlobalState: engine.RecordValue{TypeName: "global"},
		Root:        engine.WorkflowInstance{Workflow: "Pick", State: "S", LocalState: engine.RecordValue{TypeName: "local"}},
		Random:      engine.RandomState{State: 999},
	}

	commit, err := Step(p, snap, engine.Signal{Name: "Go"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	picked := commit.Snapshot.Root.Outcome.Result.(engine.StringValue).Value
	if picked != "a" && picked != "b" && picked != "c" {
		t.Fatalf("got %q, want one of a/b/c", picked)
	}
}

func TestStep_DrawRandomEmptyCollectionErrors(t *testing.T) {
	workflow := engine.Workflow{
		Name:         "Pick",
		ResultType:   engine.UnitType{},
		InitialState: "S",
		States: []engine.WorkflowState{
			{
				Name: "S",
				Transitions: []engine.Transition{
					{
						Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Go"}},
						Operations: engine.Block{Operations: []engine.Operation{
							engine.DrawRandomOperation{
								Name:      "picked",
								Generator: engine.RandomElementGenerator{Collection: engine.ListExpression{ElementType: engine.StringType{}}},
							},
						}},
						Control: engine.CompleteControl{Result: engine.UnitLiteralExpression{}},
					},
				},
			},
		},
	}
	p := engine.Program{RootWorkflow: "Pick", Workflows: map[string]engine.Workflow{"Pick": workflow}}
	snap := engine.Snapshot{
		Root: engine.WorkflowInstance{Workflow: "Pick", State: "S", LocalState: engine.RecordValue{TypeName: "local"}},
	}

	_, err := Step(p, snap, engine.Signal{Name: "Go"}, engine.DefaultLimits())
	execErr, ok := err.(*ExecutionError)
	if !ok || execErr.Code != ExecutionErrorEmptyRandomCollection {
		t.Fatalf("expected ExecutionErrorEmptyRandomCollection, got %v", err)
	}
}

func TestStep_ShuffleProducesAPermutation(t *testing.T) {
	workflow := engine.Workflow{
		Name:         "Shuffle",
		ResultType:   engine.ListType{Element: engine.NumberType{}},
		InitialState: "S",
		States: []engine.WorkflowState{
			{
				Name: "S",
				Transitions: []engine.Transition{
					{
						Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Go"}},
						Operations: engine.Block{Operations: []engine.Operation{
							engine.DrawRandomOperation{
								Name: "shuffled",
								Generator: engine.RandomShuffleGenerator{Collection: engine.ListExpression{
									ElementType: engine.NumberType{},
									Elements: []engine.Expression{
										engine.NumberLiteralExpression{Value: 1}, engine.NumberLiteralExpression{Value: 2},
										engine.NumberLiteralExpression{Value: 3}, engine.NumberLiteralExpression{Value: 4},
									},
								}},
							},
						}},
						Control: engine.CompleteControl{Result: engine.ReferenceExpression{Name: "shuffled"}},
					},
				},
			},
		},
	}
	p := engine.Program{RootWorkflow: "Shuffle", Workflows: map[string]engine.Workflow{"Shuffle": workflow}}
	snap := engine.Snapshot{
		Root:   engine.WorkflowInstance{Workflow: "Shuffle", State: "S", LocalState: engine.RecordValue{TypeName: "local"}},
		Random: engine.RandomState{State: 7},
	}

	commit, err := Step(p, snap, engine.Signal{Name: "Go"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	elements := commit.Snapshot.Root.Outcome.Result.(engine.ListValue).Elements
	seen := map[float64]bool{}
	for _, e := range elements {
		seen[e.(engine.NumberValue).Value] = true
	}
	if len(seen) != 4 {
		t.Fatalf("expected a permutation of 4 distinct elements, got %v", elements)
	}
}

func TestStep_RandomStateRollsBackWhenStepFails(t *testing.T) {
	// Draw a random value, then unconditionally fail the invariant check
	// that follows. The candidate RandomState must not be committed.
	workflow := engine.Workflow{
		Name:         "RollbackDemo",
		ResultType:   engine.UnitType{},
		InitialState: "S",
		States: []engine.WorkflowState{
			{
				Name: "S",
				Transitions: []engine.Transition{
					{
						Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Go"}},
						Operations: engine.Block{Operations: []engine.Operation{
							engine.DrawRandomOperation{
								Name:      "roll",
								Generator: engine.RandomIntegerGenerator{Minimum: engine.NumberLiteralExpression{Value: 1}, Maximum: engine.NumberLiteralExpression{Value: 6}},
							},
						}},
						Control: engine.CompleteControl{Result: engine.UnitLiteralExpression{}},
					},
				},
			},
		},
	}
	p := engine.Program{
		RootWorkflow: "RollbackDemo",
		Workflows:    map[string]engine.Workflow{"RollbackDemo": workflow},
		Invariants:   []engine.Invariant{{Name: "AlwaysFalse", Condition: engine.BoolLiteralExpression{Value: false}}},
	}
	snap := engine.Snapshot{
		GlobalState: engine.RecordValue{TypeName: "global"},
		Root:        engine.WorkflowInstance{Workflow: "RollbackDemo", State: "S", LocalState: engine.RecordValue{TypeName: "local"}},
		Random:      engine.RandomState{State: 42},
	}

	_, err := Step(p, snap, engine.Signal{Name: "Go"}, engine.DefaultLimits())
	execErr, ok := err.(*ExecutionError)
	if !ok || execErr.Code != ExecutionErrorInvariantViolation {
		t.Fatalf("expected ExecutionErrorInvariantViolation, got %v", err)
	}
	if snap.Random.State != 42 {
		t.Fatalf("original snapshot's random state was mutated: %+v", snap.Random)
	}
}

func TestStep_ExecutionBudgetExceeded(t *testing.T) {
	// A ForEach over a collection within the loop-iteration limit, but
	// whose body contains enough operations to blow the overall
	// operation budget.
	body := make([]engine.Operation, 0, 50)
	for i := 0; i < 50; i++ {
		body = append(body, engine.LetOperation{Name: "x", Value: engine.NumberLiteralExpression{Value: 1}})
	}
	workflow := engine.Workflow{
		Name:         "Budget",
		ResultType:   engine.UnitType{},
		InitialState: "S",
		States: []engine.WorkflowState{
			{
				Name: "S",
				Transitions: []engine.Transition{
					{
						Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Go"}},
						Operations: engine.Block{Operations: []engine.Operation{
							engine.ForEachOperation{
								Collection: engine.ListExpression{ElementType: engine.NumberType{}, Elements: []engine.Expression{
									engine.NumberLiteralExpression{Value: 1}, engine.NumberLiteralExpression{Value: 2}, engine.NumberLiteralExpression{Value: 3},
								}},
								ItemName: "n",
								Body:     engine.Block{Operations: body},
							},
						}},
						Control: engine.CompleteControl{Result: engine.UnitLiteralExpression{}},
					},
				},
			},
		},
	}
	p := engine.Program{RootWorkflow: "Budget", Workflows: map[string]engine.Workflow{"Budget": workflow}}
	snap := engine.Snapshot{
		Root: engine.WorkflowInstance{Workflow: "Budget", State: "S", LocalState: engine.RecordValue{TypeName: "local"}},
	}

	// 3 iterations * 50 lets = 150 ops, plus the ForEach itself; a budget
	// of 100 must trip.
	_, err := Step(p, snap, engine.Signal{Name: "Go"}, engine.Limits{MaxOperations: 100, MaxLoopIterations: 10_000})
	execErr, ok := err.(*ExecutionError)
	if !ok || execErr.Code != ExecutionErrorBudgetExceeded {
		t.Fatalf("expected ExecutionErrorBudgetExceeded, got %v", err)
	}
}

func TestStep_LoopLimitExceeded(t *testing.T) {
	elements := make([]engine.Expression, 20)
	for i := range elements {
		elements[i] = engine.NumberLiteralExpression{Value: 1}
	}
	workflow := engine.Workflow{
		Name:         "LoopLimit",
		ResultType:   engine.UnitType{},
		InitialState: "S",
		States: []engine.WorkflowState{
			{
				Name: "S",
				Transitions: []engine.Transition{
					{
						Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Go"}},
						Operations: engine.Block{Operations: []engine.Operation{
							engine.ForEachOperation{
								Collection: engine.ListExpression{ElementType: engine.NumberType{}, Elements: elements},
								ItemName:   "n",
								Body:       engine.Block{},
							},
						}},
						Control: engine.CompleteControl{Result: engine.UnitLiteralExpression{}},
					},
				},
			},
		},
	}
	p := engine.Program{RootWorkflow: "LoopLimit", Workflows: map[string]engine.Workflow{"LoopLimit": workflow}}
	snap := engine.Snapshot{
		Root: engine.WorkflowInstance{Workflow: "LoopLimit", State: "S", LocalState: engine.RecordValue{TypeName: "local"}},
	}

	_, err := Step(p, snap, engine.Signal{Name: "Go"}, engine.Limits{MaxOperations: 10_000, MaxLoopIterations: 10})
	execErr, ok := err.(*ExecutionError)
	if !ok || execErr.Code != ExecutionErrorLoopLimitExceeded {
		t.Fatalf("expected ExecutionErrorLoopLimitExceeded, got %v", err)
	}
}
