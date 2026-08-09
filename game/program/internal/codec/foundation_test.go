package codec

import "github.com/diegobermudez03/playhoot/game/program"

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

// --- generic helpers ---

func mustEncodeExpression(t *testing.T, value program.Expression) json.RawMessage {
	t.Helper()
	raw, err := encodeExpression("$", value)
	if err != nil {
		t.Fatalf("encodeExpression: %v", err)
	}
	return raw
}

func genericJSON(t *testing.T, raw json.RawMessage) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("unmarshal generic JSON: %v", err)
	}
	return v
}

func assertSemanticJSONEqual(t *testing.T, a, b json.RawMessage) {
	t.Helper()
	ga, gb := genericJSON(t, a), genericJSON(t, b)
	if !reflect.DeepEqual(ga, gb) {
		t.Fatalf("JSON values differ:\n  a = %s\n  b = %s", a, b)
	}
}

// --- program.TypeReference ---

func TestTypeReference_RoundTrip(t *testing.T) {
	original := program.OptionalTypeReference{
		Element: program.MapTypeReference{
			Key: program.NamedTypeReference{Name: "ParticipantId"},
			Value: program.ListTypeReference{
				Element: program.NamedTypeReference{Name: "TokenId"},
			},
		},
	}

	raw, err := encodeTypeReference("$", original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	decoded, err := decodeTypeReference("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", original, decoded)
	}
}

func TestTypeReference_AllVariants(t *testing.T) {
	cases := []program.TypeReference{
		program.BuiltinTypeReference{Type: program.BuiltinTypeNumber},
		program.NamedTypeReference{Name: "ParticipantId"},
		program.ListTypeReference{Element: program.BuiltinTypeReference{Type: program.BuiltinTypeString}},
		program.MapTypeReference{Key: program.BuiltinTypeReference{Type: program.BuiltinTypeString}, Value: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}},
		program.OptionalTypeReference{Element: program.NamedTypeReference{Name: "ParticipantId"}},
	}

	for _, original := range cases {
		raw, err := encodeTypeReference("$", original)
		if err != nil {
			t.Fatalf("encode %T: %v", original, err)
		}
		decoded, err := decodeTypeReference("$", raw)
		if err != nil {
			t.Fatalf("decode %T: %v", original, err)
		}
		if !reflect.DeepEqual(original, decoded) {
			t.Fatalf("round trip mismatch for %T:\n  original = %#v\n  decoded  = %#v", original, original, decoded)
		}
	}
}

func TestTypeReference_ExactJSON_Builtin(t *testing.T) {
	raw, err := encodeTypeReference("$", program.BuiltinTypeReference{Type: program.BuiltinTypeNumber})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["kind"] != "builtin" || obj["type"] != "number" {
		t.Fatalf("unexpected shape: %#v", obj)
	}
}

// --- program.TypeDeclaration ---

func TestTypeDeclaration_RoundTrip(t *testing.T) {
	cases := []program.TypeDeclaration{
		program.EnumTypeDeclaration{
			Name: "Color",
			Values: []program.EnumValueDeclaration{
				{Name: "RED"},
				{Name: "BLUE"},
			},
		},
		program.RecordTypeDeclaration{
			Name: "Participant",
			Fields: []program.FieldDeclaration{
				{Name: "id", Type: program.NamedTypeReference{Name: "ParticipantId"}},
				{Name: "user", Type: program.BuiltinTypeReference{Type: program.BuiltinTypeUser}},
			},
		},
		program.UnionTypeDeclaration{
			Name: "TokenPosition",
			Variants: []program.UnionVariantDeclaration{
				{Name: "Jail", Fields: []program.FieldDeclaration{}},
				{Name: "OnRoute", Fields: []program.FieldDeclaration{
					{Name: "routeIndex", Type: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}},
				}},
				{Name: "Finished", Fields: []program.FieldDeclaration{}},
			},
		},
		program.NewTypeDeclaration{
			Name:       "ParticipantId",
			Underlying: program.BuiltinTypeReference{Type: program.BuiltinTypeString},
		},
	}

	for _, original := range cases {
		raw, err := encodeTypeDeclaration("$", original)
		if err != nil {
			t.Fatalf("encode %T: %v", original, err)
		}
		decoded, err := decodeTypeDeclaration("$", raw)
		if err != nil {
			t.Fatalf("decode %T: %v", original, err)
		}
		if !reflect.DeepEqual(original, decoded) {
			t.Fatalf("round trip mismatch for %T:\n  original = %#v\n  decoded  = %#v", original, original, decoded)
		}
	}
}

