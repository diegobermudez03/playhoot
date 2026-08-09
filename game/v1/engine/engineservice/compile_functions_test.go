package engineservice

import (
	"testing"

	"github.com/diegobermudez03/playhoot/game/v1/engine"
	"github.com/diegobermudez03/playhoot/game/v1/program"
)

func numberType() program.TypeReference {
	return program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}
}
func boolType() program.TypeReference {
	return program.BuiltinTypeReference{Type: program.BuiltinTypeBool}
}
func stringType() program.TypeReference {
	return program.BuiltinTypeReference{Type: program.BuiltinTypeString}
}

func TestCompile_SimpleFunction(t *testing.T) {
	def := program.Definition{
		Functions: []program.FunctionDeclaration{
			{
				Name:       "Double",
				Parameters: []program.FieldDeclaration{{Name: "x", Type: numberType()}},
				ResultType: numberType(),
				Body: program.BinaryExpression{
					Operator: program.BinaryOperatorMultiply,
					Left:     program.ReferenceExpression{Name: "x"},
					Right:    program.NumberLiteralExpression{Value: "2"},
				},
			},
		},
	}

	p, diags := Compile(withMinimalRootWorkflow(def))
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	fn, ok := p.Functions["Double"]
	if !ok {
		t.Fatal("Double not compiled")
	}
	v, err := Evaluate(p, fn.Body, engine.Scope{Bindings: map[string]engine.Value{"x": engine.NumberValue{Value: 21}}})
	if err != nil {
		t.Fatalf("unexpected evaluation error: %v", err)
	}
	if v.(engine.NumberValue).Value != 42 {
		t.Fatalf("got %v, want 42", v)
	}
}

func TestCompile_FunctionCallsAnotherFunction(t *testing.T) {
	def := program.Definition{
		Functions: []program.FunctionDeclaration{
			{
				Name:       "Square",
				Parameters: []program.FieldDeclaration{{Name: "x", Type: numberType()}},
				ResultType: numberType(),
				Body: program.BinaryExpression{
					Operator: program.BinaryOperatorMultiply,
					Left:     program.ReferenceExpression{Name: "x"},
					Right:    program.ReferenceExpression{Name: "x"},
				},
			},
			{
				Name:       "SumOfSquares",
				Parameters: []program.FieldDeclaration{{Name: "a", Type: numberType()}, {Name: "b", Type: numberType()}},
				ResultType: numberType(),
				Body: program.BinaryExpression{
					Operator: program.BinaryOperatorAdd,
					Left:     program.CallExpression{Function: "Square", Arguments: []program.CallArgument{{Name: "x", Value: program.ReferenceExpression{Name: "a"}}}},
					Right:    program.CallExpression{Function: "Square", Arguments: []program.CallArgument{{Name: "x", Value: program.ReferenceExpression{Name: "b"}}}},
				},
			},
		},
	}

	p, diags := Compile(withMinimalRootWorkflow(def))
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	fn := p.Functions["SumOfSquares"]
	v, err := Evaluate(p, fn.Body, engine.Scope{Bindings: map[string]engine.Value{
		"a": engine.NumberValue{Value: 3},
		"b": engine.NumberValue{Value: 4},
	}})
	if err != nil {
		t.Fatalf("unexpected evaluation error: %v", err)
	}
	if v.(engine.NumberValue).Value != 25 {
		t.Fatalf("got %v, want 25", v)
	}
}

func TestCompile_DirectRecursionRejected(t *testing.T) {
	def := program.Definition{
		Functions: []program.FunctionDeclaration{
			{
				Name:       "Loop",
				ResultType: numberType(),
				Body:       program.CallExpression{Function: "Loop"},
			},
		},
	}
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a recursion error")
	}
}

func TestCompile_MutualRecursionRejected(t *testing.T) {
	def := program.Definition{
		Functions: []program.FunctionDeclaration{
			{Name: "A", ResultType: numberType(), Body: program.CallExpression{Function: "B"}},
			{Name: "B", ResultType: numberType(), Body: program.CallExpression{Function: "A"}},
		},
	}
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a mutual recursion error")
	}
}

