package engineservice

import (
	"testing"

	"github.com/diegobermudez03/playhoot/game/v1/engine"
	"github.com/diegobermudez03/playhoot/game/v1/program"
)

func TestCompile_SimpleResource(t *testing.T) {
	def := program.Definition{
		Resources: []program.ResourceDeclaration{
			{Name: "StartingGold", Type: numberType(), Value: program.NumberLiteralExpression{Value: "100"}},
		},
	}
	p, diags := Compile(withMinimalRootWorkflow(def))
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	v, ok := p.Resources["StartingGold"]
	if !ok || v.(engine.NumberValue).Value != 100 {
		t.Fatalf("got %+v", p.Resources)
	}
}

func TestCompile_ResourceDependsOnAnotherResource(t *testing.T) {
	def := program.Definition{
		Resources: []program.ResourceDeclaration{
			{Name: "Base", Type: numberType(), Value: program.NumberLiteralExpression{Value: "10"}},
			{
				Name: "Doubled",
				Type: numberType(),
				Value: program.BinaryExpression{
					Operator: program.BinaryOperatorMultiply,
					Left:     program.FieldExpression{Target: program.ReferenceExpression{Name: "resources"}, Field: "Base"},
					Right:    program.NumberLiteralExpression{Value: "2"},
				},
			},
		},
	}
	p, diags := Compile(withMinimalRootWorkflow(def))
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	if p.Resources["Doubled"].(engine.NumberValue).Value != 20 {
		t.Fatalf("got %+v", p.Resources)
	}
}

func TestCompile_ResourceDirectCycleRejected(t *testing.T) {
	def := program.Definition{
		Resources: []program.ResourceDeclaration{
			{Name: "A", Type: numberType(), Value: program.FieldExpression{Target: program.ReferenceExpression{Name: "resources"}, Field: "A"}},
		},
	}
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a resource self-dependency error")
	}
}

func TestCompile_ResourceMutualCycleRejected(t *testing.T) {
	def := program.Definition{
		Resources: []program.ResourceDeclaration{
			{Name: "A", Type: numberType(), Value: program.FieldExpression{Target: program.ReferenceExpression{Name: "resources"}, Field: "B"}},
			{Name: "B", Type: numberType(), Value: program.FieldExpression{Target: program.ReferenceExpression{Name: "resources"}, Field: "A"}},
		},
	}
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a resource mutual-dependency error")
	}
}

func TestCompile_ResourceCycleThroughFunctionRejected(t *testing.T) {
	// A's value calls ReadB, which reads resources.B; B's value reads
	// resources.A directly. The cycle only exists once ReadB's own
	// resource references are accounted for.
	def := program.Definition{
		Resources: []program.ResourceDeclaration{
			{Name: "A", Type: numberType(), Value: program.CallExpression{Function: "ReadB"}},
			{Name: "B", Type: numberType(), Value: program.FieldExpression{Target: program.ReferenceExpression{Name: "resources"}, Field: "A"}},
		},
		Functions: []program.FunctionDeclaration{
			{Name: "ReadB", ResultType: numberType(), Body: program.FieldExpression{Target: program.ReferenceExpression{Name: "resources"}, Field: "B"}},
		},
	}
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a resource cycle error reached through a function call")
	}
}

func TestCompile_ResourceReferencesUndeclaredResource(t *testing.T) {
	def := program.Definition{
		Resources: []program.ResourceDeclaration{
			{Name: "A", Type: numberType(), Value: program.FieldExpression{Target: program.ReferenceExpression{Name: "resources"}, Field: "Nonexistent"}},
		},
	}
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an undeclared-resource-reference error")
	}
}

func TestCompile_ResourceTypeMismatch(t *testing.T) {
	def := program.Definition{
		Resources: []program.ResourceDeclaration{
			{Name: "A", Type: numberType(), Value: program.StringLiteralExpression{Value: "oops"}},
		},
	}
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a resource type-mismatch error")
	}
}

func TestCompile_DuplicateResourceName(t *testing.T) {
	def := program.Definition{
		Resources: []program.ResourceDeclaration{
			{Name: "A", Type: numberType(), Value: program.NumberLiteralExpression{Value: "1"}},
			{Name: "A", Type: numberType(), Value: program.NumberLiteralExpression{Value: "2"}},
		},
	}
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a duplicate resource name error")
	}
}

func TestCompile_FunctionCanReadResources(t *testing.T) {
	def := program.Definition{
		Resources: []program.ResourceDeclaration{
			{Name: "Bonus", Type: numberType(), Value: program.NumberLiteralExpression{Value: "5"}},
		},
		Functions: []program.FunctionDeclaration{
			{
				Name:       "WithBonus",
				Parameters: []program.FieldDeclaration{{Name: "base", Type: numberType()}},
				ResultType: numberType(),
				Body: program.BinaryExpression{
					Operator: program.BinaryOperatorAdd,
					Left:     program.ReferenceExpression{Name: "base"},
					Right:    program.FieldExpression{Target: program.ReferenceExpression{Name: "resources"}, Field: "Bonus"},
				},
			},
		},
	}
	p, diags := Compile(withMinimalRootWorkflow(def))
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	fn := p.Functions["WithBonus"]
	v, err := Evaluate(p, fn.Body, engine.Scope{Bindings: map[string]engine.Value{"base": engine.NumberValue{Value: 10}}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.(engine.NumberValue).Value != 15 {
		t.Fatalf("got %v, want 15", v)
	}
}