func TestTypeDeclaration_ExactJSON_Union(t *testing.T) {
	original := program.UnionTypeDeclaration{
		Name: "TokenPosition",
		Variants: []program.UnionVariantDeclaration{
			{Name: "Jail", Fields: []program.FieldDeclaration{}},
		},
	}
	raw, err := encodeTypeDeclaration("$", original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["kind"] != "union" || obj["name"] != "TokenPosition" {
		t.Fatalf("unexpected shape: %#v", obj)
	}
	variants, ok := obj["variants"].([]any)
	if !ok || len(variants) != 1 {
		t.Fatalf("expected 1 variant, got %#v", obj["variants"])
	}
}

// --- program.MatchPattern ---

func TestMatchPattern_RoundTrip(t *testing.T) {
	cases := []program.MatchPattern{
		program.WildcardMatchPattern{},
		program.EnumValueMatchPattern{TypeName: "MatchPhase", ValueName: "PLAYING"},
		program.UnionVariantMatchPattern{
			TypeName:    "TokenPosition",
			VariantName: "OnRoute",
			Bindings: []program.MatchFieldBinding{
				{Field: "routeIndex", Name: "index"},
			},
		},
		program.UnionVariantMatchPattern{TypeName: "TokenPosition", VariantName: "Jail"},
		program.OptionalNoneMatchPattern{},
		program.OptionalSomeMatchPattern{Binding: "participant"},
		program.OptionalSomeMatchPattern{},
	}

	for _, original := range cases {
		raw, err := encodeMatchPattern("$", original)
		if err != nil {
			t.Fatalf("encode %#v: %v", original, err)
		}
		decoded, err := decodeMatchPattern("$", raw)
		if err != nil {
			t.Fatalf("decode %#v: %v", original, err)
		}
		if !reflect.DeepEqual(original, decoded) {
			t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", original, decoded)
		}
	}
}

func TestMatchPattern_BindingOrderPreserved(t *testing.T) {
	original := program.UnionVariantMatchPattern{
		TypeName:    "TurnResult",
		VariantName: "Won",
		Bindings: []program.MatchFieldBinding{
			{Field: "participant", Name: "winner"},
			{Field: "score", Name: "finalScore"},
		},
	}
	raw, err := encodeMatchPattern("$", original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeMatchPattern("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, ok := decoded.(program.UnionVariantMatchPattern)
	if !ok {
		t.Fatalf("expected program.UnionVariantMatchPattern, got %T", decoded)
	}
	if len(got.Bindings) != 2 || got.Bindings[0].Field != "participant" || got.Bindings[1].Field != "score" {
		t.Fatalf("binding order not preserved: %#v", got.Bindings)
	}
}

// --- program.RandomGenerator ---

func TestRandomGenerator_RoundTrip(t *testing.T) {
	cases := []program.RandomGenerator{
		program.RandomIntegerGenerator{
			Minimum: program.NumberLiteralExpression{Value: "1"},
			Maximum: program.NumberLiteralExpression{Value: "6"},
		},
		program.RandomElementGenerator{
			Collection: program.FieldExpression{Target: program.ReferenceExpression{Name: "global"}, Field: "participants"},
		},
		program.RandomShuffleGenerator{
			Collection: program.FieldExpression{Target: program.ReferenceExpression{Name: "resources"}, Field: "cards"},
		},
	}

	for _, original := range cases {
		raw, err := encodeRandomGenerator("$", original)
		if err != nil {
			t.Fatalf("encode %T: %v", original, err)
		}
		decoded, err := decodeRandomGenerator("$", raw)
		if err != nil {
			t.Fatalf("decode %T: %v", original, err)
		}
		if !reflect.DeepEqual(original, decoded) {
			t.Fatalf("round trip mismatch for %T:\n  original = %#v\n  decoded  = %#v", original, original, decoded)
		}
	}
}

func TestRandomGenerator_ExactJSON_Integer(t *testing.T) {
	original := program.RandomIntegerGenerator{
		Minimum: program.NumberLiteralExpression{Value: "1"},
		Maximum: program.NumberLiteralExpression{Value: "6"},
	}
	raw, err := encodeRandomGenerator("$", original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["kind"] != "random_integer" {
		t.Fatalf("unexpected kind: %#v", obj["kind"])
	}
	minimum, ok := obj["minimum"].(map[string]any)
	if !ok || minimum["kind"] != "number_literal" || minimum["value"] != "1" {
		t.Fatalf("unexpected minimum: %#v", obj["minimum"])
	}
}

// --- Expression: every variant ---

func allExpressionVariants() []program.Expression {
	return []program.Expression{
		program.UnitLiteralExpression{},
		program.BoolLiteralExpression{Value: true},
		program.NumberLiteralExpression{Value: "3.1415"},
		program.StringLiteralExpression{Value: "hello"},
		program.OptionalNoneExpression{ElementType: program.NamedTypeReference{Name: "ParticipantId"}},
		program.OptionalSomeExpression{Value: program.ReferenceExpression{Name: "participant"}},
		program.ListExpression{
			ElementType: program.NamedTypeReference{Name: "ParticipantId"},
			Elements: []program.Expression{
				program.NewTypeExpression{TypeName: "ParticipantId", Value: program.StringLiteralExpression{Value: "p1"}},
			},
		},
		program.MapExpression{
			KeyType:   program.BuiltinTypeReference{Type: program.BuiltinTypeString},
			ValueType: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber},
			Entries: []program.MapEntryExpression{
				{Key: program.StringLiteralExpression{Value: "score"}, Value: program.NumberLiteralExpression{Value: "10"}},
			},
		},
		program.EnumValueExpression{TypeName: "Color", ValueName: "RED"},
		program.RecordExpression{
			TypeName: "Participant",
			Fields: []program.FieldInitializer{
				{Name: "id", Value: program.ReferenceExpression{Name: "participantId"}},
			},
		},
		program.UnionExpression{
			TypeName:    "TokenPosition",
			VariantName: "OnRoute",
			Fields: []program.FieldInitializer{
				{Name: "routeIndex", Value: program.NumberLiteralExpression{Value: "17"}},
			},
		},
		program.NewTypeExpression{TypeName: "ParticipantId", Value: program.StringLiteralExpression{Value: "participant-1"}},
		program.ReferenceExpression{Name: "global"},
		program.FieldExpression{Target: program.ReferenceExpression{Name: "global"}, Field: "participants"},
		program.IndexExpression{
			Target: program.FieldExpression{Target: program.ReferenceExpression{Name: "global"}, Field: "participants"},
			Index:  program.ReferenceExpression{Name: "currentUser"},
		},
		program.UnaryExpression{Operator: program.UnaryOperatorNot, Operand: program.ReferenceExpression{Name: "disabled"}},
		program.BinaryExpression{
			Operator: program.BinaryOperatorGreaterOrEqual,
			Left:     program.ReferenceExpression{Name: "score"},
			Right:    program.NumberLiteralExpression{Value: "10"},
		},
		program.ConditionalExpression{
			Condition: program.ReferenceExpression{Name: "hasWinner"},
			Then:      program.StringLiteralExpression{Value: "Winner"},
			Else:      program.StringLiteralExpression{Value: "No winner"},
		},
		program.CallExpression{
			Function: "distance",
			Arguments: []program.CallArgument{
				{Name: "from", Value: program.ReferenceExpression{Name: "origin"}},
				{Name: "to", Value: program.ReferenceExpression{Name: "destination"}},
			},
		},
		program.MatchExpression{
			Value: program.FieldExpression{Target: program.ReferenceExpression{Name: "token"}, Field: "position"},
			Cases: []program.MatchExpressionCase{
				{
					Pattern: program.UnionVariantMatchPattern{TypeName: "TokenPosition", VariantName: "Jail"},
					Result:  program.OptionalNoneExpression{ElementType: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}},
				},
				{
					Pattern: program.UnionVariantMatchPattern{
						TypeName:    "TokenPosition",
						VariantName: "OnRoute",
						Bindings:    []program.MatchFieldBinding{{Field: "routeIndex", Name: "index"}},
					},
					Result: program.OptionalSomeExpression{Value: program.ReferenceExpression{Name: "index"}},
				},
			},
		},
		program.ListMapExpression{
			Collection: program.ReferenceExpression{Name: "tokens"},
			ItemName:   "token",
			IndexName:  "index",
			Result:     program.FieldExpression{Target: program.ReferenceExpression{Name: "token"}, Field: "id"},
		},
		program.ListFilterExpression{
			Collection: program.ReferenceExpression{Name: "participants"},
			ItemName:   "participant",
			Predicate:  program.FieldExpression{Target: program.ReferenceExpression{Name: "participant"}, Field: "connected"},
		},
		program.ListFlatMapExpression{
			Collection: program.ReferenceExpression{Name: "teams"},
			ItemName:   "team",
			Result:     program.FieldExpression{Target: program.ReferenceExpression{Name: "team"}, Field: "members"},
		},
		program.ListAnyExpression{
			Collection: program.ReferenceExpression{Name: "movePlans"},
			ItemName:   "plan",
			Predicate:  program.FieldExpression{Target: program.ReferenceExpression{Name: "plan"}, Field: "captures"},
		},
		program.ListAllExpression{
			Collection: program.ReferenceExpression{Name: "tokens"},
			ItemName:   "token",
			Predicate:  program.FieldExpression{Target: program.ReferenceExpression{Name: "token"}, Field: "finished"},
		},
		program.ListCountExpression{
			Collection: program.ReferenceExpression{Name: "tokens"},
			ItemName:   "token",
			Predicate:  program.FieldExpression{Target: program.ReferenceExpression{Name: "token"}, Field: "owned"},
		},
		program.ListFirstExpression{
			Collection: program.ReferenceExpression{Name: "participants"},
			ItemName:   "participant",
			Predicate:  program.FieldExpression{Target: program.ReferenceExpression{Name: "participant"}, Field: "isViewer"},
		},
	}
}

func TestExpression_AllVariants_RoundTrip(t *testing.T) {
	for _, original := range allExpressionVariants() {
		t.Run(reflect.TypeOf(original).Name(), func(t *testing.T) {
			raw := mustEncodeExpression(t, original)

			decoded, err := decodeExpression("$", raw)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if reflect.TypeOf(decoded) != reflect.TypeOf(original) {
				t.Fatalf("expected decoded type %T, got %T", original, decoded)
			}
			if !reflect.DeepEqual(original, decoded) {
				t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", original, decoded)
			}

			reencoded, err := encodeExpression("$", decoded)
			if err != nil {
				t.Fatalf("re-encode: %v", err)
			}
			assertSemanticJSONEqual(t, raw, reencoded)
		})
	}
}

func TestExpression_ExactJSON_NumberLiteral(t *testing.T) {
	raw := mustEncodeExpression(t, program.NumberLiteralExpression{Value: "3.1415"})
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["kind"] != "number_literal" {
		t.Fatalf("expected kind number_literal, got %#v", obj["kind"])
	}
	value, ok := obj["value"].(string)
	if !ok || value != "3.1415" {
		t.Fatalf("expected string value \"3.1415\", got %#v (must not be a JSON number)", obj["value"])
	}
}

func TestExpression_ExactJSON_Binary(t *testing.T) {
	raw := mustEncodeExpression(t, program.BinaryExpression{
		Operator: program.BinaryOperatorGreaterOrEqual,
		Left:     program.ReferenceExpression{Name: "score"},
		Right:    program.NumberLiteralExpression{Value: "10"},
	})
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["kind"] != "binary" || obj["operator"] != "greater_or_equal" {
		t.Fatalf("unexpected shape: %#v", obj)
	}
}

func TestExpression_ExactJSON_Match(t *testing.T) {
	raw := mustEncodeExpression(t, program.MatchExpression{
		Value: program.ReferenceExpression{Name: "phase"},
		Cases: []program.MatchExpressionCase{
			{Pattern: program.WildcardMatchPattern{}, Result: program.BoolLiteralExpression{Value: false}},
		},
	})
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["kind"] != "match" {
		t.Fatalf("expected kind match, got %#v", obj["kind"])
	}
	cases, ok := obj["cases"].([]any)
	if !ok || len(cases) != 1 {
		t.Fatalf("expected 1 case, got %#v", obj["cases"])
	}
}

func TestExpression_ExactJSON_ListMap(t *testing.T) {
	raw := mustEncodeExpression(t, program.ListMapExpression{
		Collection: program.ReferenceExpression{Name: "tokens"},
		ItemName:   "token",
		IndexName:  "index",
		Result:     program.FieldExpression{Target: program.ReferenceExpression{Name: "token"}, Field: "id"},
	})
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["kind"] != "list_map" || obj["item_name"] != "token" || obj["index_name"] != "index" {
		t.Fatalf("unexpected shape: %#v", obj)
	}
}

// --- pointer and value equivalence ---

func TestPointerAndValueEquivalence_Expression(t *testing.T) {
	value := program.BinaryExpression{
		Operator: program.BinaryOperatorAdd,
		Left:     program.NumberLiteralExpression{Value: "1"},
		Right:    program.NumberLiteralExpression{Value: "2"},
	}
	valueJSON, err := encodeExpression("$", value)
	if err != nil {
		t.Fatalf("encode value: %v", err)
	}
	pointerJSON, err := encodeExpression("$", &value)
	if err != nil {
		t.Fatalf("encode pointer: %v", err)
	}
	assertSemanticJSONEqual(t, valueJSON, pointerJSON)
}

func TestPointerAndValueEquivalence_TypeReference(t *testing.T) {
	value := program.NamedTypeReference{Name: "ParticipantId"}
	valueJSON, err := encodeTypeReference("$", value)
	if err != nil {
		t.Fatalf("encode value: %v", err)
	}
	pointerJSON, err := encodeTypeReference("$", &value)
	if err != nil {
		t.Fatalf("encode pointer: %v", err)
	}
	assertSemanticJSONEqual(t, valueJSON, pointerJSON)
}

func TestPointerAndValueEquivalence_TypeDeclaration(t *testing.T) {
	value := program.EnumTypeDeclaration{Name: "Color", Values: []program.EnumValueDeclaration{{Name: "RED"}}}
	valueJSON, err := encodeTypeDeclaration("$", value)
	if err != nil {
		t.Fatalf("encode value: %v", err)
	}
	pointerJSON, err := encodeTypeDeclaration("$", &value)
	if err != nil {
		t.Fatalf("encode pointer: %v", err)
	}
	assertSemanticJSONEqual(t, valueJSON, pointerJSON)
}

func TestPointerAndValueEquivalence_MatchPattern(t *testing.T) {
	value := program.EnumValueMatchPattern{TypeName: "MatchPhase", ValueName: "PLAYING"}
	valueJSON, err := encodeMatchPattern("$", value)
	if err != nil {
		t.Fatalf("encode value: %v", err)
	}
	pointerJSON, err := encodeMatchPattern("$", &value)
	if err != nil {
		t.Fatalf("encode pointer: %v", err)
	}
	assertSemanticJSONEqual(t, valueJSON, pointerJSON)
}

func TestPointerAndValueEquivalence_RandomGenerator(t *testing.T) {
	value := program.RandomElementGenerator{Collection: program.ReferenceExpression{Name: "tokens"}}
	valueJSON, err := encodeRandomGenerator("$", value)
	if err != nil {
		t.Fatalf("encode value: %v", err)
	}
	pointerJSON, err := encodeRandomGenerator("$", &value)
	if err != nil {
		t.Fatalf("encode pointer: %v", err)
	}
	assertSemanticJSONEqual(t, valueJSON, pointerJSON)
}

// --- typed nil pointers ---

func TestTypedNilPointer_EncodesToNull(t *testing.T) {
	var expression *program.BinaryExpression
	var source program.Expression = expression

	raw, err := encodeExpression("$", source)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if string(raw) != "null" {
		t.Fatalf("expected JSON null, got %s", raw)
	}
}

func TestNilInterface_EncodesToNull(t *testing.T) {
	raw, err := encodeExpression("$", nil)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if string(raw) != "null" {
		t.Fatalf("expected JSON null, got %s", raw)
	}
}

// --- null decoding ---

func TestJSONNullDecoding_AllFamilies(t *testing.T) {
	typeRef, err := decodeTypeReference("$", json.RawMessage("null"))
	if err != nil || typeRef != nil {
		t.Fatalf("expected nil, nil got %#v, %v", typeRef, err)
	}

	typeDecl, err := decodeTypeDeclaration("$", json.RawMessage("null"))
	if err != nil || typeDecl != nil {
		t.Fatalf("expected nil, nil got %#v, %v", typeDecl, err)
	}

	expr, err := decodeExpression("$", json.RawMessage("null"))
	if err != nil || expr != nil {
		t.Fatalf("expected nil, nil got %#v, %v", expr, err)
	}

	pattern, err := decodeMatchPattern("$", json.RawMessage("null"))
	if err != nil || pattern != nil {
		t.Fatalf("expected nil, nil got %#v, %v", pattern, err)
	}

	generator, err := decodeRandomGenerator("$", json.RawMessage("null"))
	if err != nil || generator != nil {
		t.Fatalf("expected nil, nil got %#v, %v", generator, err)
	}
}

// --- error handling ---

func TestDecode_UnknownDiscriminator_Expression(t *testing.T) {
	_, err := decodeExpression("$", json.RawMessage(`{"kind":"unknown_expression"}`))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$" {
		t.Fatalf("expected path $, got %q", decodeErr.Path)
	}
	if decodeErr.Message == "" {
		t.Fatal("expected a non-empty message identifying the unsupported kind")
	}
}

func TestDecode_UnknownDiscriminator_TypeReference(t *testing.T) {
	_, err := decodeTypeReference("$", json.RawMessage(`{"kind":"unknown_type"}`))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$" {
		t.Fatalf("expected path $, got %q", decodeErr.Path)
	}
}

