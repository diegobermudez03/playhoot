package program

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

// --- generic helpers ---

func mustEncodeExpression(t *testing.T, value Expression) json.RawMessage {
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

// --- TypeReference ---

func TestTypeReference_RoundTrip(t *testing.T) {
	original := OptionalTypeReference{
		Element: MapTypeReference{
			Key: NamedTypeReference{Name: "ParticipantId"},
			Value: ListTypeReference{
				Element: NamedTypeReference{Name: "TokenId"},
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
	cases := []TypeReference{
		BuiltinTypeReference{Type: BuiltinTypeNumber},
		NamedTypeReference{Name: "ParticipantId"},
		ListTypeReference{Element: BuiltinTypeReference{Type: BuiltinTypeString}},
		MapTypeReference{Key: BuiltinTypeReference{Type: BuiltinTypeString}, Value: BuiltinTypeReference{Type: BuiltinTypeNumber}},
		OptionalTypeReference{Element: NamedTypeReference{Name: "ParticipantId"}},
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
	raw, err := encodeTypeReference("$", BuiltinTypeReference{Type: BuiltinTypeNumber})
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

// --- TypeDeclaration ---

func TestTypeDeclaration_RoundTrip(t *testing.T) {
	cases := []TypeDeclaration{
		EnumTypeDeclaration{
			Name: "Color",
			Values: []EnumValueDeclaration{
				{Name: "RED"},
				{Name: "BLUE"},
			},
		},
		RecordTypeDeclaration{
			Name: "Participant",
			Fields: []FieldDeclaration{
				{Name: "id", Type: NamedTypeReference{Name: "ParticipantId"}},
				{Name: "user", Type: BuiltinTypeReference{Type: BuiltinTypeUser}},
			},
		},
		UnionTypeDeclaration{
			Name: "TokenPosition",
			Variants: []UnionVariantDeclaration{
				{Name: "Jail", Fields: []FieldDeclaration{}},
				{Name: "OnRoute", Fields: []FieldDeclaration{
					{Name: "routeIndex", Type: BuiltinTypeReference{Type: BuiltinTypeNumber}},
				}},
				{Name: "Finished", Fields: []FieldDeclaration{}},
			},
		},
		NewTypeDeclaration{
			Name:       "ParticipantId",
			Underlying: BuiltinTypeReference{Type: BuiltinTypeString},
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
	original := UnionTypeDeclaration{
		Name: "TokenPosition",
		Variants: []UnionVariantDeclaration{
			{Name: "Jail", Fields: []FieldDeclaration{}},
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

// --- MatchPattern ---

func TestMatchPattern_RoundTrip(t *testing.T) {
	cases := []MatchPattern{
		WildcardMatchPattern{},
		EnumValueMatchPattern{TypeName: "MatchPhase", ValueName: "PLAYING"},
		UnionVariantMatchPattern{
			TypeName:    "TokenPosition",
			VariantName: "OnRoute",
			Bindings: []MatchFieldBinding{
				{Field: "routeIndex", Name: "index"},
			},
		},
		UnionVariantMatchPattern{TypeName: "TokenPosition", VariantName: "Jail"},
		OptionalNoneMatchPattern{},
		OptionalSomeMatchPattern{Binding: "participant"},
		OptionalSomeMatchPattern{},
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
	original := UnionVariantMatchPattern{
		TypeName:    "TurnResult",
		VariantName: "Won",
		Bindings: []MatchFieldBinding{
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
	got, ok := decoded.(UnionVariantMatchPattern)
	if !ok {
		t.Fatalf("expected UnionVariantMatchPattern, got %T", decoded)
	}
	if len(got.Bindings) != 2 || got.Bindings[0].Field != "participant" || got.Bindings[1].Field != "score" {
		t.Fatalf("binding order not preserved: %#v", got.Bindings)
	}
}

// --- RandomGenerator ---

func TestRandomGenerator_RoundTrip(t *testing.T) {
	cases := []RandomGenerator{
		RandomIntegerGenerator{
			Minimum: NumberLiteralExpression{Value: "1"},
			Maximum: NumberLiteralExpression{Value: "6"},
		},
		RandomElementGenerator{
			Collection: FieldExpression{Target: ReferenceExpression{Name: "global"}, Field: "participants"},
		},
		RandomShuffleGenerator{
			Collection: FieldExpression{Target: ReferenceExpression{Name: "resources"}, Field: "cards"},
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
	original := RandomIntegerGenerator{
		Minimum: NumberLiteralExpression{Value: "1"},
		Maximum: NumberLiteralExpression{Value: "6"},
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

func allExpressionVariants() []Expression {
	return []Expression{
		UnitLiteralExpression{},
		BoolLiteralExpression{Value: true},
		NumberLiteralExpression{Value: "3.1415"},
		StringLiteralExpression{Value: "hello"},
		OptionalNoneExpression{ElementType: NamedTypeReference{Name: "ParticipantId"}},
		OptionalSomeExpression{Value: ReferenceExpression{Name: "participant"}},
		ListExpression{
			ElementType: NamedTypeReference{Name: "ParticipantId"},
			Elements: []Expression{
				NewTypeExpression{TypeName: "ParticipantId", Value: StringLiteralExpression{Value: "p1"}},
			},
		},
		MapExpression{
			KeyType:   BuiltinTypeReference{Type: BuiltinTypeString},
			ValueType: BuiltinTypeReference{Type: BuiltinTypeNumber},
			Entries: []MapEntryExpression{
				{Key: StringLiteralExpression{Value: "score"}, Value: NumberLiteralExpression{Value: "10"}},
			},
		},
		EnumValueExpression{TypeName: "Color", ValueName: "RED"},
		RecordExpression{
			TypeName: "Participant",
			Fields: []FieldInitializer{
				{Name: "id", Value: ReferenceExpression{Name: "participantId"}},
			},
		},
		UnionExpression{
			TypeName:    "TokenPosition",
			VariantName: "OnRoute",
			Fields: []FieldInitializer{
				{Name: "routeIndex", Value: NumberLiteralExpression{Value: "17"}},
			},
		},
		NewTypeExpression{TypeName: "ParticipantId", Value: StringLiteralExpression{Value: "participant-1"}},
		ReferenceExpression{Name: "global"},
		FieldExpression{Target: ReferenceExpression{Name: "global"}, Field: "participants"},
		IndexExpression{
			Target: FieldExpression{Target: ReferenceExpression{Name: "global"}, Field: "participants"},
			Index:  ReferenceExpression{Name: "currentUser"},
		},
		UnaryExpression{Operator: UnaryOperatorNot, Operand: ReferenceExpression{Name: "disabled"}},
		BinaryExpression{
			Operator: BinaryOperatorGreaterOrEqual,
			Left:     ReferenceExpression{Name: "score"},
			Right:    NumberLiteralExpression{Value: "10"},
		},
		ConditionalExpression{
			Condition: ReferenceExpression{Name: "hasWinner"},
			Then:      StringLiteralExpression{Value: "Winner"},
			Else:      StringLiteralExpression{Value: "No winner"},
		},
		CallExpression{
			Function: "distance",
			Arguments: []CallArgument{
				{Name: "from", Value: ReferenceExpression{Name: "origin"}},
				{Name: "to", Value: ReferenceExpression{Name: "destination"}},
			},
		},
		MatchExpression{
			Value: FieldExpression{Target: ReferenceExpression{Name: "token"}, Field: "position"},
			Cases: []MatchExpressionCase{
				{
					Pattern: UnionVariantMatchPattern{TypeName: "TokenPosition", VariantName: "Jail"},
					Result:  OptionalNoneExpression{ElementType: BuiltinTypeReference{Type: BuiltinTypeNumber}},
				},
				{
					Pattern: UnionVariantMatchPattern{
						TypeName:    "TokenPosition",
						VariantName: "OnRoute",
						Bindings:    []MatchFieldBinding{{Field: "routeIndex", Name: "index"}},
					},
					Result: OptionalSomeExpression{Value: ReferenceExpression{Name: "index"}},
				},
			},
		},
		ListMapExpression{
			Collection: ReferenceExpression{Name: "tokens"},
			ItemName:   "token",
			IndexName:  "index",
			Result:     FieldExpression{Target: ReferenceExpression{Name: "token"}, Field: "id"},
		},
		ListFilterExpression{
			Collection: ReferenceExpression{Name: "participants"},
			ItemName:   "participant",
			Predicate:  FieldExpression{Target: ReferenceExpression{Name: "participant"}, Field: "connected"},
		},
		ListFlatMapExpression{
			Collection: ReferenceExpression{Name: "teams"},
			ItemName:   "team",
			Result:     FieldExpression{Target: ReferenceExpression{Name: "team"}, Field: "members"},
		},
		ListAnyExpression{
			Collection: ReferenceExpression{Name: "movePlans"},
			ItemName:   "plan",
			Predicate:  FieldExpression{Target: ReferenceExpression{Name: "plan"}, Field: "captures"},
		},
		ListAllExpression{
			Collection: ReferenceExpression{Name: "tokens"},
			ItemName:   "token",
			Predicate:  FieldExpression{Target: ReferenceExpression{Name: "token"}, Field: "finished"},
		},
		ListCountExpression{
			Collection: ReferenceExpression{Name: "tokens"},
			ItemName:   "token",
			Predicate:  FieldExpression{Target: ReferenceExpression{Name: "token"}, Field: "owned"},
		},
		ListFirstExpression{
			Collection: ReferenceExpression{Name: "participants"},
			ItemName:   "participant",
			Predicate:  FieldExpression{Target: ReferenceExpression{Name: "participant"}, Field: "isViewer"},
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
	raw := mustEncodeExpression(t, NumberLiteralExpression{Value: "3.1415"})
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
	raw := mustEncodeExpression(t, BinaryExpression{
		Operator: BinaryOperatorGreaterOrEqual,
		Left:     ReferenceExpression{Name: "score"},
		Right:    NumberLiteralExpression{Value: "10"},
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
	raw := mustEncodeExpression(t, MatchExpression{
		Value: ReferenceExpression{Name: "phase"},
		Cases: []MatchExpressionCase{
			{Pattern: WildcardMatchPattern{}, Result: BoolLiteralExpression{Value: false}},
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
	raw := mustEncodeExpression(t, ListMapExpression{
		Collection: ReferenceExpression{Name: "tokens"},
		ItemName:   "token",
		IndexName:  "index",
		Result:     FieldExpression{Target: ReferenceExpression{Name: "token"}, Field: "id"},
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
	value := BinaryExpression{
		Operator: BinaryOperatorAdd,
		Left:     NumberLiteralExpression{Value: "1"},
		Right:    NumberLiteralExpression{Value: "2"},
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
	value := NamedTypeReference{Name: "ParticipantId"}
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
	value := EnumTypeDeclaration{Name: "Color", Values: []EnumValueDeclaration{{Name: "RED"}}}
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
	value := EnumValueMatchPattern{TypeName: "MatchPhase", ValueName: "PLAYING"}
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
	value := RandomElementGenerator{Collection: ReferenceExpression{Name: "tokens"}}
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
	var expression *BinaryExpression
	var source Expression = expression

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
	builtin, ok := decoded.(BuiltinTypeReference)
	if !ok {
		t.Fatalf("expected BuiltinTypeReference, got %T", decoded)
	}
	if builtin.Type != BuiltinType("not_a_real_type") {
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
	unary, ok := decoded.(UnaryExpression)
	if !ok {
		t.Fatalf("expected UnaryExpression, got %T", decoded)
	}
	if unary.Operator != UnaryOperator("not_a_real_operator") {
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
	binary, ok := decodedBinary.(BinaryExpression)
	if !ok {
		t.Fatalf("expected BinaryExpression, got %T", decodedBinary)
	}
	if binary.Operator != BinaryOperator("not_a_real_operator") {
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
	record, ok := decoded.(RecordTypeDeclaration)
	if !ok {
		t.Fatalf("expected RecordTypeDeclaration, got %T", decoded)
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
	mapExpr, ok := decoded.(MapExpression)
	if !ok {
		t.Fatalf("expected MapExpression, got %T", decoded)
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
	enum, ok := decoded.(EnumTypeDeclaration)
	if !ok {
		t.Fatalf("expected EnumTypeDeclaration, got %T", decoded)
	}
	if enum.Name != "" {
		t.Fatalf("expected empty name preserved, got %q", enum.Name)
	}
}

// --- nil vs empty slices ---

func TestNilVsEmptySlices_EnumValues(t *testing.T) {
	nilCase := EnumTypeDeclaration{Name: "Empty", Values: nil}
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

	emptyCase := EnumTypeDeclaration{Name: "Empty", Values: []EnumValueDeclaration{}}
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
	if got := decodedNil.(EnumTypeDeclaration).Values; got != nil {
		t.Fatalf("expected nil slice preserved through round trip, got %#v", got)
	}

	decodedEmpty, err := decodeTypeDeclaration("$", mustEncode(t, emptyCase))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := decodedEmpty.(EnumTypeDeclaration).Values
	if got == nil || len(got) != 0 {
		t.Fatalf("expected empty non-nil slice preserved through round trip, got %#v", got)
	}
}

func mustEncode(t *testing.T, value TypeDeclaration) json.RawMessage {
	t.Helper()
	raw, err := encodeTypeDeclaration("$", value)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return raw
}

// --- duplicate ordering ---

func TestDuplicateOrdering_CallArguments(t *testing.T) {
	original := CallExpression{
		Function: "sum",
		Arguments: []CallArgument{
			{Name: "value", Value: NumberLiteralExpression{Value: "1"}},
			{Name: "value", Value: NumberLiteralExpression{Value: "2"}},
		},
	}
	raw := mustEncodeExpression(t, original)
	decoded, err := decodeExpression("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	call, ok := decoded.(CallExpression)
	if !ok {
		t.Fatalf("expected CallExpression, got %T", decoded)
	}
	if len(call.Arguments) != 2 {
		t.Fatalf("expected both duplicate arguments preserved, got %d", len(call.Arguments))
	}
	if call.Arguments[0].Value.(NumberLiteralExpression).Value != "1" || call.Arguments[1].Value.(NumberLiteralExpression).Value != "2" {
		t.Fatalf("argument order not preserved: %#v", call.Arguments)
	}
}
