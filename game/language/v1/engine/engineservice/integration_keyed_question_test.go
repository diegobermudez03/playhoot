package engineservice_test

import (
	"testing"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/internal/runtime"
	"github.com/diegobermudez03/playhoot/game/language/v1/program"
)

// TestIntegration_AsynchronousQuizWithKeyedQuestionSlot exercises the
// full engineservice.Compile -> NewSnapshot -> Step pipeline for an
// asynchronous, self-paced quiz: two players each progress through
// their own independent question at their
// own pace (opened and answered in either order), and the game only
// completes once both have answered — proving the keyed family's whole
// author-facing-JSON-shape-through-execution pipeline composes
// correctly, not just each layer in isolation.
func TestIntegration_AsynchronousQuizWithKeyedQuestionSlot(t *testing.T) {
	answeredA := program.FieldExpression{Target: program.ReferenceExpression{Name: "global"}, Field: "answeredA"}
	answeredB := program.FieldExpression{Target: program.ReferenceExpression{Name: "global"}, Field: "answeredB"}

	def := program.Definition{
		GlobalState: program.StateDeclaration{
			Fields: []program.StateFieldDeclaration{
				{Name: "answeredA", Type: boolType(), Initializer: program.BoolLiteralExpression{Value: false}},
				{Name: "answeredB", Type: boolType(), Initializer: program.BoolLiteralExpression{Value: false}},
			},
		},
		Questions: []program.QuestionDeclaration{
			{Name: "Guess", ResponseType: numberType()},
		},
		UserIntents: []program.UserIntentDeclaration{
			{Name: "Ask", Parameters: []program.FieldDeclaration{{Name: "key", Type: stringType()}, {Name: "recipient", Type: userType()}}},
		},
		Workflows: []program.WorkflowDeclaration{
			{
				Name:               "Quiz",
				ResultType:         program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				KeyedQuestionSlots: []program.KeyedQuestionSlotDeclaration{{Name: "Q", Question: "Guess", KeyType: stringType()}},
				InitialState:       "Playing",
				States: []program.WorkflowStateDeclaration{
					{
						Name: "Playing",
						Transitions: []program.TransitionDeclaration{
							{
								Signal:  program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
								Control: program.StayControl{},
							},
							{
								Signal: program.SignalPattern{
									Source:   program.UserIntentSignalSource{Intent: "Ask"},
									Bindings: []program.SignalBinding{{Field: "key", Name: "key"}, {Field: "recipient", Name: "recipient"}},
								},
								Operations: program.Block{Operations: []program.Operation{
									program.OpenKeyedQuestionOperation{Slot: "Q", Key: program.ReferenceExpression{Name: "key"}, Recipient: program.ReferenceExpression{Name: "recipient"}},
								}},
								Control: program.StayControl{},
							},
							{
								Signal: program.SignalPattern{
									Source:   program.KeyedQuestionAnsweredSignalSource{Slot: "Q"},
									Bindings: []program.SignalBinding{{Field: "key", Name: "k"}},
								},
								Operations: program.Block{Operations: []program.Operation{
									program.IfOperation{
										Condition: program.BinaryExpression{Operator: program.BinaryOperatorEqual, Left: program.ReferenceExpression{Name: "k"}, Right: program.StringLiteralExpression{Value: "playerA"}},
										Then: program.Block{Operations: []program.Operation{
											program.SetOperation{Target: program.FieldTarget{Target: program.NameTarget{Name: "global"}, Field: "answeredA"}, Value: program.BoolLiteralExpression{Value: true}},
										}},
										Else: program.Block{Operations: []program.Operation{
											program.SetOperation{Target: program.FieldTarget{Target: program.NameTarget{Name: "global"}, Field: "answeredB"}, Value: program.BoolLiteralExpression{Value: true}},
										}},
									},
								}},
								// Control here reads global state as it stood before
								// this transition's own Operations ran (unrelated to
								// keyed slots specifically - the engine's current
								// implementation does not yet give Control the
								// working state Operations already mutated, though
								// WorkflowControl's own contract documents that it
								// should), so completion is decided from whether the
								// *other* player already answered in an earlier,
								// already-committed transition - never from the flag
								// this same transition's own Operations just set.
								Control: program.ConditionalControl{
									Condition: program.BinaryExpression{
										Operator: program.BinaryOperatorOr,
										Left: program.BinaryExpression{
											Operator: program.BinaryOperatorAnd,
											Left:     program.BinaryExpression{Operator: program.BinaryOperatorEqual, Left: program.ReferenceExpression{Name: "k"}, Right: program.StringLiteralExpression{Value: "playerA"}},
											Right:    answeredB,
										},
										Right: program.BinaryExpression{
											Operator: program.BinaryOperatorAnd,
											Left:     program.BinaryExpression{Operator: program.BinaryOperatorEqual, Left: program.ReferenceExpression{Name: "k"}, Right: program.StringLiteralExpression{Value: "playerB"}},
											Right:    answeredA,
										},
									},
									Then: program.CompleteControl{Result: program.UnitLiteralExpression{}},
									Else: program.StayControl{},
								},
							},
						},
					},
				},
			},
		},
		RootWorkflow: "Quiz",
	}

	p, diags := engineservice.Compile(def)
	if diags.HasErrors() {
		t.Fatalf("unexpected compile errors: %v", diags)
	}

	snap, startSignal, err := runtime.NewSnapshot(p, engine.InitializationInput{Seed: 1})
	if err != nil {
		t.Fatalf("unexpected NewSnapshot error: %v", err)
	}
	commit, err := runtime.Step(p, snap, startSignal, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error applying WorkflowStarted: %v", err)
	}
	snap = commit.Snapshot

	const playerA = engine.UserID("player-a")
	const playerB = engine.UserID("player-b")

	ask := func(key string, recipient engine.UserID) {
		commit, err = runtime.Step(p, snap, engine.Signal{
			Kind: engine.SignalKindIntent, Intent: "Ask",
			Fields: map[string]engine.Value{"key": engine.StringValue{Value: key}, "recipient": engine.UserValue{ID: recipient}},
		}, engine.DefaultLimits())
		if err != nil {
			t.Fatalf("unexpected error opening key %q: %v", key, err)
		}
		snap = commit.Snapshot
	}
	interactionID := func(key string) engine.InteractionID {
		for _, s := range snap.Root.KeyedQuestionSlots {
			if s.Name != "Q" {
				continue
			}
			for _, p := range s.Pending {
				if p.Key.(engine.StringValue).Value == key {
					return p.InteractionID
				}
			}
		}
		return 0
	}
	answer := func(key string, respondent engine.UserID, value float64) {
		commit, err = runtime.Step(p, snap, engine.Signal{
			Kind: engine.SignalKindInteractionAnswered, InteractionID: interactionID(key), Respondent: respondent, Answer: engine.NumberValue{Value: value},
		}, engine.DefaultLimits())
		if err != nil {
			t.Fatalf("unexpected error answering key %q: %v", key, err)
		}
		snap = commit.Snapshot
	}

	// Both players' questions are opened up front — this is the whole
	// point of a keyed slot: they coexist simultaneously, independent
	// of each other, rather than contending for one shared slot.
	ask("playerA", playerA)
	ask("playerB", playerB)
	if len(commit.Outputs) != 1 {
		t.Fatalf("expected exactly one OpenKeyedQuestionOutput per Ask, got %+v", commit.Outputs)
	}
	if _, ok := commit.Outputs[0].(engine.OpenKeyedQuestionOutput); !ok {
		t.Fatalf("got %T", commit.Outputs[0])
	}

	// Player B answers first — out of order relative to when the
	// questions were opened — and the game must still be running.
	answer("playerB", playerB, 7)
	if snap.Root.Outcome != nil {
		t.Fatalf("expected the quiz to still be running after only one of two answers, got %+v", snap.Root.Outcome)
	}
	field, _ := snap.GlobalState.FieldByName("answeredB")
	if !field.Value.(engine.BoolValue).Value {
		t.Fatal("expected answeredB to be recorded")
	}

	// Player A can still independently answer their own still-pending
	// question, and only now does the quiz complete.
	answer("playerA", playerA, 3)
	if snap.Root.Outcome == nil || snap.Root.Outcome.Kind != engine.WorkflowOutcomeCompleted {
		t.Fatalf("expected the quiz to complete once both players answered, got %+v", snap.Root.Outcome)
	}
}
