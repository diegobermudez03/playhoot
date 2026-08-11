package compiler_test

import (
	"testing"

	"github.com/diegobermudez03/playhoot/game/v1/engine"
	"github.com/diegobermudez03/playhoot/game/v1/engine/internal/compiler"
	"github.com/diegobermudez03/playhoot/game/v1/program"
)

func TestCompile_CannotAssignToImmutableRoot(t *testing.T) {
	def := workflowWithOneTransition(program.TransitionDeclaration{
		Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
		Operations: program.Block{Operations: []program.Operation{
			program.SetOperation{
				Target: program.NameTarget{Name: "startAt"},
				Value:  program.NumberLiteralExpression{Value: "1"},
			},
		}},
		Control: program.StayControl{},
	})
	// give the workflow a parameter named "startAt" so the only reason
	// this fails is assignment authorization, not an undeclared name.
	def.Workflows[0].Parameters = []program.FieldDeclaration{{Name: "startAt", Type: numberType()}}

	_, diags := compiler.Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an error assigning to a lexical parameter")
	}
}

func TestCompile_DrawRandomOperation(t *testing.T) {
	def := workflowWithOneTransition(program.TransitionDeclaration{
		Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
		Operations: program.Block{Operations: []program.Operation{
			program.DrawRandomOperation{
				Name:      "roll",
				Generator: program.RandomIntegerGenerator{Minimum: program.NumberLiteralExpression{Value: "1"}, Maximum: program.NumberLiteralExpression{Value: "6"}},
			},
		}},
		Control: program.CompleteControl{Result: program.ReferenceExpression{Name: "roll"}},
	})
	def.Workflows[0].ResultType = numberType()

	p, diags := compiler.Compile(def)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	ops := p.Workflows["Main"].States[0].Transitions[0].Operations.Operations
	if len(ops) != 1 {
		t.Fatalf("got %+v", ops)
	}
	draw, ok := ops[0].(engine.DrawRandomOperation)
	if !ok {
		t.Fatalf("got %T", ops[0])
	}
	if _, ok := draw.Generator.(engine.RandomIntegerGenerator); !ok {
		t.Fatalf("got %T", draw.Generator)
	}
}

func TestCompile_DrawRandomElementInfersElementType(t *testing.T) {
	def := workflowWithOneTransition(program.TransitionDeclaration{
		Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
		Operations: program.Block{Operations: []program.Operation{
			program.DrawRandomOperation{
				Name: "picked",
				Generator: program.RandomElementGenerator{Collection: program.ListExpression{
					ElementType: program.BuiltinTypeReference{Type: program.BuiltinTypeString},
					Elements:    []program.Expression{program.StringLiteralExpression{Value: "a"}},
				}},
			},
		}},
		Control: program.CompleteControl{Result: program.ReferenceExpression{Name: "picked"}},
	})
	def.Workflows[0].ResultType = program.BuiltinTypeReference{Type: program.BuiltinTypeString}

	_, diags := compiler.Compile(def)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
}

func TestCompile_ListAndMapOperationsTypeChecked(t *testing.T) {
	def := program.Definition{
		Workflows: []program.WorkflowDeclaration{
			{
				Name:       "Main",
				ResultType: program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				LocalState: program.StateDeclaration{Fields: []program.StateFieldDeclaration{
					{Name: "items", Type: program.ListTypeReference{Element: numberType()}, Initializer: program.ListExpression{ElementType: numberType()}},
				}},
				InitialState: "S",
				States: []program.WorkflowStateDeclaration{
					{
						Name: "S",
						Transitions: []program.TransitionDeclaration{
							{
								Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
								Operations: program.Block{Operations: []program.Operation{
									program.ListAppendOperation{
										Target: program.FieldTarget{Target: program.NameTarget{Name: "local"}, Field: "items"},
										Value:  program.StringLiteralExpression{Value: "wrong type"},
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
	_, diags := compiler.Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a type-mismatch error appending a string to a list<number>")
	}
}
