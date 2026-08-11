package runtime_test

import (
	"errors"
	"testing"

	"github.com/diegobermudez03/playhoot/game/v1/engine"
	"github.com/diegobermudez03/playhoot/game/v1/engine/internal/runtime"
)

func emptyScope() engine.Scope { return engine.Scope{} }

// poisonDivide evaluates to an runtime.ExecutionErrorDivisionByZero if ever
// evaluated — used to prove short-circuit evaluation never reaches it.
var poisonDivide = engine.BinaryExpression{
	Operator: engine.BinaryOperatorDivide,
	Left:     engine.NumberLiteralExpression{Value: 10},
	Right:    engine.NumberLiteralExpression{Value: 0},
}

func TestEvaluate_Literals(t *testing.T) {
	cases := []struct {
		expr engine.Expression
		want engine.Value
	}{
		{engine.UnitLiteralExpression{}, engine.UnitValue{}},
		{engine.BoolLiteralExpression{Value: true}, engine.BoolValue{Value: true}},
		{engine.NumberLiteralExpression{Value: 3.5}, engine.NumberValue{Value: 3.5}},
		{engine.StringLiteralExpression{Value: "hi"}, engine.StringValue{Value: "hi"}},
	}
	for _, c := range cases {
		v, err := runtime.Evaluate(engine.Program{}, c.expr, emptyScope())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !v.Equal(c.want) {
			t.Fatalf("got %#v, want %#v", v, c.want)
		}
	}
}

func TestEvaluate_AndShortCircuits(t *testing.T) {
	expr := engine.BinaryExpression{Operator: engine.BinaryOperatorAnd, Left: engine.BoolLiteralExpression{Value: false}, Right: poisonDivide}
	v, err := runtime.Evaluate(engine.Program{}, expr, emptyScope())
	if err != nil {
		t.Fatalf("expected short-circuit to avoid the error, got: %v", err)
	}
	if v.(engine.BoolValue).Value != false {
		t.Fatalf("got %v, want false", v)
	}
}

func TestEvaluate_AndEvaluatesRightWhenNeeded(t *testing.T) {
	expr := engine.BinaryExpression{Operator: engine.BinaryOperatorAnd, Left: engine.BoolLiteralExpression{Value: true}, Right: engine.BoolLiteralExpression{Value: false}}
	v, err := runtime.Evaluate(engine.Program{}, expr, emptyScope())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.(engine.BoolValue).Value != false {
		t.Fatalf("got %v, want false", v)
	}
}

func TestEvaluate_OrShortCircuits(t *testing.T) {
	expr := engine.BinaryExpression{Operator: engine.BinaryOperatorOr, Left: engine.BoolLiteralExpression{Value: true}, Right: poisonDivide}
	v, err := runtime.Evaluate(engine.Program{}, expr, emptyScope())
	if err != nil {
		t.Fatalf("expected short-circuit to avoid the error, got: %v", err)
	}
	if v.(engine.BoolValue).Value != true {
		t.Fatalf("got %v, want true", v)
	}
}

func TestEvaluate_OrEvaluatesRightWhenNeeded(t *testing.T) {
	expr := engine.BinaryExpression{Operator: engine.BinaryOperatorOr, Left: engine.BoolLiteralExpression{Value: false}, Right: engine.BoolLiteralExpression{Value: true}}
	v, err := runtime.Evaluate(engine.Program{}, expr, emptyScope())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.(engine.BoolValue).Value != true {
		t.Fatalf("got %v, want true", v)
	}
}

func TestEvaluate_ConditionalOnlyEvaluatesSelectedBranch(t *testing.T) {
	thenExpr := engine.ConditionalExpression{Condition: engine.BoolLiteralExpression{Value: true}, Then: engine.NumberLiteralExpression{Value: 1}, Else: poisonDivide}
	v, err := runtime.Evaluate(engine.Program{}, thenExpr, emptyScope())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.(engine.NumberValue).Value != 1 {
		t.Fatalf("got %v, want 1", v)
	}

	elseExpr := engine.ConditionalExpression{Condition: engine.BoolLiteralExpression{Value: false}, Then: poisonDivide, Else: engine.NumberLiteralExpression{Value: 2}}
	v, err = runtime.Evaluate(engine.Program{}, elseExpr, emptyScope())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.(engine.NumberValue).Value != 2 {
		t.Fatalf("got %v, want 2", v)
	}
}