func TestCompile_FunctionNameCollidesWithBuiltin(t *testing.T) {
	def := program.Definition{
		Functions: []program.FunctionDeclaration{
			{Name: "abs", ResultType: numberType(), Body: program.NumberLiteralExpression{Value: "1"}},
		},
	}
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a built-in collision error")
	}
}

func TestCompile_UndeclaredFunctionCall(t *testing.T) {
	def := program.Definition{
		Functions: []program.FunctionDeclaration{
			{Name: "F", ResultType: numberType(), Body: program.CallExpression{Function: "Nonexistent"}},
		},
	}
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an undeclared-function error")
	}
}

func TestCompile_BuiltinCatalog(t *testing.T) {
	def := program.Definition{
		Functions: []program.FunctionDeclaration{
			{
				Name:       "ClampedAbs",
				Parameters: []program.FieldDeclaration{{Name: "x", Type: numberType()}},
				ResultType: numberType(),
				Body: program.CallExpression{
					Function: "max",
					Arguments: []program.CallArgument{
						{Name: "a", Value: program.CallExpression{Function: "abs", Arguments: []program.CallArgument{{Name: "value", Value: program.ReferenceExpression{Name: "x"}}}}},
						{Name: "b", Value: program.NumberLiteralExpression{Value: "0"}},
					},
				},
			},
		},
	}
	p, diags := Compile(withMinimalRootWorkflow(def))
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	fn := p.Functions["ClampedAbs"]
	v, err := Evaluate(p, fn.Body, engine.Scope{Bindings: map[string]engine.Value{"x": engine.NumberValue{Value: -7}}})
	if err != nil {
		t.Fatalf("unexpected evaluation error: %v", err)
	}
	if v.(engine.NumberValue).Value != 7 {
		t.Fatalf("got %v, want 7", v)
	}
}

func TestCompile_LengthBuiltinPolymorphic(t *testing.T) {
	def := program.Definition{
		Functions: []program.FunctionDeclaration{
			{
				Name:       "ListLen",
				Parameters: []program.FieldDeclaration{{Name: "items", Type: program.ListTypeReference{Element: numberType()}}},
				ResultType: numberType(),
				Body:       program.CallExpression{Function: "length", Arguments: []program.CallArgument{{Name: "value", Value: program.ReferenceExpression{Name: "items"}}}},
			},
		},
	}
	p, diags := Compile(withMinimalRootWorkflow(def))
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	fn := p.Functions["ListLen"]
	v, err := Evaluate(p, fn.Body, engine.Scope{Bindings: map[string]engine.Value{
		"items": engine.ListValue{ElementType: engine.NumberType{}, Elements: []engine.Value{engine.NumberValue{Value: 1}, engine.NumberValue{Value: 2}, engine.NumberValue{Value: 3}}},
	}})
	if err != nil {
		t.Fatalf("unexpected evaluation error: %v", err)
	}
	if v.(engine.NumberValue).Value != 3 {
		t.Fatalf("got %v, want 3", v)
	}
}

func TestCompile_ScopeViolation_ReferenceToUndeclaredName(t *testing.T) {
	def := program.Definition{
		Functions: []program.FunctionDeclaration{
			{Name: "F", ResultType: numberType(), Body: program.ReferenceExpression{Name: "global"}},
		},
	}
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an undeclared-reference error")
	}
}

func TestCompile_TypeMismatch_ResultType(t *testing.T) {
	def := program.Definition{
		Functions: []program.FunctionDeclaration{
			{Name: "F", ResultType: numberType(), Body: program.StringLiteralExpression{Value: "oops"}},
		},
	}
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a result-type mismatch error")
	}
}

