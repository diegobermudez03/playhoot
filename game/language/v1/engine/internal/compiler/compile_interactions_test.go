package compiler_test

import (
	"testing"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/internal/compiler"
	"github.com/diegobermudez03/playhoot/game/language/v1/program"
)

func userType() program.TypeReference {
	return program.BuiltinTypeReference{Type: program.BuiltinTypeUser}
}

func TestCompile_OpenQuestionOperation(t *testing.T) {
	q := program.QuestionDeclaration{
		Name:         "Confirm",
		Parameters:   []program.FieldDeclaration{{Name: "prompt", Type: stringType()}},
		ResponseType: boolType(),
	}

	// The recipient must be statically user; a workflow parameter typed
	// user is the simplest way to provide one in scope.
	def := program.Definition{
		Questions: []program.QuestionDeclaration{q},
		Workflows: []program.WorkflowDeclaration{
			{
				Name:          "Main",
				Parameters:    []program.FieldDeclaration{{Name: "player", Type: userType()}},
				ResultType:    program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				QuestionSlots: []program.QuestionSlotDeclaration{{Name: "Ask", Question: "Confirm"}},
				InitialState:  "S",
				States: []program.WorkflowStateDeclaration{
					{
						Name: "S",
						Transitions: []program.TransitionDeclaration{
							{
								Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
								Operations: program.Block{Operations: []program.Operation{
									program.OpenQuestionOperation{
										Slot:      "Ask",
										Recipient: program.ReferenceExpression{Name: "player"},
										Arguments: []program.CallArgument{{Name: "prompt", Value: program.StringLiteralExpression{Value: "Ready?"}}},
									},
								}},
								Control: program.StayControl{},
							},
						},
					},
				},
			},
		},
		RootWorkflow: "Main",
	}

	p, diags := compiler.Compile(def)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	ops := p.Workflows["Main"].States[0].Transitions[0].Operations.Operations
	if len(ops) != 1 {
		t.Fatalf("got %+v", ops)
	}
	if _, ok := ops[0].(engine.OpenQuestionOperation); !ok {
		t.Fatalf("got %T", ops[0])
	}
}

func TestCompile_OpenQuestionUndeclaredSlot(t *testing.T) {
	def := workflowWithOneTransition(program.TransitionDeclaration{
		Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
		Operations: program.Block{Operations: []program.Operation{
			program.OpenQuestionOperation{Slot: "Nonexistent", Recipient: program.NumberLiteralExpression{Value: "1"}},
		}},
		Control: program.StayControl{},
	})
	_, diags := compiler.Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an undeclared question slot error")
	}
}

func TestCompile_OpenQuestionArgumentMismatch(t *testing.T) {
	q := program.QuestionDeclaration{Name: "Confirm", Parameters: []program.FieldDeclaration{{Name: "prompt", Type: stringType()}}, ResponseType: boolType()}
	def := program.Definition{
		Questions: []program.QuestionDeclaration{q},
		Workflows: []program.WorkflowDeclaration{
			{
				Name:          "Main",
				Parameters:    []program.FieldDeclaration{{Name: "player", Type: userType()}},
				ResultType:    program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				QuestionSlots: []program.QuestionSlotDeclaration{{Name: "Ask", Question: "Confirm"}},
				InitialState:  "S",
				States: []program.WorkflowStateDeclaration{
					{
						Name: "S",
						Transitions: []program.TransitionDeclaration{
							{
								Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
								Operations: program.Block{Operations: []program.Operation{
									program.OpenQuestionOperation{Slot: "Ask", Recipient: program.ReferenceExpression{Name: "player"}}, // missing "prompt" argument
								}},
								Control: program.StayControl{},
							},
						},
					},
				},
			},
		},
		RootWorkflow: "Main",
	}
	_, diags := compiler.Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a missing-argument error")
	}
}

func TestCompile_CloseQuestionUndeclaredSlot(t *testing.T) {
	def := workflowWithOneTransition(program.TransitionDeclaration{
		Signal:     program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
		Operations: program.Block{Operations: []program.Operation{program.CloseQuestionOperation{Slot: "Nope"}}},
		Control:    program.StayControl{},
	})
	_, diags := compiler.Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an undeclared question slot error")
	}
}

func TestCompile_ScheduleAndCancelTimer(t *testing.T) {
	def := program.Definition{
		Workflows: []program.WorkflowDeclaration{
			{
				Name:         "Main",
				ResultType:   program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				TimerSlots:   []program.TimerSlotDeclaration{{Name: "Deadline"}},
				InitialState: "S",
				States: []program.WorkflowStateDeclaration{
					{
						Name: "S",
						Transitions: []program.TransitionDeclaration{
							{
								Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
								Operations: program.Block{Operations: []program.Operation{
									program.ScheduleTimerOperation{Slot: "Deadline", DelayMilliseconds: program.NumberLiteralExpression{Value: "5000"}},
								}},
								Control: program.StayControl{},
							},
							{
								Signal:     program.SignalPattern{Source: program.NamedSignalSource{Name: "SessionCancelled"}},
								Operations: program.Block{Operations: []program.Operation{program.CancelTimerOperation{Slot: "Deadline"}}},
								Control:    program.CancelControl{Reason: program.StringLiteralExpression{Value: "cancelled"}},
							},
						},
					},
				},
			},
		},
		RootWorkflow: "Main",
	}
	p, diags := compiler.Compile(def)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	ops := p.Workflows["Main"].States[0].Transitions[0].Operations.Operations
	if _, ok := ops[0].(engine.ScheduleTimerOperation); !ok {
		t.Fatalf("got %T", ops[0])
	}
}

