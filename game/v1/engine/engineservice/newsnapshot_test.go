package engineservice

import (
	"testing"

	"github.com/diegobermudez03/playhoot/game/v1/engine"
	"github.com/diegobermudez03/playhoot/game/v1/program"
)

func TestNewSnapshot_ConstructsInitialGlobalState(t *testing.T) {
	def := program.Definition{
		Resources: []program.ResourceDeclaration{
			{Name: "StartingScore", Type: numberType(), Value: program.NumberLiteralExpression{Value: "7"}},
		},
		GlobalState: program.StateDeclaration{
			Fields: []program.StateFieldDeclaration{
				{Name: "score", Type: numberType(), Initializer: program.FieldExpression{Target: program.ReferenceExpression{Name: "resources"}, Field: "StartingScore"}},
				{Name: "started", Type: boolType(), Initializer: program.BoolLiteralExpression{Value: false}},
			},
		},
	}
	p, diags := Compile(withMinimalRootWorkflow(def))
	if diags.HasErrors() {
		t.Fatalf("unexpected compile errors: %v", diags)
	}

	snap, signal, err := NewSnapshot(p, engine.InitializationInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	score, ok := snap.GlobalState.FieldByName("score")
	if !ok || score.Value.(engine.NumberValue).Value != 7 {
		t.Fatalf("got %+v", snap.GlobalState)
	}
	started, ok := snap.GlobalState.FieldByName("started")
	if !ok || started.Value.(engine.BoolValue).Value != false {
		t.Fatalf("got %+v", snap.GlobalState)
	}
	if signal.Name != "WorkflowStarted" {
		t.Fatalf("expected a WorkflowStarted signal, got %+v", signal)
	}
	if snap.Root.Workflow != "Main" || snap.Root.State != "Start" {
		t.Fatalf("root instance not initialized correctly: %+v", snap.Root)
	}
}

func TestNewSnapshot_PassingInvariantSucceeds(t *testing.T) {
	def := program.Definition{
		GlobalState: program.StateDeclaration{
			Fields: []program.StateFieldDeclaration{
				{Name: "score", Type: numberType(), Initializer: program.NumberLiteralExpression{Value: "0"}},
			},
		},
		Invariants: []program.InvariantDeclaration{
			{
				Name: "ScoreNeverNegative",
				Condition: program.BinaryExpression{
					Operator: program.BinaryOperatorGreaterOrEqual,
					Left:     program.FieldExpression{Target: program.ReferenceExpression{Name: "global"}, Field: "score"},
					Right:    program.NumberLiteralExpression{Value: "0"},
				},
			},
		},
	}
	p, diags := Compile(withMinimalRootWorkflow(def))
	if diags.HasErrors() {
		t.Fatalf("unexpected compile errors: %v", diags)
	}
	if _, _, err := NewSnapshot(p, engine.InitializationInput{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewSnapshot_ViolatedInvariantAtomicallyRejectsInitialization(t *testing.T) {
	def := program.Definition{
		GlobalState: program.StateDeclaration{
			Fields: []program.StateFieldDeclaration{
				{Name: "score", Type: numberType(), Initializer: program.NumberLiteralExpression{Value: "-5"}},
			},
		},
		Invariants: []program.InvariantDeclaration{
			{
				Name: "ScoreNeverNegative",
				Condition: program.BinaryExpression{
					Operator: program.BinaryOperatorGreaterOrEqual,
					Left:     program.FieldExpression{Target: program.ReferenceExpression{Name: "global"}, Field: "score"},
					Right:    program.NumberLiteralExpression{Value: "0"},
				},
			},
		},
	}
	p, diags := Compile(withMinimalRootWorkflow(def))
	if diags.HasErrors() {
		t.Fatalf("unexpected compile errors: %v", diags)
	}

	snap, _, err := NewSnapshot(p, engine.InitializationInput{})
	if err == nil {
		t.Fatal("expected an invariant-violation error")
	}
	if len(snap.GlobalState.Fields) != 0 || snap.GlobalState.TypeName != "" {
		t.Fatalf("expected a zero Snapshot on rejection, got %+v", snap)
	}
	execErr, ok := err.(*ExecutionError)
	if !ok || execErr.Code != ExecutionErrorInvariantViolation {
		t.Fatalf("expected ExecutionErrorInvariantViolation, got %v", err)
	}
}

func TestCompile_InvariantMustBeStaticallyBool(t *testing.T) {
	def := program.Definition{
		Invariants: []program.InvariantDeclaration{
			{Name: "Broken", Condition: program.NumberLiteralExpression{Value: "1"}},
		},
	}
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an invariant-type error")
	}
}

func TestCompile_DuplicateGlobalStateFieldName(t *testing.T) {
	def := program.Definition{
		GlobalState: program.StateDeclaration{
			Fields: []program.StateFieldDeclaration{
				{Name: "x", Type: numberType(), Initializer: program.NumberLiteralExpression{Value: "1"}},
				{Name: "x", Type: numberType(), Initializer: program.NumberLiteralExpression{Value: "2"}},
			},
		},
	}
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a duplicate state field name error")
	}
}