func TestDecode_MissingDiscriminator(t *testing.T) {
	_, err := decodeExpression("$", json.RawMessage(`{"value":true}`))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$" {
		t.Fatalf("expected path $, got %q", decodeErr.Path)
	}
}

func TestDecode_UnknownField_Rejected(t *testing.T) {
	_, err := decodeExpression("$", json.RawMessage(`{"kind":"string_literal","value":"hello","unexpected":true}`))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

func TestDecode_NestedPathError_Field(t *testing.T) {
	data := json.RawMessage(`{
		"kind": "binary",
		"operator": "add",
		"left": {"kind": "number_literal", "value": "1"},
		"right": {"kind": "unknown_expression"}
	}`)
	_, err := decodeExpression("$", data)
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$.right" {
		t.Fatalf("expected path $.right, got %q", decodeErr.Path)
	}
}

func TestDecode_NestedPathError_ArrayIndex_Cases(t *testing.T) {
	data := json.RawMessage(`{
		"kind": "match",
		"value": {"kind": "reference", "name": "phase"},
		"cases": [
			{
				"pattern": {"kind": "wildcard"},
				"result": {"kind": "bool_literal", "value": true}
			},
			{
				"pattern": {"kind": "unknown_pattern"},
				"result": {"kind": "bool_literal", "value": false}
			}
		]
	}`)
	_, err := decodeExpression("$", data)
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$.cases[1].pattern" {
		t.Fatalf("expected path $.cases[1].pattern, got %q", decodeErr.Path)
	}
}

