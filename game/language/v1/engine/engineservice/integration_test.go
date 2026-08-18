package engineservice_test

import (
	"testing"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
	"github.com/diegobermudez03/playhoot/game/language/v1/program"
)

// TestIntegration_HeadlessGuessTheRollGame exercises the full
// engineservice.Compile -> engineservice.NewSnapshot -> engineservice.Step pipeline for a small but complete
// headless game: state (global attempts/rolled), rules (a guess must
// match the authoritative roll, capped at 3 attempts), turns (repeated
// "Guess" signals), and randomness (an authoritative dice roll drawn
// once at startup) — with no questions, timers, or child workflows.
func TestIntegration_HeadlessGuessTheRollGame(t *testing.T) {
	globalRolled := program.FieldExpression{Target: program.ReferenceExpression{Name: "global"}, Field: "rolled"}
	globalAttempts := program.FieldExpression{Target: program.ReferenceExpression{Name: "global"}, Field: "attempts"}

	def := program.Definition{
		GlobalState: program.StateDeclaration{
			Fields: []program.StateFieldDeclaration{
				{Name: "rolled", Type: numberType(), Initializer: program.NumberLiteralExpression{Value: "0"}},
				{Name: "attempts", Type: numberType(), Initializer: program.NumberLiteralExpression{Value: "0"}},
			},
		},
		UserIntents: []program.UserIntentDeclaration{
			{Name: "Guess", Parameters: []program.FieldDeclaration{{Name: "value", Type: numberType()}}},
		},
		Workflows: []program.WorkflowDeclaration{
			{
				Name:         "Game",
				ResultType:   program.BuiltinTypeReference{Type: program.BuiltinTypeBool},
				InitialState: "Start",
				States: []program.WorkflowStateDeclaration{
					{
						Name: "Start",
						Transitions: []program.TransitionDeclaration{
							{
								Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
								Operations: program.Block{Operations: []program.Operation{
									program.DrawRandomOperation{
										Name:      "roll",
										Generator: program.RandomIntegerGenerator{Minimum: program.NumberLiteralExpression{Value: "1"}, Maximum: program.NumberLiteralExpression{Value: "6"}},
									},
									program.SetOperation{
										Target: program.FieldTarget{Target: program.NameTarget{Name: "global"}, Field: "rolled"},
										Value:  program.ReferenceExpression{Name: "roll"},
									},
								}},
								Control: program.GotoControl{State: "Playing"},
							},
						},
					},
					{
						Name: "Playing",
						Transitions: []program.TransitionDeclaration{
							{
								Signal: program.SignalPattern{
									Source:   program.UserIntentSignalSource{Intent: "Guess"},
									Bindings: []program.SignalBinding{{Field: "value", Name: "guess"}},
								},
								Operations: program.Block{Operations: []program.Operation{
									program.SetOperation{
										Target: program.FieldTarget{Target: program.NameTarget{Name: "global"}, Field: "attempts"},
										Value:  program.BinaryExpression{Operator: program.BinaryOperatorAdd, Left: globalAttempts, Right: program.NumberLiteralExpression{Value: "1"}},
									},
								}},
								Control: program.ConditionalControl{
									Condition: program.BinaryExpression{Operator: program.BinaryOperatorEqual, Left: program.ReferenceExpression{Name: "guess"}, Right: globalRolled},
									Then:      program.CompleteControl{Result: program.BoolLiteralExpression{Value: true}},
									Else: program.ConditionalControl{
										Condition: program.BinaryExpression{Operator: program.BinaryOperatorGreaterOrEqual, Left: globalAttempts, Right: program.NumberLiteralExpression{Value: "3"}},
										Then:      program.CompleteControl{Result: program.BoolLiteralExpression{Value: false}},
										Else:      program.StayControl{},
									},
								},
							},
						},
					},
				},
			},
		},
		RootWorkflow: "Game",
	}

	p, diags := engineservice.Compile(def)
	if diags.HasErrors() {
		t.Fatalf("unexpected compile errors: %v", diags)
	}

	snap, startSignal, err := engineservice.NewSnapshot(p, engine.InitializationInput{Seed: 123})
	if err != nil {
		t.Fatalf("unexpected engineservice.NewSnapshot error: %v", err)
	}
	if snap.Root.State != "Start" {
		t.Fatalf("expected the root to start in 'Start', got %q", snap.Root.State)
	}

	// Apply the first lifecycle signal engineservice.NewSnapshot handed back — this is
	// the caller's job, not engineservice.NewSnapshot's; see LOGICAL_CONTRACT.md.
	commit, err := engineservice.Step(p, snap, startSignal, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error applying WorkflowStarted: %v", err)
	}
	snap = commit.Snapshot
	if snap.Root.State != "Playing" {
		t.Fatalf("expected to reach 'Playing' after startup, got %q", snap.Root.State)
	}
	rolledField, _ := snap.GlobalState.FieldByName("rolled")
	rolled := rolledField.Value.(engine.NumberValue).Value
	if rolled < 1 || rolled > 6 {
		t.Fatalf("authoritative roll out of range: %v", rolled)
	}

	// Guess wrong up to twice, then correctly — the game must still be
	// running (Outcome nil) after each wrong guess within the attempt
	// cap, and complete successfully once the correct guess lands.
	wrongGuess := rolled + 1
	if wrongGuess > 6 {
		wrongGuess = 1
	}
	for i := 0; i < 2; i++ {
		commit, err = engineservice.Step(p, snap, engine.Signal{Kind: engine.SignalKindIntent, Intent: "Guess", Fields: map[string]engine.Value{"value": engine.NumberValue{Value: wrongGuess}}}, engine.DefaultLimits())
		if err != nil {
			t.Fatalf("unexpected error on wrong guess %d: %v", i, err)
		}
		snap = commit.Snapshot
		if snap.Root.Outcome != nil {
			t.Fatalf("game ended early after %d wrong guesses: %+v", i+1, snap.Root.Outcome)
		}
	}

	commit, err = engineservice.Step(p, snap, engine.Signal{Kind: engine.SignalKindIntent, Intent: "Guess", Fields: map[string]engine.Value{"value": engine.NumberValue{Value: rolled}}}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error on correct guess: %v", err)
	}
	snap = commit.Snapshot
	if snap.Root.Outcome == nil || snap.Root.Outcome.Kind != engine.WorkflowOutcomeCompleted {
		t.Fatalf("expected the game to complete on a correct guess, got %+v", snap.Root.Outcome)
	}
	if !snap.Root.Outcome.Result.(engine.BoolValue).Value {
		t.Fatalf("expected a winning result, got %+v", snap.Root.Outcome.Result)
	}
	attemptsField, _ := snap.GlobalState.FieldByName("attempts")
	if attemptsField.Value.(engine.NumberValue).Value != 3 {
		t.Fatalf("expected 3 recorded attempts, got %v", attemptsField.Value)
	}

	// The instance is terminated: further signals are rejected.
	_, err = engineservice.Step(p, snap, engine.Signal{Kind: engine.SignalKindIntent, Intent: "Guess", Fields: map[string]engine.Value{"value": engine.NumberValue{Value: rolled}}}, engine.DefaultLimits())
	if err != engineservice.ErrSignalRejected {
		t.Fatalf("expected a terminated game to reject further signals, got %v", err)
	}
}
