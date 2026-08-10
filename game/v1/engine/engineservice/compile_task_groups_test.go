package engineservice

import (
	"testing"

	"github.com/diegobermudez03/playhoot/game/v1/engine"
	"github.com/diegobermudez03/playhoot/game/v1/program"
)

// taskGroupDef builds a definition with a "Worker" workflow (one number
// parameter, number result) and a "Main" root workflow declaring a
// number-keyed task-group slot "Workers" for it, whose one transition
// runs ops.
func taskGroupDef(ops []program.Operation) program.Definition {
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
				Name:           "Main",
				ResultType:     program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				TaskGroupSlots: []program.TaskGroupSlotDeclaration{{Name: "Workers", Workflow: "Worker", KeyType: numberType()}},
				InitialState:   "S",
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

func TestCompile_BeginTaskGroupOperation(t *testing.T) {
	def := taskGroupDef([]program.Operation{
		program.BeginTaskGroupOperation{Slot: "Workers", Completion: program.TaskGroupAllTerminalPolicy{}},
		program.SealTaskGroupOperation{Slot: "Workers"},
	})
	p, diags := Compile(def)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	ops := p.Workflows["Main"].States[0].Transitions[0].Operations.Operations
	if _, ok := ops[0].(engine.BeginTaskGroupOperation); !ok {
		t.Fatalf("got %+v", ops)
	}
}

func TestCompile_BeginTaskGroupUndeclaredSlot(t *testing.T) {
	def := taskGroupDef([]program.Operation{
		program.BeginTaskGroupOperation{Slot: "Nope", Completion: program.TaskGroupAllTerminalPolicy{}},
	})
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an undeclared task-group slot error")
	}
}

func TestCompile_SpawnTaskGroupChildOperation(t *testing.T) {
	def := taskGroupDef([]program.Operation{
		program.BeginTaskGroupOperation{Slot: "Workers", Completion: program.TaskGroupAllTerminalPolicy{}},
		program.SpawnTaskGroupChildOperation{
			Slot:      "Workers",
			Key:       program.NumberLiteralExpression{Value: "1"},
			Arguments: []program.CallArgument{{Name: "amount", Value: program.NumberLiteralExpression{Value: "10"}}},
		},
		program.SealTaskGroupOperation{Slot: "Workers"},
	})
	p, diags := Compile(def)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	ops := p.Workflows["Main"].States[0].Transitions[0].Operations.Operations
	if _, ok := ops[1].(engine.SpawnTaskGroupChildOperation); !ok {
		t.Fatalf("got %+v", ops)
	}
}

func TestCompile_SpawnTaskGroupChildUndeclaredSlot(t *testing.T) {
	def := taskGroupDef([]program.Operation{
		program.SpawnTaskGroupChildOperation{Slot: "Nope", Key: program.NumberLiteralExpression{Value: "1"}},
	})
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an undeclared task-group slot error")
	}
}

func TestCompile_SpawnTaskGroupChildKeyTypeMismatch(t *testing.T) {
	def := taskGroupDef([]program.Operation{
		program.BeginTaskGroupOperation{Slot: "Workers", Completion: program.TaskGroupAllTerminalPolicy{}},
		program.SpawnTaskGroupChildOperation{
			Slot:      "Workers",
			Key:       program.StringLiteralExpression{Value: "one"}, // slot's KeyType is number
			Arguments: []program.CallArgument{{Name: "amount", Value: program.NumberLiteralExpression{Value: "10"}}},
		},
	})
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a key-type mismatch error")
	}
}

func TestCompile_SpawnTaskGroupChildArgumentMismatch(t *testing.T) {
	def := taskGroupDef([]program.Operation{
		program.BeginTaskGroupOperation{Slot: "Workers", Completion: program.TaskGroupAllTerminalPolicy{}},
		program.SpawnTaskGroupChildOperation{Slot: "Workers", Key: program.NumberLiteralExpression{Value: "1"}}, // missing "amount"
	})
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a missing-argument error")
	}
}

func TestCompile_SealTaskGroupOperation(t *testing.T) {
	def := taskGroupDef([]program.Operation{
		program.BeginTaskGroupOperation{Slot: "Workers", Completion: program.TaskGroupAllTerminalPolicy{}},
		program.SealTaskGroupOperation{Slot: "Workers"},
	})
	p, diags := Compile(def)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	ops := p.Workflows["Main"].States[0].Transitions[0].Operations.Operations
	if _, ok := ops[1].(engine.SealTaskGroupOperation); !ok {
		t.Fatalf("got %+v", ops)
	}
}

func TestCompile_SealTaskGroupUndeclaredSlot(t *testing.T) {
	def := taskGroupDef([]program.Operation{program.SealTaskGroupOperation{Slot: "Nope"}})
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an undeclared task-group slot error")
	}
}

func TestCompile_FinalizeTaskGroupOperation(t *testing.T) {
	def := taskGroupDef([]program.Operation{program.FinalizeTaskGroupOperation{Slot: "Workers"}})
	p, diags := Compile(def)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	ops := p.Workflows["Main"].States[0].Transitions[0].Operations.Operations
	if _, ok := ops[0].(engine.FinalizeTaskGroupOperation); !ok {
		t.Fatalf("got %+v", ops)
	}
}

func TestCompile_FinalizeTaskGroupUndeclaredSlot(t *testing.T) {
	def := taskGroupDef([]program.Operation{program.FinalizeTaskGroupOperation{Slot: "Nope"}})
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an undeclared task-group slot error")
	}
}

func TestCompile_CancelTaskGroupOperation(t *testing.T) {
	def := taskGroupDef([]program.Operation{program.CancelTaskGroupOperation{Slot: "Workers", Reason: program.StringLiteralExpression{Value: "done"}}})
	p, diags := Compile(def)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	ops := p.Workflows["Main"].States[0].Transitions[0].Operations.Operations
	if _, ok := ops[0].(engine.CancelTaskGroupOperation); !ok {
		t.Fatalf("got %+v", ops)
	}
}

func TestCompile_CancelTaskGroupUndeclaredSlot(t *testing.T) {
	def := taskGroupDef([]program.Operation{program.CancelTaskGroupOperation{Slot: "Nope", Reason: program.StringLiteralExpression{Value: "done"}}})
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an undeclared task-group slot error")
	}
}

func TestCompile_CancelTaskGroupReasonMustBeString(t *testing.T) {
	def := taskGroupDef([]program.Operation{program.CancelTaskGroupOperation{Slot: "Workers", Reason: program.NumberLiteralExpression{Value: "1"}}})
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a reason-type error")
	}
}

func TestCompile_TaskGroupQuorumCountMustBeNumber(t *testing.T) {
	def := taskGroupDef([]program.Operation{
		program.BeginTaskGroupOperation{Slot: "Workers", Completion: program.TaskGroupQuorumTerminalPolicy{Count: program.StringLiteralExpression{Value: "two"}}},
	})
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a quorum-count-type error")
	}
}

func TestCompile_TaskGroupCompletedSignalUndeclaredSlot(t *testing.T) {
	def := workflowWithOneTransition(program.TransitionDeclaration{
		Signal:  program.SignalPattern{Source: program.TaskGroupCompletedSignalSource{Slot: "Nope"}},
		Control: program.StayControl{},
	})
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an undeclared task-group slot error")
	}
}