func TestDecode_NestedPathError_ArrayIndex_Entries(t *testing.T) {
	data := json.RawMessage(`{
		"kind": "map",
		"key_type": {"kind": "builtin", "type": "string"},
		"value_type": {"kind": "builtin", "type": "number"},
		"entries": [
			{
				"key": {"kind": "string_literal", "value": "score"},
				"value": {"kind": "unknown_expression"}
			}
		]
	}`)
	_, err := decodeExpression("$", data)
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$.entries[0].value" {
		t.Fatalf("expected path $.entries[0].value, got %q", decodeErr.Path)
	}
}

func TestDecode_TrailingJSON_Rejected(t *testing.T) {
	data := json.RawMessage(`{"kind":"unit_literal"} {"kind":"unit_literal"}`)
	_, err := decodeExpression("$", data)
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

// --- semantic invalidity remains representable ---

func TestDecode_SemanticInvalidity_UnknownBuiltinType(t *testing.T) {
	decoded, err := decodeTypeReference("$", json.RawMessage(`{"kind":"builtin","type":"not_a_real_type"}`))
	if err != nil {
		t.Fatalf("expected structural success, got error: %v", err)
	}
	builtin, ok := decoded.(program.BuiltinTypeReference)
	if !ok {
		t.Fatalf("expected program.BuiltinTypeReference, got %T", decoded)
	}
	if builtin.Type != program.BuiltinType("not_a_real_type") {
		t.Fatalf("expected unknown built-in string preserved, got %q", builtin.Type)
	}
}

func TestDecode_SemanticInvalidity_UnknownOperators(t *testing.T) {
	decoded, err := decodeExpression("$", json.RawMessage(`{
		"kind": "unary",
		"operator": "not_a_real_operator",
		"operand": {"kind": "bool_literal", "value": true}
	}`))
	if err != nil {
		t.Fatalf("expected structural success, got error: %v", err)
	}
	unary, ok := decoded.(program.UnaryExpression)
	if !ok {
		t.Fatalf("expected program.UnaryExpression, got %T", decoded)
	}
	if unary.Operator != program.UnaryOperator("not_a_real_operator") {
		t.Fatalf("expected unknown operator preserved, got %q", unary.Operator)
	}

	decodedBinary, err := decodeExpression("$", json.RawMessage(`{
		"kind": "binary",
		"operator": "not_a_real_operator",
		"left": {"kind": "number_literal", "value": "1"},
		"right": {"kind": "number_literal", "value": "2"}
	}`))
	if err != nil {
		t.Fatalf("expected structural success, got error: %v", err)
	}
	binary, ok := decodedBinary.(program.BinaryExpression)
	if !ok {
		t.Fatalf("expected program.BinaryExpression, got %T", decodedBinary)
	}
	if binary.Operator != program.BinaryOperator("not_a_real_operator") {
		t.Fatalf("expected unknown operator preserved, got %q", binary.Operator)
	}
}

func TestDecode_SemanticInvalidity_DuplicateRecordFields(t *testing.T) {
	decoded, err := decodeTypeDeclaration("$", json.RawMessage(`{
		"kind": "record",
		"name": "Participant",
		"fields": [
			{"name": "id", "type": {"kind": "builtin", "type": "string"}},
			{"name": "id", "type": {"kind": "builtin", "type": "number"}}
		]
	}`))
	if err != nil {
		t.Fatalf("expected structural success, got error: %v", err)
	}
	record, ok := decoded.(program.RecordTypeDeclaration)
	if !ok {
		t.Fatalf("expected program.RecordTypeDeclaration, got %T", decoded)
	}
	if len(record.Fields) != 2 {
		t.Fatalf("expected both duplicate fields preserved, got %d", len(record.Fields))
	}
}

func TestDecode_SemanticInvalidity_DuplicateMapEntries(t *testing.T) {
	decoded, err := decodeExpression("$", json.RawMessage(`{
		"kind": "map",
		"key_type": null,
		"value_type": null,
		"entries": [
			{"key": {"kind": "string_literal", "value": "a"}, "value": {"kind": "number_literal", "value": "1"}},
			{"key": {"kind": "string_literal", "value": "a"}, "value": {"kind": "number_literal", "value": "2"}}
		]
	}`))
	if err != nil {
		t.Fatalf("expected structural success, got error: %v", err)
	}
	mapExpr, ok := decoded.(program.MapExpression)
	if !ok {
		t.Fatalf("expected program.MapExpression, got %T", decoded)
	}
	if len(mapExpr.Entries) != 2 {
		t.Fatalf("expected both duplicate entries preserved, got %d", len(mapExpr.Entries))
	}
}

func TestDecode_SemanticInvalidity_EmptyTypeName(t *testing.T) {
	decoded, err := decodeTypeDeclaration("$", json.RawMessage(`{"kind":"enum","name":"","values":[]}`))
	if err != nil {
		t.Fatalf("expected structural success, got error: %v", err)
	}
	enum, ok := decoded.(program.EnumTypeDeclaration)
	if !ok {
		t.Fatalf("expected program.EnumTypeDeclaration, got %T", decoded)
	}
	if enum.Name != "" {
		t.Fatalf("expected empty name preserved, got %q", enum.Name)
	}
}

// --- nil vs empty slices ---

func TestNilVsEmptySlices_EnumValues(t *testing.T) {
	nilCase := program.EnumTypeDeclaration{Name: "Empty", Values: nil}
	raw, err := encodeTypeDeclaration("$", nilCase)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["values"] != nil {
		t.Fatalf("expected null values for nil slice, got %#v", obj["values"])
	}

	emptyCase := program.EnumTypeDeclaration{Name: "Empty", Values: []program.EnumValueDeclaration{}}
	raw, err = encodeTypeDeclaration("$", emptyCase)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	obj = nil
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	values, ok := obj["values"].([]any)
	if !ok || len(values) != 0 {
		t.Fatalf("expected empty array for empty slice, got %#v", obj["values"])
	}

	decodedNil, err := decodeTypeDeclaration("$", mustEncode(t, nilCase))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := decodedNil.(program.EnumTypeDeclaration).Values; got != nil {
		t.Fatalf("expected nil slice preserved through round trip, got %#v", got)
	}

	decodedEmpty, err := decodeTypeDeclaration("$", mustEncode(t, emptyCase))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := decodedEmpty.(program.EnumTypeDeclaration).Values
	if got == nil || len(got) != 0 {
		t.Fatalf("expected empty non-nil slice preserved through round trip, got %#v", got)
	}
}

func mustEncode(t *testing.T, value program.TypeDeclaration) json.RawMessage {
	t.Helper()
	raw, err := encodeTypeDeclaration("$", value)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return raw
}

// --- duplicate ordering ---

func TestDuplicateOrdering_CallArguments(t *testing.T) {
	original := program.CallExpression{
		Function: "sum",
		Arguments: []program.CallArgument{
			{Name: "value", Value: program.NumberLiteralExpression{Value: "1"}},
			{Name: "value", Value: program.NumberLiteralExpression{Value: "2"}},
		},
	}
	raw := mustEncodeExpression(t, original)
	decoded, err := decodeExpression("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	call, ok := decoded.(program.CallExpression)
	if !ok {
		t.Fatalf("expected program.CallExpression, got %T", decoded)
	}
	if len(call.Arguments) != 2 {
		t.Fatalf("expected both duplicate arguments preserved, got %d", len(call.Arguments))
	}
	if call.Arguments[0].Value.(program.NumberLiteralExpression).Value != "1" || call.Arguments[1].Value.(program.NumberLiteralExpression).Value != "2" {
		t.Fatalf("argument order not preserved: %#v", call.Arguments)
	}
}
