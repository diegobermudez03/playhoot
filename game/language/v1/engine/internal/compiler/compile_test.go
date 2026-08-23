package compiler_test

import (
	"testing"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/internal/compiler"
	"github.com/diegobermudez03/playhoot/game/language/v1/program"
)

// withMinimalRootWorkflow adds a trivially valid root workflow to def,
// for tests that exercise other compiler concerns and don't care about
// workflow validation, but must satisfy validateRootWorkflow's now-
// mandatory check to compile without errors.
func withMinimalRootWorkflow(def program.Definition) program.Definition {
	def.Workflows = append(def.Workflows, program.WorkflowDeclaration{
		Name:         "Main",
		ResultType:   program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
		InitialState: "Start",
		States:       []program.WorkflowStateDeclaration{{Name: "Start"}},
	})
	def.RootWorkflow = "Main"
	return def
}

func TestCompile_Metadata(t *testing.T) {
	def := program.Definition{
		Metadata: program.Metadata{
			ID:              "parques",
			Name:            "Parques",
			Description:     "A race board game",
			Version:         "1.0.0",
			LanguageVersion: "1",
		},
	}

	p, diags := compiler.Compile(withMinimalRootWorkflow(def))
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	if p.Metadata != (engine.Metadata{
		ID:              "parques",
		Name:            "Parques",
		Description:     "A race board game",
		Version:         "1.0.0",
		LanguageVersion: "1",
	}) {
		t.Fatalf("metadata not carried over: %+v", p.Metadata)
	}
}

func TestCompile_PlayerPolicy(t *testing.T) {
	def := program.Definition{
		Players: program.PlayerPolicy{Min: 2, Max: 6},
	}

	p, diags := compiler.Compile(withMinimalRootWorkflow(def))
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	if p.Players != (engine.PlayerPolicy{Min: 2, Max: 6}) {
		t.Fatalf("player policy not carried over: %+v", p.Players)
	}
}

func TestCompile_PlayerPolicyMinGreaterThanMax(t *testing.T) {
	def := program.Definition{
		Players: program.PlayerPolicy{Min: 6, Max: 4},
	}

	_, diags := compiler.Compile(withMinimalRootWorkflow(def))
	if !diags.HasErrors() {
		t.Fatal("expected player policy compile error")
	}
}

func TestCompile_EnumType(t *testing.T) {
	def := program.Definition{
		Types: []program.TypeDeclaration{
			program.EnumTypeDeclaration{
				Name: "Color",
				Values: []program.EnumValueDeclaration{
					{Name: "RED"},
					{Name: "GREEN"},
				},
			},
		},
	}

	p, diags := compiler.Compile(withMinimalRootWorkflow(def))
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	got, ok := p.Types["Color"].(engine.EnumType)
	if !ok {
		t.Fatalf("Color not compiled as EnumType: %#v", p.Types["Color"])
	}
	want := engine.EnumType{Name: "Color", Values: []string{"RED", "GREEN"}}
	if !got.Equal(want) || len(got.Values) != len(want.Values) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestCompile_RecordWithNamedAndBuiltinFields(t *testing.T) {
	def := program.Definition{
		Types: []program.TypeDeclaration{
			program.EnumTypeDeclaration{
				Name:   "Color",
				Values: []program.EnumValueDeclaration{{Name: "RED"}},
			},
			program.RecordTypeDeclaration{
				Name: "Piece",
				Fields: []program.FieldDeclaration{
					{Name: "color", Type: program.NamedTypeReference{Name: "Color"}},
					{Name: "position", Type: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}},
				},
			},
		},
	}

	p, diags := compiler.Compile(withMinimalRootWorkflow(def))
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	piece, ok := p.Types["Piece"].(engine.RecordType)
	if !ok {
		t.Fatalf("Piece not compiled as RecordType: %#v", p.Types["Piece"])
	}
	colorField, ok := piece.FieldByName("color")
	if !ok {
		t.Fatal("missing color field")
	}
	colorType, ok := colorField.Type.(engine.EnumType)
	if !ok || colorType.Name != "Color" {
		t.Fatalf("color field not resolved to the Color enum: %#v", colorField.Type)
	}
	positionField, ok := piece.FieldByName("position")
	if !ok {
		t.Fatal("missing position field")
	}
	if _, ok := positionField.Type.(engine.NumberType); !ok {
		t.Fatalf("position field not compiled as NumberType: %#v", positionField.Type)
	}
}