func TestEvaluate_DivisionByZero(t *testing.T) {
	_, err := runtime.Evaluate(engine.Program{}, poisonDivide, emptyScope())
	if err == nil {
		t.Fatal("expected a division-by-zero error")
	}
	var execErr *runtime.ExecutionError
	if !errors.As(err, &execErr) || execErr.Code != runtime.ExecutionErrorDivisionByZero {
		t.Fatalf("expected runtime.ExecutionErrorDivisionByZero, got %v", err)
	}
}

func TestEvaluate_UndefinedReference(t *testing.T) {
	_, err := runtime.Evaluate(engine.Program{}, engine.ReferenceExpression{Name: "missing"}, emptyScope())
	if err == nil {
		t.Fatal("expected an undefined-reference error")
	}
	var execErr *runtime.ExecutionError
	if !errors.As(err, &execErr) || execErr.Code != runtime.ExecutionErrorUndefinedReference {
		t.Fatalf("expected runtime.ExecutionErrorUndefinedReference, got %v", err)
	}
}

func TestEvaluate_ListIndexOutOfRange(t *testing.T) {
	expr := engine.IndexExpression{
		Target: engine.ListExpression{ElementType: engine.NumberType{}, Elements: []engine.Expression{engine.NumberLiteralExpression{Value: 1}}},
		Index:  engine.NumberLiteralExpression{Value: 5},
	}
	_, err := runtime.Evaluate(engine.Program{}, expr, emptyScope())
	var execErr *runtime.ExecutionError
	if !errors.As(err, &execErr) || execErr.Code != runtime.ExecutionErrorIndexOutOfRange {
		t.Fatalf("expected runtime.ExecutionErrorIndexOutOfRange, got %v", err)
	}
}

func TestEvaluate_MapKeyNotFound(t *testing.T) {
	expr := engine.IndexExpression{
		Target: engine.MapExpression{
			KeyType: engine.StringType{}, ValueType: engine.NumberType{},
			Entries: []engine.MapEntryExpression{{Key: engine.StringLiteralExpression{Value: "a"}, Value: engine.NumberLiteralExpression{Value: 1}}},
		},
		Index: engine.StringLiteralExpression{Value: "b"},
	}
	_, err := runtime.Evaluate(engine.Program{}, expr, emptyScope())
	var execErr *runtime.ExecutionError
	if !errors.As(err, &execErr) || execErr.Code != runtime.ExecutionErrorKeyNotFound {
		t.Fatalf("expected runtime.ExecutionErrorKeyNotFound, got %v", err)
	}
}

func TestEvaluate_ListAnyShortCircuits(t *testing.T) {
	// items: [5, 0]; predicate: (10 / item) == 2 — true for item=5,
	// division-by-zero for item=0. Any must stop after item=5.
	expr := engine.ListAnyExpression{
		Collection: engine.ListExpression{ElementType: engine.NumberType{}, Elements: []engine.Expression{
			engine.NumberLiteralExpression{Value: 5}, engine.NumberLiteralExpression{Value: 0},
		}},
		ItemName: "item",
		Predicate: engine.BinaryExpression{
			Operator: engine.BinaryOperatorEqual,
			Left:     engine.BinaryExpression{Operator: engine.BinaryOperatorDivide, Left: engine.NumberLiteralExpression{Value: 10}, Right: engine.ReferenceExpression{Name: "item"}},
			Right:    engine.NumberLiteralExpression{Value: 2},
		},
	}
	v, err := runtime.Evaluate(engine.Program{}, expr, emptyScope())
	if err != nil {
		t.Fatalf("expected short-circuit to avoid the division error, got: %v", err)
	}
	if v.(engine.BoolValue).Value != true {
		t.Fatalf("got %v, want true", v)
	}
}

func TestEvaluate_ListAllShortCircuits(t *testing.T) {
	// items: [5, 0]; predicate: (10 / item) != 2 — false for item=5,
	// division-by-zero for item=0. All must stop after item=5.
	expr := engine.ListAllExpression{
		Collection: engine.ListExpression{ElementType: engine.NumberType{}, Elements: []engine.Expression{
			engine.NumberLiteralExpression{Value: 5}, engine.NumberLiteralExpression{Value: 0},
		}},
		ItemName: "item",
		Predicate: engine.BinaryExpression{
			Operator: engine.BinaryOperatorNotEqual,
			Left:     engine.BinaryExpression{Operator: engine.BinaryOperatorDivide, Left: engine.NumberLiteralExpression{Value: 10}, Right: engine.ReferenceExpression{Name: "item"}},
			Right:    engine.NumberLiteralExpression{Value: 2},
		},
	}
	v, err := runtime.Evaluate(engine.Program{}, expr, emptyScope())
	if err != nil {
		t.Fatalf("expected short-circuit to avoid the division error, got: %v", err)
	}
	if v.(engine.BoolValue).Value != false {
		t.Fatalf("got %v, want false", v)
	}
}