func TestCompile_BinaryOperatorTypeMismatch(t *testing.T) {
	def := program.Definition{
		Functions: []program.FunctionDeclaration{
			{
				Name:       "F",
				ResultType: numberType(),
				Body: program.BinaryExpression{
					Operator: program.BinaryOperatorAdd,
					Left:     program.StringLiteralExpression{Value: "a"},
					Right:    program.NumberLiteralExpression{Value: "1"},
				},
			},
		},
	}
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an operand type-mismatch error")
	}
}

func TestCompile_ConditionalBranchMismatch(t *testing.T) {
	def := program.Definition{
		Functions: []program.FunctionDeclaration{
			{
				Name:       "F",
				ResultType: numberType(),
				Body: program.ConditionalExpression{
					Condition: program.BoolLiteralExpression{Value: true},
					Then:      program.NumberLiteralExpression{Value: "1"},
					Else:      program.StringLiteralExpression{Value: "nope"},
				},
			},
		},
	}
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a conditional branch mismatch error")
	}
}

func TestCompile_RecordAndEnumConstructionInFunction(t *testing.T) {
	def := program.Definition{
		Types: []program.TypeDeclaration{
			program.EnumTypeDeclaration{Name: "Color", Values: []program.EnumValueDeclaration{{Name: "RED"}}},
			program.RecordTypeDeclaration{Name: "Piece", Fields: []program.FieldDeclaration{
				{Name: "color", Type: program.NamedTypeReference{Name: "Color"}},
				{Name: "position", Type: numberType()},
			}},
		},
		Functions: []program.FunctionDeclaration{
			{
				Name:       "NewPiece",
				Parameters: []program.FieldDeclaration{{Name: "pos", Type: numberType()}},
				ResultType: program.NamedTypeReference{Name: "Piece"},
				Body: program.RecordExpression{
					TypeName: "Piece",
					Fields: []program.FieldInitializer{
						{Name: "color", Value: program.EnumValueExpression{TypeName: "Color", ValueName: "RED"}},
						{Name: "position", Value: program.ReferenceExpression{Name: "pos"}},
					},
				},
			},
		},
	}
	p, diags := Compile(withMinimalRootWorkflow(def))
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	fn := p.Functions["NewPiece"]
	v, err := Evaluate(p, fn.Body, engine.Scope{Bindings: map[string]engine.Value{"pos": engine.NumberValue{Value: 9}}})
	if err != nil {
		t.Fatalf("unexpected evaluation error: %v", err)
	}
	rv := v.(engine.RecordValue)
	if rv.TypeName != "Piece" {
		t.Fatalf("got %+v", rv)
	}
	posField, _ := rv.FieldByName("position")
	if posField.Value.(engine.NumberValue).Value != 9 {
		t.Fatalf("position not carried through: %+v", posField)
	}
}

func TestCompile_MissingRequiredField(t *testing.T) {
	def := program.Definition{
		Types: []program.TypeDeclaration{
			program.RecordTypeDeclaration{Name: "Piece", Fields: []program.FieldDeclaration{
				{Name: "position", Type: numberType()},
			}},
		},
		Functions: []program.FunctionDeclaration{
			{
				Name:       "F",
				ResultType: program.NamedTypeReference{Name: "Piece"},
				Body:       program.RecordExpression{TypeName: "Piece"},
			},
		},
	}
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a missing-field error")
	}
}