func TestCompile_ScheduleTimerUndeclaredSlot(t *testing.T) {
	def := workflowWithOneTransition(program.TransitionDeclaration{
		Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
		Operations: program.Block{Operations: []program.Operation{
			program.ScheduleTimerOperation{Slot: "Nope", DelayMilliseconds: program.NumberLiteralExpression{Value: "1"}},
		}},
		Control: program.StayControl{},
	})
	_, diags := compiler.Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an undeclared timer slot error")
	}
}

func TestCompile_EmitEffectOperation(t *testing.T) {
	def := program.Definition{
		Effects: []program.EffectDeclaration{
			{Name: "Confetti", Parameters: []program.FieldDeclaration{{Name: "amount", Type: numberType()}}},
		},
		Workflows: []program.WorkflowDeclaration{
			{
				Name:         "Main",
				Parameters:   []program.FieldDeclaration{{Name: "player", Type: userType()}},
				ResultType:   program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				InitialState: "S",
				States: []program.WorkflowStateDeclaration{
					{
						Name: "S",
						Transitions: []program.TransitionDeclaration{
							{
								Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
								Operations: program.Block{Operations: []program.Operation{
									program.EmitEffectOperation{
										Effect:     "Confetti",
										Recipients: program.ListExpression{ElementType: userType(), Elements: []program.Expression{program.ReferenceExpression{Name: "player"}}},
										Arguments:  []program.CallArgument{{Name: "amount", Value: program.NumberLiteralExpression{Value: "10"}}},
									},
								}},
								Control: program.StayControl{},
							},
						},
					},
				},
			},
		},
		RootWorkflow: "Main",
	}
	p, diags := compiler.Compile(def)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	ops := p.Workflows["Main"].States[0].Transitions[0].Operations.Operations
	if _, ok := ops[0].(engine.EmitEffectOperation); !ok {
		t.Fatalf("got %T", ops[0])
	}
}

func TestCompile_EmitEffectUndeclaredEffect(t *testing.T) {
	def := workflowWithOneTransition(program.TransitionDeclaration{
		Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
		Operations: program.Block{Operations: []program.Operation{
			program.EmitEffectOperation{Effect: "Nope", Recipients: program.ListExpression{ElementType: userType()}},
		}},
		Control: program.StayControl{},
	})
	_, diags := compiler.Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an undeclared effect error")
	}
}

func TestCompile_QuestionValidationScope(t *testing.T) {
	// Validation may reference resources, global, respondent, answer,
	// and the question's own parameters.
	def := program.Definition{
		GlobalState: program.StateDeclaration{Fields: []program.StateFieldDeclaration{
			{Name: "minAge", Type: numberType(), Initializer: program.NumberLiteralExpression{Value: "18"}},
		}},
		Questions: []program.QuestionDeclaration{
			{
				Name:         "AgeCheck",
				Parameters:   []program.FieldDeclaration{{Name: "bonus", Type: numberType()}},
				ResponseType: numberType(),
				Validation: program.BinaryExpression{
					Operator: program.BinaryOperatorGreaterOrEqual,
					Left:     program.BinaryExpression{Operator: program.BinaryOperatorAdd, Left: program.ReferenceExpression{Name: "answer"}, Right: program.ReferenceExpression{Name: "bonus"}},
					Right:    program.FieldExpression{Target: program.ReferenceExpression{Name: "global"}, Field: "minAge"},
				},
			},
		},
		RootWorkflow: "",
	}
	def = withMinimalRootWorkflow(def)

	p, diags := compiler.Compile(def)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	q, ok := p.Questions["AgeCheck"]
	if !ok || q.Validation == nil {
		t.Fatalf("got %+v", q)
	}
}

func TestCompile_QuestionValidationMustBeBool(t *testing.T) {
	def := program.Definition{
		Questions: []program.QuestionDeclaration{
			{Name: "Bad", ResponseType: numberType(), Validation: program.NumberLiteralExpression{Value: "1"}},
		},
	}
	def = withMinimalRootWorkflow(def)
	_, diags := compiler.Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a validation-type error")
	}
}

func TestCompile_DuplicateQuestionName(t *testing.T) {
	def := program.Definition{
		Questions: []program.QuestionDeclaration{
			{Name: "Q", ResponseType: boolType()},
			{Name: "Q", ResponseType: boolType()},
		},
	}
	def = withMinimalRootWorkflow(def)
	_, diags := compiler.Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a duplicate question name error")
	}
}

func TestCompile_DuplicateEffectName(t *testing.T) {
	def := program.Definition{
		Effects: []program.EffectDeclaration{
			{Name: "E"},
			{Name: "E"},
		},
	}
	def = withMinimalRootWorkflow(def)
	_, diags := compiler.Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a duplicate effect name error")
	}
}