func TestCompile_UnionAndNewType(t *testing.T) {
	def := program.Definition{
		Types: []program.TypeDeclaration{
			program.UnionTypeDeclaration{
				Name: "Shape",
				Variants: []program.UnionVariantDeclaration{
					{Name: "Circle", Fields: []program.FieldDeclaration{
						{Name: "radius", Type: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}},
					}},
					{Name: "Point"},
				},
			},
			program.NewTypeDeclaration{
				Name:       "UserId",
				Underlying: program.BuiltinTypeReference{Type: program.BuiltinTypeString},
			},
		},
	}

	p, diags := compiler.Compile(withMinimalRootWorkflow(def))
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}

	shape, ok := p.Types["Shape"].(engine.UnionType)
	if !ok {
		t.Fatalf("Shape not compiled as UnionType: %#v", p.Types["Shape"])
	}
	circle, ok := shape.VariantByName("Circle")
	if !ok || len(circle.Fields) != 1 {
		t.Fatalf("Circle variant not compiled correctly: %#v", circle)
	}
	point, ok := shape.VariantByName("Point")
	if !ok || len(point.Fields) != 0 {
		t.Fatalf("Point variant not compiled correctly: %#v", point)
	}

	userID, ok := p.Types["UserId"].(engine.NewType)
	if !ok {
		t.Fatalf("UserId not compiled as NewType: %#v", p.Types["UserId"])
	}
	if _, ok := userID.Underlying.(engine.StringType); !ok {
		t.Fatalf("UserId underlying not compiled as StringType: %#v", userID.Underlying)
	}
}

func TestCompile_ListMapOptionalReferences(t *testing.T) {
	def := program.Definition{
		Types: []program.TypeDeclaration{
			program.EnumTypeDeclaration{
				Name:   "Color",
				Values: []program.EnumValueDeclaration{{Name: "RED"}},
			},
			program.RecordTypeDeclaration{
				Name: "Board",
				Fields: []program.FieldDeclaration{
					{Name: "colors", Type: program.ListTypeReference{Element: program.NamedTypeReference{Name: "Color"}}},
					{Name: "scores", Type: program.MapTypeReference{
						Key:   program.BuiltinTypeReference{Type: program.BuiltinTypeString},
						Value: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber},
					}},
					{Name: "winner", Type: program.OptionalTypeReference{Element: program.NamedTypeReference{Name: "Color"}}},
				},
			},
		},
	}

	p, diags := compiler.Compile(withMinimalRootWorkflow(def))
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	board := p.Types["Board"].(engine.RecordType)

	colors, _ := board.FieldByName("colors")
	list, ok := colors.Type.(engine.ListType)
	if !ok {
		t.Fatalf("colors not a ListType: %#v", colors.Type)
	}
	if _, ok := list.Element.(engine.EnumType); !ok {
		t.Fatalf("colors element not resolved: %#v", list.Element)
	}

	scores, _ := board.FieldByName("scores")
	m, ok := scores.Type.(engine.MapType)
	if !ok {
		t.Fatalf("scores not a MapType: %#v", scores.Type)
	}
	if _, ok := m.Key.(engine.StringType); !ok {
		t.Fatal("scores key not StringType")
	}
	if _, ok := m.Value.(engine.NumberType); !ok {
		t.Fatal("scores value not NumberType")
	}

	winner, _ := board.FieldByName("winner")
	opt, ok := winner.Type.(engine.OptionalType)
	if !ok {
		t.Fatalf("winner not an OptionalType: %#v", winner.Type)
	}
	if _, ok := opt.Element.(engine.EnumType); !ok {
		t.Fatalf("winner element not resolved: %#v", opt.Element)
	}
}

func TestCompile_DuplicateTypeName(t *testing.T) {
	def := program.Definition{
		Types: []program.TypeDeclaration{
			program.EnumTypeDeclaration{Name: "Color", Values: []program.EnumValueDeclaration{{Name: "RED"}}},
			program.RecordTypeDeclaration{Name: "Color"},
		},
	}

	_, diags := compiler.Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a duplicate type name error")
	}
	if diags[0].Path != "$.types[1]" {
		t.Fatalf("unexpected diagnostic path: %+v", diags[0])
	}
}

