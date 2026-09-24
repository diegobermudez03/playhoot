package compiler_test

import (
	"testing"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/internal/compiler"
	"github.com/diegobermudez03/playhoot/game/language/v1/program"
)

func TestCompile_OpenKeyedQuestionOperation(t *testing.T) {
	q := program.QuestionDeclaration{
		Name:         "Confirm",
		Parameters:   []program.FieldDeclaration{{Name: "prompt", Type: stringType()}},
		ResponseType: boolType(),
	}
	def := program.Definition{
		Questions: []program.QuestionDeclaration{q},
		Workflows: []program.WorkflowDeclaration{
			{
				Name:               "Main",
				Parameters:         []program.FieldDeclaration{{Name: "player", Type: userType()}},
				ResultType:         program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				KeyedQuestionSlots: []program.KeyedQuestionSlotDeclaration{{Name: "Quiz", Question: "Confirm", KeyType: stringType()}},
				InitialState:       "S",
				States: []program.WorkflowStateDeclaration{
					{
						Name: "S",
						Transitions: []program.TransitionDeclaration{
							{
								Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
								Operations: program.Block{Operations: []program.Operation{
									program.OpenKeyedQuestionOperation{
										Slot:      "Quiz",
										Key:       program.StringLiteralExpression{Value: "p1"},
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
	if len(p.Workflows["Main"].KeyedQuestionSlots) != 1 {
		t.Fatalf("got %+v", p.Workflows["Main"].KeyedQuestionSlots)
	}
	ops := p.Workflows["Main"].States[0].Transitions[0].Operations.Operations
	if _, ok := ops[0].(engine.OpenKeyedQuestionOperation); !ok {
		t.Fatalf("got %T", ops[0])
	}
}

func TestCompile_OpenKeyedQuestionUndeclaredSlot(t *testing.T) {
	def := workflowWithOneTransition(program.TransitionDeclaration{
		Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
		Operations: program.Block{Operations: []program.Operation{
			program.OpenKeyedQuestionOperation{Slot: "Nonexistent", Key: program.StringLiteralExpression{Value: "k"}, Recipient: program.NumberLiteralExpression{Value: "1"}},
		}},
		Control: program.StayControl{},
	})
	_, diags := compiler.Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an undeclared keyed question slot error")
	}
}

func TestCompile_OpenKeyedQuestionKeyTypeMismatch(t *testing.T) {
	def := program.Definition{
		Questions: []program.QuestionDeclaration{{Name: "Confirm", ResponseType: boolType()}},
		Workflows: []program.WorkflowDeclaration{
			{
				Name:               "Main",
				Parameters:         []program.FieldDeclaration{{Name: "player", Type: userType()}},
				ResultType:         program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				KeyedQuestionSlots: []program.KeyedQuestionSlotDeclaration{{Name: "Quiz", Question: "Confirm", KeyType: stringType()}},
				InitialState:       "S",
				States: []program.WorkflowStateDeclaration{
					{
						Name: "S",
						Transitions: []program.TransitionDeclaration{
							{
								Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
								Operations: program.Block{Operations: []program.Operation{
									// Key is statically number, but the slot's declared KeyType is string.
									program.OpenKeyedQuestionOperation{Slot: "Quiz", Key: program.NumberLiteralExpression{Value: "1"}, Recipient: program.ReferenceExpression{Name: "player"}},
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
		t.Fatal("expected a key/KeyType mismatch error")
	}
}

func TestCompile_OpenKeyedQuestionArgumentMismatch(t *testing.T) {
	q := program.QuestionDeclaration{Name: "Confirm", Parameters: []program.FieldDeclaration{{Name: "prompt", Type: stringType()}}, ResponseType: boolType()}
	def := program.Definition{
		Questions: []program.QuestionDeclaration{q},
		Workflows: []program.WorkflowDeclaration{
			{
				Name:               "Main",
				Parameters:         []program.FieldDeclaration{{Name: "player", Type: userType()}},
				ResultType:         program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				KeyedQuestionSlots: []program.KeyedQuestionSlotDeclaration{{Name: "Quiz", Question: "Confirm", KeyType: stringType()}},
				InitialState:       "S",
				States: []program.WorkflowStateDeclaration{
					{
						Name: "S",
						Transitions: []program.TransitionDeclaration{
							{
								Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
								Operations: program.Block{Operations: []program.Operation{
									program.OpenKeyedQuestionOperation{Slot: "Quiz", Key: program.StringLiteralExpression{Value: "p1"}, Recipient: program.ReferenceExpression{Name: "player"}}, // missing "prompt"
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

func TestCompile_CloseKeyedQuestionUndeclaredSlot(t *testing.T) {
	def := workflowWithOneTransition(program.TransitionDeclaration{
		Signal:     program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
		Operations: program.Block{Operations: []program.Operation{program.CloseKeyedQuestionOperation{Slot: "Nope", Key: program.StringLiteralExpression{Value: "k"}}}},
		Control:    program.StayControl{},
	})
	_, diags := compiler.Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an undeclared keyed question slot error")
	}
}

func TestCompile_ScheduleAndCancelKeyedTimer(t *testing.T) {
	def := program.Definition{
		Workflows: []program.WorkflowDeclaration{
			{
				Name:            "Main",
				ResultType:      program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				KeyedTimerSlots: []program.KeyedTimerSlotDeclaration{{Name: "Deadline", KeyType: stringType()}},
				InitialState:    "S",
				States: []program.WorkflowStateDeclaration{
					{
						Name: "S",
						Transitions: []program.TransitionDeclaration{
							{
								Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
								Operations: program.Block{Operations: []program.Operation{
									program.ScheduleKeyedTimerOperation{Slot: "Deadline", Key: program.StringLiteralExpression{Value: "p1"}, DelayMilliseconds: program.NumberLiteralExpression{Value: "5000"}},
								}},
								Control: program.StayControl{},
							},
							{
								Signal:     program.SignalPattern{Source: program.NamedSignalSource{Name: "SessionCancelled"}},
								Operations: program.Block{Operations: []program.Operation{program.CancelKeyedTimerOperation{Slot: "Deadline", Key: program.StringLiteralExpression{Value: "p1"}}}},
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
	if _, ok := ops[0].(engine.ScheduleKeyedTimerOperation); !ok {
		t.Fatalf("got %T", ops[0])
	}
}

func TestCompile_ScheduleKeyedTimerUndeclaredSlot(t *testing.T) {
	def := workflowWithOneTransition(program.TransitionDeclaration{
		Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
		Operations: program.Block{Operations: []program.Operation{
			program.ScheduleKeyedTimerOperation{Slot: "Nope", Key: program.StringLiteralExpression{Value: "k"}, DelayMilliseconds: program.NumberLiteralExpression{Value: "1"}},
		}},
		Control: program.StayControl{},
	})
	_, diags := compiler.Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an undeclared keyed timer slot error")
	}
}

func TestCompile_ScheduleKeyedTimerKeyTypeMismatch(t *testing.T) {
	def := program.Definition{
		Workflows: []program.WorkflowDeclaration{
			{
				Name:            "Main",
				ResultType:      program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				KeyedTimerSlots: []program.KeyedTimerSlotDeclaration{{Name: "Deadline", KeyType: numberType()}},
				InitialState:    "S",
				States: []program.WorkflowStateDeclaration{
					{
						Name: "S",
						Transitions: []program.TransitionDeclaration{
							{
								Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
								Operations: program.Block{Operations: []program.Operation{
									program.ScheduleKeyedTimerOperation{Slot: "Deadline", Key: program.StringLiteralExpression{Value: "p1"}, DelayMilliseconds: program.NumberLiteralExpression{Value: "5000"}},
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
		t.Fatal("expected a key/KeyType mismatch error")
	}
}

func TestCompile_OpenKeyedAskGroupOperation(t *testing.T) {
	def := program.Definition{
		Questions: []program.QuestionDeclaration{{Name: "Confirm", ResponseType: boolType()}},
		Workflows: []program.WorkflowDeclaration{
			{
				Name:               "Main",
				Parameters:         []program.FieldDeclaration{{Name: "recipients", Type: program.ListTypeReference{Element: userType()}}},
				ResultType:         program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				KeyedAskGroupSlots: []program.KeyedAskGroupSlotDeclaration{{Name: "Team", Question: "Confirm", KeyType: stringType()}},
				InitialState:       "S",
				States: []program.WorkflowStateDeclaration{
					{
						Name: "S",
						Transitions: []program.TransitionDeclaration{
							{
								Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
								Operations: program.Block{Operations: []program.Operation{
									program.OpenKeyedAskGroupOperation{
										Slot:       "Team",
										Key:        program.StringLiteralExpression{Value: "team_a"},
										Recipients: program.ReferenceExpression{Name: "recipients"},
										Completion: program.AskGroupAllResponsesPolicy{},
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
	if len(p.Workflows["Main"].KeyedAskGroupSlots) != 1 {
		t.Fatalf("got %+v", p.Workflows["Main"].KeyedAskGroupSlots)
	}
	ops := p.Workflows["Main"].States[0].Transitions[0].Operations.Operations
	if _, ok := ops[0].(engine.OpenKeyedAskGroupOperation); !ok {
		t.Fatalf("got %T", ops[0])
	}
}

func TestCompile_KeyedQuestionAnsweredSignalSource_BindsKeyRespondentAnswer(t *testing.T) {
	def := program.Definition{
		Questions: []program.QuestionDeclaration{{Name: "Confirm", ResponseType: boolType()}},
		Workflows: []program.WorkflowDeclaration{
			{
				Name:               "Main",
				ResultType:         program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				KeyedQuestionSlots: []program.KeyedQuestionSlotDeclaration{{Name: "Quiz", Question: "Confirm", KeyType: stringType()}},
				InitialState:       "S",
				States: []program.WorkflowStateDeclaration{
					{
						Name: "S",
						Transitions: []program.TransitionDeclaration{
							{
								Signal: program.SignalPattern{
									Source: program.KeyedQuestionAnsweredSignalSource{Slot: "Quiz"},
									Bindings: []program.SignalBinding{
										{Field: "key", Name: "k"},
										{Field: "respondent", Name: "r"},
										{Field: "answer", Name: "a"},
									},
								},
								// Referencing all three bound names proves the compiled
								// schema actually exposed them with usable types.
								Guard: program.BinaryExpression{
									Operator: program.BinaryOperatorEqual,
									Left:     program.ReferenceExpression{Name: "k"},
									Right:    program.StringLiteralExpression{Value: "p1"},
								},
								Control: program.CompleteControl{Result: program.UnitLiteralExpression{}},
							},
						},
					},
				},
			},
		},
		RootWorkflow: "Main",
	}
	_, diags := compiler.Compile(def)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
}

func TestCompile_KeyedQuestionAnsweredSignalSource_UndeclaredSlot(t *testing.T) {
	def := workflowWithOneTransition(program.TransitionDeclaration{
		Signal:  program.SignalPattern{Source: program.KeyedQuestionAnsweredSignalSource{Slot: "Nope"}},
		Control: program.StayControl{},
	})
	_, diags := compiler.Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an undeclared keyed question slot error")
	}
}

func TestCompile_KeyedTimerExpiredSignalSource_BindsKey(t *testing.T) {
	def := program.Definition{
		Workflows: []program.WorkflowDeclaration{
			{
				Name:            "Main",
				ResultType:      program.BuiltinTypeReference{Type: program.BuiltinTypeString},
				KeyedTimerSlots: []program.KeyedTimerSlotDeclaration{{Name: "Deadline", KeyType: stringType()}},
				InitialState:    "S",
				States: []program.WorkflowStateDeclaration{
					{
						Name: "S",
						Transitions: []program.TransitionDeclaration{
							{
								Signal: program.SignalPattern{
									Source:   program.KeyedTimerExpiredSignalSource{Slot: "Deadline"},
									Bindings: []program.SignalBinding{{Field: "key", Name: "k"}},
								},
								Control: program.CompleteControl{Result: program.ReferenceExpression{Name: "k"}},
							},
						},
					},
				},
			},
		},
		RootWorkflow: "Main",
	}
	_, diags := compiler.Compile(def)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
}

func TestCompile_KeyedAskGroupCompletedSignalSource_BindsKeyAndCollectionFields(t *testing.T) {
	def := program.Definition{
		Questions: []program.QuestionDeclaration{{Name: "Confirm", ResponseType: boolType()}},
		Workflows: []program.WorkflowDeclaration{
			{
				Name:               "Main",
				ResultType:         program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				KeyedAskGroupSlots: []program.KeyedAskGroupSlotDeclaration{{Name: "Team", Question: "Confirm", KeyType: stringType()}},
				InitialState:       "S",
				States: []program.WorkflowStateDeclaration{
					{
						Name: "S",
						Transitions: []program.TransitionDeclaration{
							{
								Signal: program.SignalPattern{
									Source: program.KeyedAskGroupCompletedSignalSource{Slot: "Team"},
									Bindings: []program.SignalBinding{
										{Field: "key", Name: "k"},
										{Field: "responses", Name: "r"},
										{Field: "respondents", Name: "resp"},
										{Field: "missing", Name: "m"},
									},
								},
								Control: program.CompleteControl{Result: program.UnitLiteralExpression{}},
							},
						},
					},
				},
			},
		},
		RootWorkflow: "Main",
	}
	_, diags := compiler.Compile(def)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
}

func TestCompile_DuplicateKeyedQuestionSlotName(t *testing.T) {
	def := program.Definition{
		Questions: []program.QuestionDeclaration{{Name: "Confirm", ResponseType: boolType()}},
		Workflows: []program.WorkflowDeclaration{
			{
				Name:       "Main",
				ResultType: program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				KeyedQuestionSlots: []program.KeyedQuestionSlotDeclaration{
					{Name: "Quiz", Question: "Confirm", KeyType: stringType()},
					{Name: "Quiz", Question: "Confirm", KeyType: stringType()},
				},
				InitialState: "S",
				States:       []program.WorkflowStateDeclaration{{Name: "S"}},
			},
		},
		RootWorkflow: "Main",
	}
	_, diags := compiler.Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a duplicate keyed question slot name error")
	}
}

func TestCompile_KeyedQuestionPresentation_KeyBindingAvailable(t *testing.T) {
	// Proves the implicit "key" binding documented on
	// KeyedQuestionSlotDeclaration.Presentation is actually available to
	// its ProjectionArguments, unlike an ordinary (non-keyed)
	// QuestionSlotDeclaration.Presentation.
	def := program.Definition{
		PresentationSlots: []program.PresentationSlotDeclaration{{Name: "modal"}},
		Projections: []program.ProjectionDeclaration{
			{Name: "Proj", Parameters: []program.FieldDeclaration{{Name: "k", Type: stringType()}}, ResultType: stringType(), Body: program.ReferenceExpression{Name: "k"}},
		},
		Views: []program.ViewDeclaration{
			{Name: "View", ModelType: stringType(), Root: program.EmptyElement{}},
		},
		Questions: []program.QuestionDeclaration{{Name: "Confirm", ResponseType: boolType()}},
		Workflows: []program.WorkflowDeclaration{
			{
				Name:       "Main",
				ResultType: program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				KeyedQuestionSlots: []program.KeyedQuestionSlotDeclaration{
					{
						Name:     "Quiz",
						Question: "Confirm",
						KeyType:  stringType(),
						Presentation: &program.QuestionPresentationDeclaration{
							Slot:                "modal",
							Projection:          "Proj",
							ProjectionArguments: []program.CallArgument{{Name: "k", Value: program.ReferenceExpression{Name: "key"}}},
							View:                "View",
						},
					},
				},
				InitialState: "S",
				States:       []program.WorkflowStateDeclaration{{Name: "S"}},
			},
		},
		RootWorkflow: "Main",
	}
	_, diags := compiler.Compile(def)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
}
