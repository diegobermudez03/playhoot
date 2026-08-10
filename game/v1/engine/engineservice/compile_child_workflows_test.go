package engineservice

import (
	"testing"

	"github.com/diegobermudez03/playhoot/game/v1/engine"
	"github.com/diegobermudez03/playhoot/game/v1/program"
)

// childWorkflowDef builds a definition with a "Worker" workflow (one
// number parameter, number result) and a "Main" root workflow declaring
// a child slot "W" for it, whose one transition runs ops.
func childWorkflowDef(ops []program.Operation) program.Definition {
	return program.Definition{
		Workflows: []program.WorkflowDeclaration{
			{
				Name:         "Worker",
				Parameters:   []program.FieldDeclaration{{Name: "amount", Type: numberType()}},
				ResultType:   numberType(),
				InitialState: "S",
				States: []program.WorkflowStateDeclaration{
					{Name: "S", Transitions: []program.TransitionDeclaration{
						{Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}}, Control: program.CompleteControl{Result: program.ReferenceExpression{Name: "amount"}}},
					}},
				},
			},
			{
				Name:         "Main",
				ResultType:   program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				ChildSlots:   []program.ChildWorkflowSlotDeclaration{{Name: "W", Workflow: "Worker"}},
				InitialState: "S",
				States: []program.WorkflowStateDeclaration{
					{Name: "S", Transitions: []program.TransitionDeclaration{
						{
							Signal:     program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
							Operations: program.Block{Operations: ops},
							Control:    program.StayControl{},
						},
					}},
				},
			},
		},
		RootWorkflow: "Main",
	}
}

func TestCompile_SpawnChildWorkflowOperation(t *testing.T) {
	def := childWorkflowDef([]program.Operation{
		program.SpawnChildWorkflowOperation{Slot: "W", Arguments: []program.CallArgument{{Name: "amount", Value: program.NumberLiteralExpression{Value: "10"}}}},
	})
	p, diags := Compile(def)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	ops := p.Workflows["Main"].States[0].Transitions[0].Operations.Operations
	if _, ok := ops[0].(engine.SpawnChildWorkflowOperation); !ok {
		t.Fatalf("got %+v", ops)
	}
}

func TestCompile_SpawnChildWorkflowUndeclaredSlot(t *testing.T) {
	def := childWorkflowDef([]program.Operation{
		program.SpawnChildWorkflowOperation{Slot: "Nope"},
	})
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an undeclared child slot error")
	}
}

func TestCompile_SpawnChildWorkflowArgumentMismatch(t *testing.T) {
	def := childWorkflowDef([]program.Operation{
		program.SpawnChildWorkflowOperation{Slot: "W"}, // missing "amount"
	})
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a missing-argument error")
	}
}

func TestCompile_CancelChildWorkflowOperation(t *testing.T) {
	def := childWorkflowDef([]program.Operation{
		program.CancelChildWorkflowOperation{Slot: "W", Reason: program.StringLiteralExpression{Value: "done"}},
	})
	p, diags := Compile(def)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	ops := p.Workflows["Main"].States[0].Transitions[0].Operations.Operations
	if _, ok := ops[0].(engine.CancelChildWorkflowOperation); !ok {
		t.Fatalf("got %+v", ops)
	}
}

func TestCompile_CancelChildWorkflowUndeclaredSlot(t *testing.T) {
	def := childWorkflowDef([]program.Operation{
		program.CancelChildWorkflowOperation{Slot: "Nope", Reason: program.StringLiteralExpression{Value: "done"}},
	})
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an undeclared child slot error")
	}
}

func TestCompile_CancelChildWorkflowReasonMustBeString(t *testing.T) {
	def := childWorkflowDef([]program.Operation{
		program.CancelChildWorkflowOperation{Slot: "W", Reason: program.NumberLiteralExpression{Value: "1"}},
	})
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a reason-type error")
	}
}

func TestCompile_ChildOutcomeSignalSchemas(t *testing.T) {
	def := program.Definition{
		Workflows: []program.WorkflowDeclaration{
			{
				Name:         "Worker",
				ResultType:   numberType(),
				InitialState: "S",
				States:       []program.WorkflowStateDeclaration{{Name: "S", Transitions: []program.TransitionDeclaration{{Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}}, Control: program.CompleteControl{Result: program.NumberLiteralExpression{Value: "1"}}}}}},
			},
			{
				Name:         "Main",
				ResultType:   program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				ChildSlots:   []program.ChildWorkflowSlotDeclaration{{Name: "W", Workflow: "Worker"}},
				InitialState: "S",
				States: []program.WorkflowStateDeclaration{
					{Name: "S", Transitions: []program.TransitionDeclaration{
						{
							Signal:  program.SignalPattern{Source: program.ChildCompletedSignalSource{Slot: "W"}, Bindings: []program.SignalBinding{{Field: "result", Name: "r"}}},
							Control: program.CompleteControl{Result: program.UnitLiteralExpression{}},
						},
						{
							Signal:  program.SignalPattern{Source: program.ChildFailedSignalSource{Slot: "W"}, Bindings: []program.SignalBinding{{Field: "error", Name: "e"}}},
							Control: program.CompleteControl{Result: program.UnitLiteralExpression{}},
						},
						{
							Signal:  program.SignalPattern{Source: program.ChildCancelledSignalSource{Slot: "W"}, Bindings: []program.SignalBinding{{Field: "reason", Name: "c"}}},
							Control: program.CompleteControl{Result: program.UnitLiteralExpression{}},
						},
					}},
				},
			},
		},
		RootWorkflow: "Main",
	}
	_, diags := Compile(def)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
}

func TestCompile_ChildOutcomeSignalUndeclaredSlot(t *testing.T) {
	def := workflowWithOneTransition(program.TransitionDeclaration{
		Signal:  program.SignalPattern{Source: program.ChildCompletedSignalSource{Slot: "Nope"}},
		Control: program.StayControl{},
	})
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an undeclared child slot error")
	}
}