func TestEvaluate_ListCountNeverShortCircuits(t *testing.T) {
	expr := engine.ListCountExpression{
		Collection: engine.ListExpression{ElementType: engine.NumberType{}, Elements: []engine.Expression{
			engine.NumberLiteralExpression{Value: 1}, engine.NumberLiteralExpression{Value: 2}, engine.NumberLiteralExpression{Value: 3},
		}},
		ItemName:  "item",
		Predicate: engine.BinaryExpression{Operator: engine.BinaryOperatorGreater, Left: engine.ReferenceExpression{Name: "item"}, Right: engine.NumberLiteralExpression{Value: 1}},
	}
	v, err := runtime.Evaluate(engine.Program{}, expr, emptyScope())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.(engine.NumberValue).Value != 2 {
		t.Fatalf("got %v, want 2", v)
	}
}

func TestEvaluate_ListFirstAndEmptyResult(t *testing.T) {
	expr := engine.ListFirstExpression{
		Collection: engine.ListExpression{ElementType: engine.NumberType{}, Elements: []engine.Expression{
			engine.NumberLiteralExpression{Value: 1}, engine.NumberLiteralExpression{Value: 4}, engine.NumberLiteralExpression{Value: 5},
		}},
		ItemName:  "item",
		Predicate: engine.BinaryExpression{Operator: engine.BinaryOperatorGreater, Left: engine.ReferenceExpression{Name: "item"}, Right: engine.NumberLiteralExpression{Value: 3}},
	}
	v, err := runtime.Evaluate(engine.Program{}, expr, emptyScope())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	opt := v.(engine.OptionalValue)
	if opt.Value == nil || opt.Value.(engine.NumberValue).Value != 4 {
		t.Fatalf("got %+v, want present 4", opt)
	}

	noneExpr := engine.ListFirstExpression{
		Collection: engine.ListExpression{ElementType: engine.NumberType{}, Elements: []engine.Expression{engine.NumberLiteralExpression{Value: 1}}},
		ItemName:   "item",
		Predicate:  engine.BinaryExpression{Operator: engine.BinaryOperatorGreater, Left: engine.ReferenceExpression{Name: "item"}, Right: engine.NumberLiteralExpression{Value: 100}},
	}
	v, err = runtime.Evaluate(engine.Program{}, noneExpr, emptyScope())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	opt = v.(engine.OptionalValue)
	if opt.Value != nil {
		t.Fatalf("got %+v, want absent", opt)
	}
}

func TestEvaluate_MatchNoCaseMatchesReturnsError(t *testing.T) {
	expr := engine.MatchExpression{
		Value: engine.EnumValueExpression{TypeName: "Color", ValueName: "BLUE"},
		Cases: []engine.MatchExpressionCase{
			{Pattern: engine.EnumValueMatchPattern{TypeName: "Color", ValueName: "RED"}, Result: engine.NumberLiteralExpression{Value: 1}},
		},
	}
	_, err := runtime.Evaluate(engine.Program{}, expr, emptyScope())
	var execErr *runtime.ExecutionError
	if !errors.As(err, &execErr) || execErr.Code != runtime.ExecutionErrorNoMatchingCase {
		t.Fatalf("expected runtime.ExecutionErrorNoMatchingCase, got %v", err)
	}
}

func TestEvaluate_InAndNotIn(t *testing.T) {
	list := engine.ListExpression{ElementType: engine.NumberType{}, Elements: []engine.Expression{
		engine.NumberLiteralExpression{Value: 1}, engine.NumberLiteralExpression{Value: 2},
	}}
	in := engine.BinaryExpression{Operator: engine.BinaryOperatorIn, Left: engine.NumberLiteralExpression{Value: 1}, Right: list}
	v, err := runtime.Evaluate(engine.Program{}, in, emptyScope())
	if err != nil || v.(engine.BoolValue).Value != true {
		t.Fatalf("got %v, %v; want true, nil", v, err)
	}

	notIn := engine.BinaryExpression{Operator: engine.BinaryOperatorNotIn, Left: engine.NumberLiteralExpression{Value: 9}, Right: list}
	v, err = runtime.Evaluate(engine.Program{}, notIn, emptyScope())
	if err != nil || v.(engine.BoolValue).Value != true {
		t.Fatalf("got %v, %v; want true, nil", v, err)
	}
}