func TestCompile_DuplicateFieldName(t *testing.T) {
	def := program.Definition{
		Types: []program.TypeDeclaration{
			program.RecordTypeDeclaration{
				Name: "Piece",
				Fields: []program.FieldDeclaration{
					{Name: "x", Type: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}},
					{Name: "x", Type: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}},
				},
			},
		},
	}

	p, diags := compiler.Compile(withMinimalRootWorkflow(def))
	if !diags.HasErrors() {
		t.Fatal("expected a duplicate field name error")
	}
	piece := p.Types["Piece"].(engine.RecordType)
	if len(piece.Fields) != 1 {
		t.Fatalf("expected only the first field to survive, got %+v", piece.Fields)
	}
}

func TestCompile_UnknownBuiltinAndUndeclaredReference(t *testing.T) {
	def := program.Definition{
		Types: []program.TypeDeclaration{
			program.RecordTypeDeclaration{
				Name: "Piece",
				Fields: []program.FieldDeclaration{
					{Name: "a", Type: program.BuiltinTypeReference{Type: "decimal"}},
					{Name: "b", Type: program.NamedTypeReference{Name: "Nonexistent"}},
				},
			},
		},
	}

	_, diags := compiler.Compile(withMinimalRootWorkflow(def))
	if len(diags) != 2 {
		t.Fatalf("expected exactly 2 diagnostics, got %d: %v", len(diags), diags)
	}
}

func TestCompile_DirectSelfReferenceIsDiagnosedNotInfinite(t *testing.T) {
	def := program.Definition{
		Types: []program.TypeDeclaration{
			program.RecordTypeDeclaration{
				Name: "Node",
				Fields: []program.FieldDeclaration{
					{Name: "next", Type: program.NamedTypeReference{Name: "Node"}},
				},
			},
		},
	}

	_, diags := compiler.Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a recursive-type error")
	}
}

func TestCompile_OptionalWrappedSelfReferenceIsDiagnosed(t *testing.T) {
	def := program.Definition{
		Types: []program.TypeDeclaration{
			program.RecordTypeDeclaration{
				Name: "Node",
				Fields: []program.FieldDeclaration{
					{Name: "next", Type: program.OptionalTypeReference{Element: program.NamedTypeReference{Name: "Node"}}},
				},
			},
		},
	}

	_, diags := compiler.Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a recursive-type error even through an optional")
	}
}

func TestCompile_MutualRecursionIsDiagnosed(t *testing.T) {
	def := program.Definition{
		Types: []program.TypeDeclaration{
			program.RecordTypeDeclaration{
				Name: "A",
				Fields: []program.FieldDeclaration{
					{Name: "b", Type: program.NamedTypeReference{Name: "B"}},
				},
			},
			program.RecordTypeDeclaration{
				Name: "B",
				Fields: []program.FieldDeclaration{
					{Name: "a", Type: program.NamedTypeReference{Name: "A"}},
				},
			},
		},
	}

	_, diags := compiler.Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a mutual-recursion error")
	}
}

func TestCompile_DiamondReferenceIsNotACycle(t *testing.T) {
	// A and B both reference D; D is not recursive. This must compile
	// cleanly even though D is resolved twice.
	def := program.Definition{
		Types: []program.TypeDeclaration{
			program.RecordTypeDeclaration{Name: "D", Fields: []program.FieldDeclaration{
				{Name: "v", Type: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}},
			}},
			program.RecordTypeDeclaration{Name: "A", Fields: []program.FieldDeclaration{
				{Name: "d", Type: program.NamedTypeReference{Name: "D"}},
			}},
			program.RecordTypeDeclaration{Name: "B", Fields: []program.FieldDeclaration{
				{Name: "d", Type: program.NamedTypeReference{Name: "D"}},
			}},
		},
	}

	p, diags := compiler.Compile(withMinimalRootWorkflow(def))
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	if _, ok := p.Types["D"].(engine.RecordType); !ok {
		t.Fatal("D not compiled")
	}
	if _, ok := p.Types["A"].(engine.RecordType); !ok {
		t.Fatal("A not compiled")
	}
	if _, ok := p.Types["B"].(engine.RecordType); !ok {
		t.Fatal("B not compiled")
	}
}
