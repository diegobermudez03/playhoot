package compiler_test

import (
	"testing"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/internal/compiler"
	"github.com/diegobermudez03/playhoot/game/language/v1/program"
)

func TestCompile_MinimalWorkflow(t *testing.T) {
	def := program.Definition{
		Workflows: []program.WorkflowDeclaration{
			{
				Name:         "Main",
				ResultType:   program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				InitialState: "Start",
				States: []program.WorkflowStateDeclaration{
					{
						Name: "Start",
						Transitions: []program.TransitionDeclaration{
							{
								Name:   "OnStart",
								Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
								Control: program.CompleteControl{
									Result: program.UnitLiteralExpression{},
								},
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
	wf, ok := p.Workflows["Main"]
	if !ok {
		t.Fatal("Main not compiled")
	}
	if len(wf.States) != 1 || wf.States[0].Name != "Start" {
		t.Fatalf("got %+v", wf.States)
	}
	if len(wf.States[0].Transitions) != 1 {
		t.Fatal("expected one transition")
	}
	if _, ok := wf.States[0].Transitions[0].Signal.Source.(engine.NamedSignalSource); !ok {
		t.Fatalf("got %+v", wf.States[0].Transitions[0].Signal.Source)
	}
	if p.RootWorkflow != "Main" {
		t.Fatalf("got %q", p.RootWorkflow)
	}
}

func TestCompile_RootWorkflowMissing(t *testing.T) {
	_, diags := compiler.Compile(program.Definition{})
	if !diags.HasErrors() {
		t.Fatal("expected a missing root workflow error")
	}
}

func TestCompile_RootWorkflowUndeclared(t *testing.T) {
	def := withMinimalRootWorkflow(program.Definition{})
	def.RootWorkflow = "Nonexistent"
	_, diags := compiler.Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an undeclared root workflow error")
	}
}

func TestCompile_DuplicateWorkflowName(t *testing.T) {
	def := program.Definition{
		Workflows: []program.WorkflowDeclaration{
			{Name: "Main", ResultType: program.BuiltinTypeReference{Type: program.BuiltinTypeUnit}, InitialState: "S", States: []program.WorkflowStateDeclaration{{Name: "S"}}},
			{Name: "Main", ResultType: program.BuiltinTypeReference{Type: program.BuiltinTypeUnit}, InitialState: "S", States: []program.WorkflowStateDeclaration{{Name: "S"}}},
		},
		RootWorkflow: "Main",
	}
	_, diags := compiler.Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a duplicate workflow name error")
	}
}

func TestCompile_UnknownNamedSignal(t *testing.T) {
	def := workflowWithOneTransition(program.TransitionDeclaration{
		Signal:  program.SignalPattern{Source: program.NamedSignalSource{Name: "NotReal"}},
		Control: program.StayControl{},
	})
	_, diags := compiler.Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an unknown named signal error")
	}
}

func TestCompile_SignalBindingUnknownField(t *testing.T) {
	def := workflowWithOneTransition(program.TransitionDeclaration{
		Signal: program.SignalPattern{
			Source:   program.NamedSignalSource{Name: "WorkflowStarted"},
			Bindings: []program.SignalBinding{{Field: "nonexistent", Name: "x"}},
		},
		Control: program.StayControl{},
	})
	_, diags := compiler.Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an unknown signal field error")
	}
}

func TestCompile_GotoUnknownState(t *testing.T) {
	def := workflowWithOneTransition(program.TransitionDeclaration{
		Signal:  program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
		Control: program.GotoControl{State: "Nowhere"},
	})
	_, diags := compiler.Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a goto-unknown-state error")
	}
}

func TestCompile_GotoKnownState(t *testing.T) {
	def := program.Definition{
		Workflows: []program.WorkflowDeclaration{
			{
				Name:         "Main",
				ResultType:   program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				InitialState: "A",
				States: []program.WorkflowStateDeclaration{
					{Name: "A", Transitions: []program.TransitionDeclaration{
						{Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}}, Control: program.GotoControl{State: "B"}},
					}},
					{Name: "B"},
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

func TestCompile_CompleteControlResultTypeMismatch(t *testing.T) {
	def := workflowWithOneTransition(program.TransitionDeclaration{
		Signal:  program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
		Control: program.CompleteControl{Result: program.StringLiteralExpression{Value: "oops"}},
	})
	_, diags := compiler.Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a complete-control result type mismatch error (workflow declares unit)")
	}
}

func TestCompile_FailControlErrorMustBeString(t *testing.T) {
	def := workflowWithOneTransition(program.TransitionDeclaration{
		Signal:  program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
		Control: program.FailControl{Error: program.NumberLiteralExpression{Value: "1"}},
	})
	_, diags := compiler.Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a fail-control error-type mismatch error")
	}
}

func TestCompile_QuestionSlotUndeclaredQuestion(t *testing.T) {
	def := program.Definition{
		Workflows: []program.WorkflowDeclaration{
			{
				Name:         "Main",
				ResultType:   program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				InitialState: "S",
				QuestionSlots: []program.QuestionSlotDeclaration{
					{Name: "Ask", Question: "Nonexistent"},
				},
				States: []program.WorkflowStateDeclaration{{Name: "S"}},
			},
		},
		RootWorkflow: "Main",
	}
	_, diags := compiler.Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an undeclared question error")
	}
}

func TestCompile_QuestionAnsweredSignalSchema(t *testing.T) {
	def := program.Definition{
		Questions: []program.QuestionDeclaration{
			{Name: "Confirm", ResponseType: program.BuiltinTypeReference{Type: program.BuiltinTypeBool}},
		},
		Workflows: []program.WorkflowDeclaration{
			{
				Name:         "Main",
				ResultType:   program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				InitialState: "S",
				QuestionSlots: []program.QuestionSlotDeclaration{
					{Name: "Ask", Question: "Confirm"},
				},
				States: []program.WorkflowStateDeclaration{
					{
						Name: "S",
						Transitions: []program.TransitionDeclaration{
							{
								Signal: program.SignalPattern{
									Source:   program.QuestionAnsweredSignalSource{Slot: "Ask"},
									Bindings: []program.SignalBinding{{Field: "answer", Name: "confirmed"}, {Field: "respondent", Name: "who"}},
								},
								Guard:   program.ReferenceExpression{Name: "confirmed"},
								Control: program.CompleteControl{Result: program.UnitLiteralExpression{}},
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
	transition := p.Workflows["Main"].States[0].Transitions[0]
	if len(transition.Signal.Bindings) != 2 {
		t.Fatalf("got %+v", transition.Signal.Bindings)
	}
}

func TestCompile_ChildCompletedSignalResolvesOtherWorkflowResultType(t *testing.T) {
	def := program.Definition{
		Workflows: []program.WorkflowDeclaration{
			{
				Name:         "SubGame",
				ResultType:   program.BuiltinTypeReference{Type: program.BuiltinTypeNumber},
				InitialState: "S",
				States:       []program.WorkflowStateDeclaration{{Name: "S"}},
			},
			{
				Name:         "Main",
				ResultType:   program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				InitialState: "S",
				ChildSlots: []program.ChildWorkflowSlotDeclaration{
					{Name: "Sub", Workflow: "SubGame"},
				},
				States: []program.WorkflowStateDeclaration{
					{
						Name: "S",
						Transitions: []program.TransitionDeclaration{
							{
								Signal: program.SignalPattern{
									Source:   program.ChildCompletedSignalSource{Slot: "Sub"},
									Bindings: []program.SignalBinding{{Field: "result", Name: "score"}},
								},
								Guard:   program.BinaryExpression{Operator: program.BinaryOperatorGreaterOrEqual, Left: program.ReferenceExpression{Name: "score"}, Right: program.NumberLiteralExpression{Value: "0"}},
								Control: program.CompleteControl{Result: program.UnitLiteralExpression{}},
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
	binding := p.Workflows["Main"].States[0].Transitions[0].Signal.Bindings[0]
	if binding.Name != "score" {
		t.Fatalf("got %+v", binding)
	}
}

func TestCompile_ChildSlotUndeclaredWorkflow(t *testing.T) {
	def := program.Definition{
		Workflows: []program.WorkflowDeclaration{
			{
				Name:         "Main",
				ResultType:   program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				InitialState: "S",
				ChildSlots:   []program.ChildWorkflowSlotDeclaration{{Name: "Sub", Workflow: "Nonexistent"}},
				States:       []program.WorkflowStateDeclaration{{Name: "S"}},
			},
		},
		RootWorkflow: "Main",
	}
	_, diags := compiler.Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an undeclared child workflow error")
	}
}

func TestCompile_TaskGroupCompletedSignalSchema(t *testing.T) {
	def := program.Definition{
		Workflows: []program.WorkflowDeclaration{
			{
				Name:         "Task",
				ResultType:   program.BuiltinTypeReference{Type: program.BuiltinTypeBool},
				InitialState: "S",
				States:       []program.WorkflowStateDeclaration{{Name: "S"}},
			},
			{
				Name:         "Main",
				ResultType:   program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				InitialState: "S",
				TaskGroupSlots: []program.TaskGroupSlotDeclaration{
					{Name: "Tasks", Workflow: "Task", KeyType: program.BuiltinTypeReference{Type: program.BuiltinTypeString}},
				},
				States: []program.WorkflowStateDeclaration{
					{
						Name: "S",
						Transitions: []program.TransitionDeclaration{
							{
								Signal: program.SignalPattern{
									Source: program.TaskGroupCompletedSignalSource{Slot: "Tasks"},
									Bindings: []program.SignalBinding{
										{Field: "results", Name: "results"},
										{Field: "unfinished", Name: "unfinished"},
									},
								},
								// results is statically map<string, bool> and
								// unfinished list<string>: indexing results
								// with a string key and comparing to a bool
								// literal, and comparing an unfinished
								// element to a string literal, only
								// type-checks if the resolved schema types
								// are correct.
								Guard: program.BinaryExpression{
									Operator: program.BinaryOperatorAnd,
									Left: program.BinaryExpression{
										Operator: program.BinaryOperatorEqual,
										Left:     program.IndexExpression{Target: program.ReferenceExpression{Name: "results"}, Index: program.StringLiteralExpression{Value: "alice"}},
										Right:    program.BoolLiteralExpression{Value: true},
									},
									Right: program.ListAllExpression{
										Collection: program.ReferenceExpression{Name: "unfinished"},
										ItemName:   "key",
										Predicate: program.BinaryExpression{
											Operator: program.BinaryOperatorNotEqual,
											Left:     program.ReferenceExpression{Name: "key"},
											Right:    program.StringLiteralExpression{Value: ""},
										},
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
	p, diags := compiler.Compile(def)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	bindings := p.Workflows["Main"].States[0].Transitions[0].Signal.Bindings
	if len(bindings) != 2 {
		t.Fatalf("got %+v", bindings)
	}
}

func TestCompile_MatchControlWithUnionBinding(t *testing.T) {
	def := program.Definition{
		Types: []program.TypeDeclaration{
			program.UnionTypeDeclaration{Name: "Outcome", Variants: []program.UnionVariantDeclaration{
				{Name: "Won", Fields: []program.FieldDeclaration{{Name: "score", Type: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}}}},
				{Name: "Lost"},
			}},
		},
		Workflows: []program.WorkflowDeclaration{
			{
				Name:         "Main",
				ResultType:   program.BuiltinTypeReference{Type: program.BuiltinTypeNumber},
				InitialState: "S",
				States: []program.WorkflowStateDeclaration{
					{
						Name: "S",
						Transitions: []program.TransitionDeclaration{
							{
								Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
								Control: program.MatchControl{
									Value: program.UnionExpression{TypeName: "Outcome", VariantName: "Won", Fields: []program.FieldInitializer{
										{Name: "score", Value: program.NumberLiteralExpression{Value: "10"}},
									}},
									Cases: []program.MatchControlCase{
										{
											Pattern: program.UnionVariantMatchPattern{TypeName: "Outcome", VariantName: "Won", Bindings: []program.MatchFieldBinding{{Field: "score", Name: "s"}}},
											Control: program.CompleteControl{Result: program.ReferenceExpression{Name: "s"}},
										},
										{
											Pattern: program.UnionVariantMatchPattern{TypeName: "Outcome", VariantName: "Lost"},
											Control: program.CompleteControl{Result: program.NumberLiteralExpression{Value: "0"}},
										},
									},
								},
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

func TestCompile_PresentationTargetsMustBeListOfUser(t *testing.T) {
	def := program.Definition{
		PresentationSlots: []program.PresentationSlotDeclaration{{Name: "hud"}},
		Projections:       []program.ProjectionDeclaration{{Name: "P", ResultType: program.BuiltinTypeReference{Type: program.BuiltinTypeUnit}, Body: program.UnitLiteralExpression{}}},
		Views:             []program.ViewDeclaration{{Name: "V", ModelType: program.BuiltinTypeReference{Type: program.BuiltinTypeUnit}, Root: program.EmptyElement{}}},
		Workflows: []program.WorkflowDeclaration{
			{
				Name:         "Main",
				ResultType:   program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				InitialState: "S",
				Presentations: []program.PresentationDeclaration{
					{Name: "Hud", Slot: "hud", Targets: program.NumberLiteralExpression{Value: "1"}, Projection: "P", View: "V"},
				},
				States: []program.WorkflowStateDeclaration{{Name: "S"}},
			},
		},
		RootWorkflow: "Main",
	}
	_, diags := compiler.Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a presentation targets type error")
	}
}

func TestCompile_LocalStateInitializerSeesWorkflowParameter(t *testing.T) {
	def := program.Definition{
		Workflows: []program.WorkflowDeclaration{
			{
				Name:       "Main",
				Parameters: []program.FieldDeclaration{{Name: "startAt", Type: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}}},
				ResultType: program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				LocalState: program.StateDeclaration{Fields: []program.StateFieldDeclaration{
					{Name: "counter", Type: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}, Initializer: program.ReferenceExpression{Name: "startAt"}},
				}},
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

// workflowWithOneTransition builds a minimal one-state, one-transition
// workflow around t, for tests that only care about validating one
// transition's signal/guard/control.
func workflowWithOneTransition(t program.TransitionDeclaration) program.Definition {
	return program.Definition{
		Workflows: []program.WorkflowDeclaration{
			{
				Name:         "Main",
				ResultType:   program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				InitialState: "S",
				States:       []program.WorkflowStateDeclaration{{Name: "S", Transitions: []program.TransitionDeclaration{t}}},
			},
		},
		RootWorkflow: "Main",
	}
}
