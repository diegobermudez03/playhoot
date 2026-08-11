package runtime_test

import (
	"testing"

	"github.com/diegobermudez03/playhoot/game/v1/engine"
	"github.com/diegobermudez03/playhoot/game/v1/engine/internal/compiler"
	"github.com/diegobermudez03/playhoot/game/v1/engine/internal/runtime"
	"github.com/diegobermudez03/playhoot/game/v1/program"
)

func compileRootWorkflowWithSlots(t *testing.T) engine.Program {
	t.Helper()
	def := program.Definition{
		Questions: []program.QuestionDeclaration{
			{Name: "Confirm", ResponseType: program.BuiltinTypeReference{Type: program.BuiltinTypeBool}},
		},
		Workflows: []program.WorkflowDeclaration{
			{Name: "Sub", ResultType: numberType(), InitialState: "S", States: []program.WorkflowStateDeclaration{{Name: "S"}}},
			{
				Name:       "Main",
				Parameters: []program.FieldDeclaration{{Name: "startAt", Type: numberType()}},
				ResultType: program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				LocalState: program.StateDeclaration{Fields: []program.StateFieldDeclaration{
					{Name: "counter", Type: numberType(), Initializer: program.ReferenceExpression{Name: "startAt"}},
				}},
				QuestionSlots:  []program.QuestionSlotDeclaration{{Name: "Ask", Question: "Confirm"}},
				AskGroupSlots:  []program.AskGroupSlotDeclaration{{Name: "AskAll", Question: "Confirm"}},
				TimerSlots:     []program.TimerSlotDeclaration{{Name: "Deadline"}},
				ChildSlots:     []program.ChildWorkflowSlotDeclaration{{Name: "SubSlot", Workflow: "Sub"}},
				TaskGroupSlots: []program.TaskGroupSlotDeclaration{{Name: "Tasks", Workflow: "Sub", KeyType: program.BuiltinTypeReference{Type: program.BuiltinTypeString}}},
				InitialState:   "Start",
				States:         []program.WorkflowStateDeclaration{{Name: "Start"}},
			},
		},
		RootWorkflow: "Main",
	}
	p, diags := compiler.Compile(def)
	if diags.HasErrors() {
		t.Fatalf("unexpected compile errors: %v", diags)
	}
	return p
}

func TestNewSnapshot_RootInstanceHasAllDeclaredSlotsEmpty(t *testing.T) {
	p := compileRootWorkflowWithSlots(t)

	snap, signal, err := runtime.NewSnapshot(p, engine.InitializationInput{
		RootParameters: map[string]engine.Value{"startAt": engine.NumberValue{Value: 5}},
		Seed:           42,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	root := snap.Root
	if root.Workflow != "Main" || root.State != "Start" {
		t.Fatalf("got %+v", root)
	}
	if len(root.Parameters) != 1 || root.Parameters[0].Name != "startAt" || root.Parameters[0].Value.(engine.NumberValue).Value != 5 {
		t.Fatalf("parameters not bound correctly: %+v", root.Parameters)
	}
	counter, ok := root.LocalState.FieldByName("counter")
	if !ok || counter.Value.(engine.NumberValue).Value != 5 {
		t.Fatalf("local state not initialized from parameter: %+v", root.LocalState)
	}

	if len(root.QuestionSlots) != 1 || root.QuestionSlots[0].Name != "Ask" || root.QuestionSlots[0].Pending != nil {
		t.Fatalf("got %+v", root.QuestionSlots)
	}
	if len(root.AskGroupSlots) != 1 || root.AskGroupSlots[0].Name != "AskAll" || root.AskGroupSlots[0].Pending != nil {
		t.Fatalf("got %+v", root.AskGroupSlots)
	}
	if len(root.TimerSlots) != 1 || root.TimerSlots[0].Name != "Deadline" || root.TimerSlots[0].Pending {
		t.Fatalf("got %+v", root.TimerSlots)
	}
	if len(root.ChildSlots) != 1 || root.ChildSlots[0].Name != "SubSlot" || root.ChildSlots[0].Child != nil {
		t.Fatalf("got %+v", root.ChildSlots)
	}
	if len(root.TaskGroupSlots) != 1 || root.TaskGroupSlots[0].Name != "Tasks" || root.TaskGroupSlots[0].Group != nil {
		t.Fatalf("got %+v", root.TaskGroupSlots)
	}

	if signal.Name != "WorkflowStarted" {
		t.Fatalf("expected WorkflowStarted signal, got %+v", signal)
	}
	if snap.Random.State != 42 {
		t.Fatalf("got seed %d, want 42", snap.Random.State)
	}
	if snap.Sequence != 0 {
		t.Fatalf("got sequence %d, want 0", snap.Sequence)
	}
}

func TestNewSnapshot_MissingRootParameterRejected(t *testing.T) {
	p := compileRootWorkflowWithSlots(t)
	_, _, err := runtime.NewSnapshot(p, engine.InitializationInput{})
	if err == nil {
		t.Fatal("expected a missing-argument error")
	}
	execErr, ok := err.(*runtime.ExecutionError)
	if !ok || execErr.Code != runtime.ExecutionErrorInvalidInitialState {
		t.Fatalf("expected runtime.ExecutionErrorInvalidInitialState, got %v", err)
	}
}

func TestNewSnapshot_WrongTypeRootParameterRejected(t *testing.T) {
	p := compileRootWorkflowWithSlots(t)
	_, _, err := runtime.NewSnapshot(p, engine.InitializationInput{
		RootParameters: map[string]engine.Value{"startAt": engine.StringValue{Value: "not a number"}},
	})
	if err == nil {
		t.Fatal("expected a type-mismatch error")
	}
}

func TestNewSnapshot_DoesNotExecuteAnyTransition(t *testing.T) {
	// A workflow whose only transition, if executed, would produce a
	// value distinguishable from the freshly initialized state: since
	// initialization must not recursively execute it, the state after
	// runtime.NewSnapshot must still be exactly the InitialState, untouched.
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
								Signal:  program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
								Control: program.GotoControl{State: "Finished"},
							},
						},
					},
					{Name: "Finished"},
				},
			},
		},
		RootWorkflow: "Main",
	}
	p, diags := compiler.Compile(def)
	if diags.HasErrors() {
		t.Fatalf("unexpected compile errors: %v", diags)
	}

	snap, signal, err := runtime.NewSnapshot(p, engine.InitializationInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.Root.State != "Start" {
		t.Fatalf("expected the workflow to remain in its InitialState, got %q", snap.Root.State)
	}
	if signal.Name != "WorkflowStarted" {
		t.Fatalf("expected runtime.NewSnapshot to hand back the WorkflowStarted signal for a future runtime.Step, got %+v", signal)
	}
}