func TestCompile_MatchExpression(t *testing.T) {
	def := program.Definition{
		Types: []program.TypeDeclaration{
			program.UnionTypeDeclaration{Name: "Shape", Variants: []program.UnionVariantDeclaration{
				{Name: "Circle", Fields: []program.FieldDeclaration{{Name: "radius", Type: numberType()}}},
				{Name: "Point"},
			}},
		},
		Functions: []program.FunctionDeclaration{
			{
				Name:       "Area",
				Parameters: []program.FieldDeclaration{{Name: "shape", Type: program.NamedTypeReference{Name: "Shape"}}},
				ResultType: numberType(),
				Body: program.MatchExpression{
					Value: program.ReferenceExpression{Name: "shape"},
					Cases: []program.MatchExpressionCase{
						{
							Pattern: program.UnionVariantMatchPattern{TypeName: "Shape", VariantName: "Circle", Bindings: []program.MatchFieldBinding{{Field: "radius", Name: "r"}}},
							Result: program.BinaryExpression{
								Operator: program.BinaryOperatorMultiply,
								Left:     program.ReferenceExpression{Name: "r"},
								Right:    program.ReferenceExpression{Name: "r"},
							},
						},
						{
							Pattern: program.WildcardMatchPattern{},
							Result:  program.NumberLiteralExpression{Value: "0"},
						},
					},
				},
			},
		},
	}
	p, diags := Compile(withMinimalRootWorkflow(def))
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	fn := p.Functions["Area"]

	v, err := Evaluate(p, fn.Body, engine.Scope{Bindings: map[string]engine.Value{
		"shape": engine.UnionValue{TypeName: "Shape", VariantName: "Circle", Fields: []engine.FieldValue{{Name: "radius", Value: engine.NumberValue{Value: 3}}}},
	}})
	if err != nil {
		t.Fatalf("unexpected evaluation error: %v", err)
	}
	if v.(engine.NumberValue).Value != 9 {
		t.Fatalf("got %v, want 9", v)
	}

	v, err = Evaluate(p, fn.Body, engine.Scope{Bindings: map[string]engine.Value{
		"shape": engine.UnionValue{TypeName: "Shape", VariantName: "Point"},
	}})
	if err != nil {
		t.Fatalf("unexpected evaluation error: %v", err)
	}
	if v.(engine.NumberValue).Value != 0 {
		t.Fatalf("got %v, want 0", v)
	}
}

func TestCompile_ListQueryExpressions(t *testing.T) {
	def := program.Definition{
		Functions: []program.FunctionDeclaration{
			{
				Name:       "EvenSquares",
				Parameters: []program.FieldDeclaration{{Name: "items", Type: program.ListTypeReference{Element: numberType()}}},
				ResultType: program.ListTypeReference{Element: numberType()},
				Body: program.ListMapExpression{
					Collection: program.ListFilterExpression{
						Collection: program.ReferenceExpression{Name: "items"},
						ItemName:   "n",
						Predicate: program.BinaryExpression{
							Operator: program.BinaryOperatorEqual,
							Left:     program.BinaryExpression{Operator: program.BinaryOperatorModulo, Left: program.ReferenceExpression{Name: "n"}, Right: program.NumberLiteralExpression{Value: "2"}},
							Right:    program.NumberLiteralExpression{Value: "0"},
						},
					},
					ItemName: "n",
					Result:   program.BinaryExpression{Operator: program.BinaryOperatorMultiply, Left: program.ReferenceExpression{Name: "n"}, Right: program.ReferenceExpression{Name: "n"}},
				},
			},
		},
	}
	p, diags := Compile(withMinimalRootWorkflow(def))
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	fn := p.Functions["EvenSquares"]
	items := engine.ListValue{ElementType: engine.NumberType{}, Elements: []engine.Value{
		engine.NumberValue{Value: 1}, engine.NumberValue{Value: 2}, engine.NumberValue{Value: 3}, engine.NumberValue{Value: 4},
	}}
	v, err := Evaluate(p, fn.Body, engine.Scope{Bindings: map[string]engine.Value{"items": items}})
	if err != nil {
		t.Fatalf("unexpected evaluation error: %v", err)
	}
	got := v.(engine.ListValue).Elements
	if len(got) != 2 || got[0].(engine.NumberValue).Value != 4 || got[1].(engine.NumberValue).Value != 16 {
		t.Fatalf("got %+v, want [4, 16]", got)
	}
}

func TestCompile_EmptyListRequiresAnnotation(t *testing.T) {
	def := program.Definition{
		Functions: []program.FunctionDeclaration{
			{Name: "F", ResultType: program.ListTypeReference{Element: numberType()}, Body: program.ListExpression{}},
		},
	}
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an empty-list annotation error")
	}
}
