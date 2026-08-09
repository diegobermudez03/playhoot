package codec

import "github.com/diegobermudez03/playhoot/game/program"

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

// --- metadata ---

func TestMetadata_RoundTrip(t *testing.T) {
	original := program.Metadata{
		ID:              "parques",
		Name:            "Parques",
		Description:     "A Colombian board game",
		Version:         "1.0.0",
		LanguageVersion: "1",
	}
	raw, err := encodeMetadata("$", original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeMetadata("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded != original {
		t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", original, decoded)
	}
}

func TestMetadata_ZeroValue_PreservesEmptyStrings(t *testing.T) {
	raw, err := encodeMetadata("$", program.Metadata{})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, field := range []string{"id", "name", "description", "version", "language_version"} {
		if obj[field] != "" {
			t.Fatalf("expected field %q to be empty string, got %#v", field, obj[field])
		}
	}
	decoded, err := decodeMetadata("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded != (program.Metadata{}) {
		t.Fatalf("expected zero-value metadata preserved, got %#v", decoded)
	}
}

func TestMetadata_DecodeNull_IsStructuralError(t *testing.T) {
	_, err := decodeMetadata("$", json.RawMessage("null"))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

func TestMetadata_UnknownField_Rejected(t *testing.T) {
	_, err := decodeMetadata("$", json.RawMessage(`{"id":"x","name":"","description":"","version":"","language_version":"","extra":true}`))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

func TestExactJSON_Metadata(t *testing.T) {
	raw, err := encodeMetadata("$", program.Metadata{ID: "parques", Name: "Parques"})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["id"] != "parques" || obj["name"] != "Parques" {
		t.Fatalf("unexpected shape: %#v", obj)
	}
}

// --- shared field declaration ---

func TestFieldDeclarations_RoundTrip(t *testing.T) {
	original := []program.FieldDeclaration{
		{Name: "participant", Type: program.NamedTypeReference{Name: "ParticipantId"}},
		{Name: "user", Type: program.BuiltinTypeReference{Type: program.BuiltinTypeUser}},
		{Name: "cards", Type: program.ListTypeReference{Element: program.NamedTypeReference{Name: "CardRef"}}},
	}
	wire, err := encodeFieldDeclarations("$.fields", original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeFieldDeclarations("$.fields", wire)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", original, decoded)
	}
}

// --- resource declarations ---

func TestResourceDeclaration_RoundTrip_Variants(t *testing.T) {
	cases := []program.ResourceDeclaration{
		{Name: "WinningScore", Type: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}, Value: program.NumberLiteralExpression{Value: "10"}},
		{
			Name: "AvailableColors",
			Type: program.ListTypeReference{Element: program.NamedTypeReference{Name: "Color"}},
			Value: program.ListExpression{
				ElementType: program.NamedTypeReference{Name: "Color"},
				Elements:    []program.Expression{},
			},
		},
		{
			Name: "Rules",
			Type: program.NamedTypeReference{Name: "RulesDefinition"},
			Value: program.RecordExpression{
				TypeName: "RulesDefinition",
				Fields:   []program.FieldInitializer{{Name: "piecesPerParticipant", Value: program.NumberLiteralExpression{Value: "4"}}},
			},
		},
		{
			Name: "ScoreTable",
			Type: program.MapTypeReference{Key: program.BuiltinTypeReference{Type: program.BuiltinTypeString}, Value: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}},
			Value: program.MapExpression{
				Entries: []program.MapEntryExpression{
					{Key: program.StringLiteralExpression{Value: "a"}, Value: program.NumberLiteralExpression{Value: "1"}},
					{Key: program.StringLiteralExpression{Value: "a"}, Value: program.NumberLiteralExpression{Value: "2"}},
				},
			},
		},
		{Name: "Nil", Type: nil, Value: nil},
	}

	for i, original := range cases {
		raw, err := encodeResourceDeclaration("$", original)
		if err != nil {
			t.Fatalf("case %d: encode: %v", i, err)
		}
		decoded, err := decodeResourceDeclaration("$", raw)
		if err != nil {
			t.Fatalf("case %d: decode: %v", i, err)
		}
		if !reflect.DeepEqual(original, decoded) {
			t.Fatalf("case %d: round trip mismatch:\n  original = %#v\n  decoded  = %#v", i, original, decoded)
		}
	}
}

func TestResourceDeclarations_OrderPreserved(t *testing.T) {
	original := []program.ResourceDeclaration{
		{Name: "A", Type: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}, Value: program.NumberLiteralExpression{Value: "1"}},
		{Name: "B", Type: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}, Value: program.NumberLiteralExpression{Value: "2"}},
	}
	raw, err := encodeResourceDeclarations("$.resources", original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeResourceDeclarations("$.resources", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(decoded) != 2 || decoded[0].Name != "A" || decoded[1].Name != "B" {
		t.Fatalf("order not preserved: %#v", decoded)
	}
}

func TestExactJSON_ResourceDeclaration(t *testing.T) {
	raw, err := encodeResourceDeclaration("$", program.ResourceDeclaration{
		Name:  "WinningScore",
		Type:  program.BuiltinTypeReference{Type: program.BuiltinTypeNumber},
		Value: program.NumberLiteralExpression{Value: "10"},
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["name"] != "WinningScore" {
		t.Fatalf("unexpected shape: %#v", obj)
	}
}

func TestDecode_UnknownField_ResourceDeclaration(t *testing.T) {
	_, err := decodeResourceDeclaration("$", json.RawMessage(`{"name":"x","type":null,"value":null,"extra":true}`))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

func TestDecode_NestedPathFailure_ResourceDeclarationValue(t *testing.T) {
	data := json.RawMessage(`{"name":"x","type":{"kind":"builtin","type":"number"},"value":{"kind":"not_a_real_expression"}}`)
	_, err := decodeResourceDeclaration("$", data)
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$.value" {
		t.Fatalf("expected path $.value, got %q", decodeErr.Path)
	}
}

func TestDecode_ResourceDeclaration_Null(t *testing.T) {
	_, err := decodeResourceDeclaration("$", json.RawMessage("null"))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

// --- state declarations ---

func TestStateDeclaration_RoundTrip_NilVsEmptyFields(t *testing.T) {
	nilFields := program.StateDeclaration{Fields: nil}
	raw, err := encodeStateDeclaration("$", nilFields)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["fields"] != nil {
		t.Fatalf("expected null fields, got %#v", obj["fields"])
	}
	decoded, err := decodeStateDeclaration("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Fields != nil {
		t.Fatalf("expected nil fields preserved, got %#v", decoded.Fields)
	}

	emptyFields := program.StateDeclaration{Fields: []program.StateFieldDeclaration{}}
	raw, err = encodeStateDeclaration("$", emptyFields)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err = decodeStateDeclaration("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Fields == nil || len(decoded.Fields) != 0 {
		t.Fatalf("expected empty non-nil fields preserved, got %#v", decoded.Fields)
	}
}

func TestStateDeclaration_RoundTrip_Fields(t *testing.T) {
	original := program.StateDeclaration{
		Fields: []program.StateFieldDeclaration{
			{
				Name:        "turnNumber",
				Type:        program.BuiltinTypeReference{Type: program.BuiltinTypeNumber},
				Initializer: program.NumberLiteralExpression{Value: "0"},
			},
			{
				Name:        "winner",
				Type:        program.OptionalTypeReference{Element: program.NamedTypeReference{Name: "ParticipantId"}},
				Initializer: program.OptionalNoneExpression{ElementType: program.NamedTypeReference{Name: "ParticipantId"}},
			},
			{
				Name: "tokens",
				Type: program.MapTypeReference{Key: program.NamedTypeReference{Name: "TokenId"}, Value: program.NamedTypeReference{Name: "Token"}},
				Initializer: program.MapExpression{
					KeyType:   program.NamedTypeReference{Name: "TokenId"},
					ValueType: program.NamedTypeReference{Name: "Token"},
				},
			},
			// Duplicate field name — structurally valid, semantically the
			// future compiler's concern.
			{
				Name:        "turnNumber",
				Type:        nil,
				Initializer: nil,
			},
		},
	}
	raw, err := encodeStateDeclaration("$", original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeStateDeclaration("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", original, decoded)
	}
	if len(decoded.Fields) != 4 {
		t.Fatalf("expected duplicate field name preserved, got %d fields", len(decoded.Fields))
	}
}

func TestDecode_StateDeclaration_Null(t *testing.T) {
	_, err := decodeStateDeclaration("$", json.RawMessage("null"))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

func TestDecode_NestedPathFailure_StateFieldInitializer(t *testing.T) {
	data := json.RawMessage(`{
		"fields": [
			{"name": "a", "type": {"kind":"builtin","type":"number"}, "initializer": {"kind": "not_a_real_expression"}}
		]
	}`)
	_, err := decodeStateDeclaration("$", data)
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$.fields[0].initializer" {
		t.Fatalf("expected path $.fields[0].initializer, got %q", decodeErr.Path)
	}
}

func TestDecode_NestedPathFailure_StateFieldType(t *testing.T) {
	data := json.RawMessage(`{
		"fields": [
			{"name": "a", "type": {"kind":"builtin","type":"number"}, "initializer": {"kind":"number_literal","value":"0"}},
			{"name": "b", "type": {"kind": "not_a_real_type"}, "initializer": null}
		]
	}`)
	_, err := decodeStateDeclaration("$", data)
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$.fields[1].type" {
		t.Fatalf("expected path $.fields[1].type, got %q", decodeErr.Path)
	}
}

func TestExactJSON_StateDeclaration(t *testing.T) {
	raw, err := encodeStateDeclaration("$", program.StateDeclaration{
		Fields: []program.StateFieldDeclaration{
			{Name: "turnNumber", Type: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}, Initializer: program.NumberLiteralExpression{Value: "0"}},
		},
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	fields, ok := obj["fields"].([]any)
	if !ok || len(fields) != 1 {
		t.Fatalf("unexpected shape: %#v", obj)
	}
}

// --- function declarations ---

func TestFunctionDeclaration_RoundTrip_Variants(t *testing.T) {
	cases := []program.FunctionDeclaration{
		{
			Name:       "IsPair",
			Parameters: []program.FieldDeclaration{{Name: "roll", Type: program.NamedTypeReference{Name: "DiceRoll"}}},
			ResultType: program.BuiltinTypeReference{Type: program.BuiltinTypeBool},
			Body: program.BinaryExpression{
				Operator: program.BinaryOperatorEqual,
				Left:     program.FieldExpression{Target: program.ReferenceExpression{Name: "roll"}, Field: "first"},
				Right:    program.FieldExpression{Target: program.ReferenceExpression{Name: "roll"}, Field: "second"},
			},
		},
		{
			Name: "IsFullReleasePair",
			Body: program.BinaryExpression{
				Operator: program.BinaryOperatorAnd,
				Left:     program.CallExpression{Function: "IsPair", Arguments: []program.CallArgument{{Name: "roll", Value: program.ReferenceExpression{Name: "roll"}}}},
				Right:    program.BoolLiteralExpression{Value: true},
			},
			ResultType: program.BuiltinTypeReference{Type: program.BuiltinTypeBool},
		},
		{
			Name:       "OwnedFinishedTokens",
			Parameters: []program.FieldDeclaration{{Name: "tokens", Type: program.ListTypeReference{Element: program.NamedTypeReference{Name: "Token"}}}},
			ResultType: program.BuiltinTypeReference{Type: program.BuiltinTypeBool},
			Body: program.ListAllExpression{
				Collection: program.ReferenceExpression{Name: "tokens"},
				ItemName:   "token",
				Predicate:  program.FieldExpression{Target: program.ReferenceExpression{Name: "token"}, Field: "finished"},
			},
		},
		{
			Name: "RouteIndex",
			Body: program.MatchExpression{
				Value: program.ReferenceExpression{Name: "position"},
				Cases: []program.MatchExpressionCase{
					{Pattern: program.WildcardMatchPattern{}, Result: program.OptionalNoneExpression{ElementType: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}}},
				},
			},
			ResultType: program.OptionalTypeReference{Element: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}},
		},
		{
			Name: "DuplicateParams",
			Parameters: []program.FieldDeclaration{
				{Name: "x", Type: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}},
				{Name: "x", Type: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}},
			},
			ResultType: nil,
			Body:       nil,
		},
	}

	for i, original := range cases {
		raw, err := encodeFunctionDeclaration("$", original)
		if err != nil {
			t.Fatalf("case %d: encode: %v", i, err)
		}
		decoded, err := decodeFunctionDeclaration("$", raw)
		if err != nil {
			t.Fatalf("case %d: decode: %v", i, err)
		}
		if !reflect.DeepEqual(original, decoded) {
			t.Fatalf("case %d: round trip mismatch:\n  original = %#v\n  decoded  = %#v", i, original, decoded)
		}
	}
}

func TestExactJSON_FunctionDeclaration(t *testing.T) {
	raw, err := encodeFunctionDeclaration("$", program.FunctionDeclaration{
		Name:       "IsPair",
		Parameters: []program.FieldDeclaration{{Name: "roll", Type: program.NamedTypeReference{Name: "DiceRoll"}}},
		ResultType: program.BuiltinTypeReference{Type: program.BuiltinTypeBool},
		Body: program.BinaryExpression{
			Operator: program.BinaryOperatorEqual,
			Left:     program.FieldExpression{Target: program.ReferenceExpression{Name: "roll"}, Field: "first"},
			Right:    program.FieldExpression{Target: program.ReferenceExpression{Name: "roll"}, Field: "second"},
		},
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["name"] != "IsPair" {
		t.Fatalf("unexpected shape: %#v", obj)
	}
}

func TestDecode_UnknownField_FunctionDeclaration(t *testing.T) {
	_, err := decodeFunctionDeclaration("$", json.RawMessage(`{"name":"x","parameters":[],"result_type":null,"body":null,"extra":true}`))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

func TestDecode_NestedPathFailure_FunctionParameterType(t *testing.T) {
	data := json.RawMessage(`{
		"name": "f",
		"parameters": [
			{"name": "a", "type": {"kind":"builtin","type":"number"}},
			{"name": "b", "type": {"kind":"builtin","type":"number"}},
			{"name": "c", "type": {"kind": "not_a_real_type"}}
		],
		"result_type": null,
		"body": null
	}`)
	_, err := decodeFunctionDeclaration("$", data)
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$.parameters[2].type" {
		t.Fatalf("expected path $.parameters[2].type, got %q", decodeErr.Path)
	}
}

func TestDecode_NestedPathFailure_FunctionBody(t *testing.T) {
	data := json.RawMessage(`{"name":"f","parameters":[],"result_type":null,"body":{"kind":"not_a_real_expression"}}`)
	_, err := decodeFunctionDeclaration("$", data)
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$.body" {
		t.Fatalf("expected path $.body, got %q", decodeErr.Path)
	}
}

func TestDecode_FunctionDeclaration_Null(t *testing.T) {
	_, err := decodeFunctionDeclaration("$", json.RawMessage("null"))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

// --- invariant declarations ---

func TestInvariantDeclaration_RoundTrip_Variants(t *testing.T) {
	cases := []program.InvariantDeclaration{
		{
			Name: "TurnNumberIsNonNegative",
			Condition: program.BinaryExpression{
				Operator: program.BinaryOperatorGreaterOrEqual,
				Left:     program.FieldExpression{Target: program.ReferenceExpression{Name: "global"}, Field: "turnNumber"},
				Right:    program.NumberLiteralExpression{Value: "0"},
			},
		},
		{
			Name: "EveryParticipantOwnsExpectedTokens",
			Condition: program.CallExpression{
				Function: "EveryParticipantHasTokenCount",
				Arguments: []program.CallArgument{
					{Name: "tokens", Value: program.FieldExpression{Target: program.ReferenceExpression{Name: "global"}, Field: "tokens"}},
				},
			},
		},
		{
			Name: "EveryTokenOwnerExists",
			Condition: program.ListAllExpression{
				Collection: program.FieldExpression{Target: program.ReferenceExpression{Name: "global"}, Field: "tokens"},
				ItemName:   "token",
				Predicate: program.ListAnyExpression{
					Collection: program.FieldExpression{Target: program.ReferenceExpression{Name: "global"}, Field: "participants"},
					ItemName:   "participant",
					Predicate: program.BinaryExpression{
						Operator: program.BinaryOperatorEqual,
						Left:     program.FieldExpression{Target: program.ReferenceExpression{Name: "participant"}, Field: "id"},
						Right:    program.FieldExpression{Target: program.ReferenceExpression{Name: "token"}, Field: "owner"},
					},
				},
			},
		},
		{Name: "NilCondition", Condition: nil},
		{
			// Structurally valid, semantically invalid: references "local".
			Name:      "InvalidScope",
			Condition: program.FieldExpression{Target: program.ReferenceExpression{Name: "local"}, Field: "x"},
		},
	}
	for i, original := range cases {
		raw, err := encodeInvariantDeclaration("$", original)
		if err != nil {
			t.Fatalf("case %d: encode: %v", i, err)
		}
		decoded, err := decodeInvariantDeclaration("$", raw)
		if err != nil {
			t.Fatalf("case %d: decode: %v", i, err)
		}
		if !reflect.DeepEqual(original, decoded) {
			t.Fatalf("case %d: round trip mismatch:\n  original = %#v\n  decoded  = %#v", i, original, decoded)
		}
	}
}

func TestExactJSON_InvariantDeclaration(t *testing.T) {
	raw, err := encodeInvariantDeclaration("$", program.InvariantDeclaration{
		Name: "TurnNumberIsNonNegative",
		Condition: program.BinaryExpression{
			Operator: program.BinaryOperatorGreaterOrEqual,
			Left:     program.FieldExpression{Target: program.ReferenceExpression{Name: "global"}, Field: "turnNumber"},
			Right:    program.NumberLiteralExpression{Value: "0"},
		},
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["name"] != "TurnNumberIsNonNegative" {
		t.Fatalf("unexpected shape: %#v", obj)
	}
}

func TestDecode_NestedPathFailure_InvariantCondition(t *testing.T) {
	data := json.RawMessage(`{"name":"x","condition":{"kind":"not_a_real_expression"}}`)
	_, err := decodeInvariantDeclaration("$", data)
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$.condition" {
		t.Fatalf("expected path $.condition, got %q", decodeErr.Path)
	}
}

func TestSemanticInvalidity_InvariantWithNumericCondition(t *testing.T) {
	decoded, err := decodeInvariantDeclaration("$", json.RawMessage(`{"name":"x","condition":{"kind":"number_literal","value":"1"}}`))
	if err != nil {
		t.Fatalf("expected structural success, got %v", err)
	}
	if _, ok := decoded.Condition.(program.NumberLiteralExpression); !ok {
		t.Fatalf("expected numeric condition preserved, got %#v", decoded.Condition)
	}
}

func TestDecode_InvariantDeclaration_Null(t *testing.T) {
	_, err := decodeInvariantDeclaration("$", json.RawMessage("null"))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

// --- projection declarations ---

func TestProjectionDeclaration_RoundTrip_Variants(t *testing.T) {
	cases := []program.ProjectionDeclaration{
		{
			Name:       "Scoreboard",
			Parameters: []program.FieldDeclaration{},
			ResultType: program.NamedTypeReference{Name: "ScoreboardModel"},
			Body: program.RecordExpression{
				TypeName: "ScoreboardModel",
				Fields: []program.FieldInitializer{
					{Name: "scores", Value: program.FieldExpression{Target: program.ReferenceExpression{Name: "global"}, Field: "scores"}},
				},
			},
		},
		{
			Name:       "MyCards",
			ResultType: program.ListTypeReference{Element: program.NamedTypeReference{Name: "CardRef"}},
			Body: program.IndexExpression{
				Target: program.FieldExpression{Target: program.ReferenceExpression{Name: "global"}, Field: "hands"},
				Index:  program.ReferenceExpression{Name: "viewer"},
			},
		},
		{
			Name:       "OpponentSummary",
			Parameters: []program.FieldDeclaration{{Name: "opponent", Type: program.NamedTypeReference{Name: "ParticipantId"}}},
			ResultType: program.NamedTypeReference{Name: "OpponentSummaryModel"},
			Body: program.RecordExpression{
				TypeName: "OpponentSummaryModel",
				Fields: []program.FieldInitializer{
					{Name: "participant", Value: program.ReferenceExpression{Name: "opponent"}},
				},
			},
		},
		{
			Name: "MatchBoard",
			Body: program.MatchExpression{
				Value: program.ReferenceExpression{Name: "phase"},
				Cases: []program.MatchExpressionCase{
					{
						Pattern: ListAnyExpressionAsUnusedPlaceholder(),
						Result:  program.BoolLiteralExpression{Value: true},
					},
				},
			},
		},
		{
			Name:       "DuplicateParams",
			Parameters: []program.FieldDeclaration{{Name: "x", Type: nil}, {Name: "x", Type: nil}},
			ResultType: nil,
			Body:       nil,
		},
	}

	for i, original := range cases {
		raw, err := encodeProjectionDeclaration("$", original)
		if err != nil {
			t.Fatalf("case %d: encode: %v", i, err)
		}
		decoded, err := decodeProjectionDeclaration("$", raw)
		if err != nil {
			t.Fatalf("case %d: decode: %v", i, err)
		}
		if !reflect.DeepEqual(original, decoded) {
			t.Fatalf("case %d: round trip mismatch:\n  original = %#v\n  decoded  = %#v", i, original, decoded)
		}
	}
}

// ListAnyExpressionAsUnusedPlaceholder exists only so the MatchBoard test
// case above has a syntactically valid (if semantically nonsensical)
// pattern to round-trip; the codec performs no semantic validation.
func ListAnyExpressionAsUnusedPlaceholder() program.MatchPattern {
	return program.WildcardMatchPattern{}
}

func TestExactJSON_ProjectionDeclaration(t *testing.T) {
	raw, err := encodeProjectionDeclaration("$", program.ProjectionDeclaration{
		Name:       "MyCards",
		ResultType: program.ListTypeReference{Element: program.NamedTypeReference{Name: "CardRef"}},
		Body: program.IndexExpression{
			Target: program.FieldExpression{Target: program.ReferenceExpression{Name: "global"}, Field: "hands"},
			Index:  program.ReferenceExpression{Name: "viewer"},
		},
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["name"] != "MyCards" {
		t.Fatalf("unexpected shape: %#v", obj)
	}
}

func TestSemanticInvalidity_ProjectionReferencingLocal(t *testing.T) {
	decoded, err := decodeProjectionDeclaration("$", json.RawMessage(`{
		"name": "Bad",
		"parameters": [],
		"result_type": null,
		"body": {"kind": "field", "target": {"kind": "reference", "name": "local"}, "field": "x"}
	}`))
	if err != nil {
		t.Fatalf("expected structural success, got %v", err)
	}
	field, ok := decoded.Body.(program.FieldExpression)
	if !ok || field.Target.(program.ReferenceExpression).Name != "local" {
		t.Fatalf("expected out-of-scope reference preserved, got %#v", decoded.Body)
	}
}

func TestDecode_ProjectionDeclaration_Null(t *testing.T) {
	_, err := decodeProjectionDeclaration("$", json.RawMessage("null"))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

// --- view declarations ---

func TestViewDeclaration_RoundTrip_Variants(t *testing.T) {
	cases := []program.ViewDeclaration{
		{
			Name:      "ChooseCardView",
			ModelType: program.NamedTypeReference{Name: "ChooseCardModel"},
			LocalState: program.StateDeclaration{
				Fields: []program.StateFieldDeclaration{
					{
						Name:        "selectedCard",
						Type:        program.OptionalTypeReference{Element: program.NamedTypeReference{Name: "CardRef"}},
						Initializer: program.OptionalNoneExpression{ElementType: program.NamedTypeReference{Name: "CardRef"}},
					},
				},
			},
			Root: program.ContainerElement{
				Layout: program.StackLayout{},
				Children: []program.UIElement{
					program.TextElement{Value: program.StringLiteralExpression{Value: "Choose a card"}},
					program.ButtonElement{
						Configuration: program.UIElementConfiguration{
							Events: []program.UIEventHandler{
								{Event: program.UIEventTypeClick, Actions: []program.UIAction{program.AnswerQuestionAction{Value: program.ReferenceExpression{Name: "selectedCard"}}}},
							},
						},
					},
				},
			},
		},
		{
			Name:       "NilModelAndRoot",
			ModelType:  nil,
			LocalState: program.StateDeclaration{},
			Root:       nil,
		},
		{
			Name:       "NilAndEmptyLocalStateFields",
			LocalState: program.StateDeclaration{Fields: []program.StateFieldDeclaration{}},
			Root:       program.EmptyElement{},
		},
	}

	for i, original := range cases {
		raw, err := encodeViewDeclaration("$", original)
		if err != nil {
			t.Fatalf("case %d: encode: %v", i, err)
		}
		decoded, err := decodeViewDeclaration("$", raw)
		if err != nil {
			t.Fatalf("case %d: decode: %v", i, err)
		}
		if !reflect.DeepEqual(original, decoded) {
			t.Fatalf("case %d: round trip mismatch:\n  original = %#v\n  decoded  = %#v", i, original, decoded)
		}
	}
}

func TestExactJSON_ViewDeclaration(t *testing.T) {
	raw, err := encodeViewDeclaration("$", program.ViewDeclaration{
		Name:      "ChooseCardView",
		ModelType: program.NamedTypeReference{Name: "ChooseCardModel"},
		Root:      program.EmptyElement{},
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["name"] != "ChooseCardView" {
		t.Fatalf("unexpected shape: %#v", obj)
	}
	root, ok := obj["root"].(map[string]any)
	if !ok || root["kind"] != "empty" {
		t.Fatalf("unexpected root: %#v", obj["root"])
	}
}

func TestSemanticInvalidity_ViewReferencingGlobal(t *testing.T) {
	decoded, err := decodeViewDeclaration("$", json.RawMessage(`{
		"name": "Bad",
		"model_type": null,
		"local_state": {"fields": []},
		"root": {"kind": "text", "configuration": {"properties": [], "events": []}, "value": {"kind": "field", "target": {"kind": "reference", "name": "global"}, "field": "x"}}
	}`))
	if err != nil {
		t.Fatalf("expected structural success, got %v", err)
	}
	text, ok := decoded.Root.(program.TextElement)
	if !ok {
		t.Fatalf("expected program.TextElement root, got %T", decoded.Root)
	}
	field := text.Value.(program.FieldExpression)
	if field.Target.(program.ReferenceExpression).Name != "global" {
		t.Fatalf("expected out-of-scope global reference preserved, got %#v", field)
	}
}

func TestDecode_ViewDeclaration_Null(t *testing.T) {
	_, err := decodeViewDeclaration("$", json.RawMessage("null"))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

func TestDecode_NestedPathFailure_ViewLocalStateFieldInitializer(t *testing.T) {
	data := json.RawMessage(`{
		"name": "x",
		"model_type": null,
		"local_state": {
			"fields": [
				{"name": "a", "type": {"kind":"builtin","type":"number"}, "initializer": {"kind": "not_a_real_expression"}}
			]
		},
		"root": {"kind": "empty"}
	}`)
	_, err := decodeViewDeclaration("$", data)
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$.local_state.fields[0].initializer" {
		t.Fatalf("expected path $.local_state.fields[0].initializer, got %q", decodeErr.Path)
	}
}

func TestDecode_NestedPathFailure_ViewRootChildren(t *testing.T) {
	data := json.RawMessage(`{
		"name": "x",
		"model_type": null,
		"local_state": {"fields": []},
		"root": {
			"kind": "container",
			"configuration": {"properties": [], "events": []},
			"layout": {"kind": "stack"},
			"children": [
				{"kind": "empty"},
				{"kind": "not_a_real_element"}
			]
		}
	}`)
	_, err := decodeViewDeclaration("$", data)
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$.root.children[1]" {
		t.Fatalf("expected path $.root.children[1], got %q", decodeErr.Path)
	}
}

func TestDecode_UnknownField_ViewDeclaration(t *testing.T) {
	_, err := decodeViewDeclaration("$", json.RawMessage(`{
		"name": "x", "model_type": null, "local_state": {"fields": []}, "root": {"kind":"empty"}, "extra": true
	}`))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

// --- presentation slots ---

func TestPresentationSlotDeclaration_RoundTrip_Order(t *testing.T) {
	original := []program.PresentationSlotDeclaration{
		{Name: "main"}, {Name: "hud"}, {Name: "main"}, {Name: ""},
	}
	raw, err := encodePresentationSlotDeclarations("$.presentation_slots", original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodePresentationSlotDeclarations("$.presentation_slots", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", original, decoded)
	}
}

// --- persistent presentation declarations ---

func TestPresentationDeclaration_RoundTrip_Variants(t *testing.T) {
	cases := []program.PresentationDeclaration{
		{
			Name:       "MatchBoard",
			Slot:       "main",
			Targets:    program.ReferenceExpression{Name: "roomUsers"},
			Projection: "BoardProjection",
			View:       "BoardView",
		},
		{
			Name:    "SingleUserTarget",
			Slot:    "hud",
			Targets: program.ListExpression{ElementType: program.BuiltinTypeReference{Type: program.BuiltinTypeUser}, Elements: []program.Expression{program.ReferenceExpression{Name: "currentUser"}}},
			ProjectionArguments: []program.CallArgument{
				{Name: "selectedTeam", Value: program.FieldExpression{Target: program.ReferenceExpression{Name: "local"}, Field: "selectedTeam"}},
				{Name: "selectedTeam", Value: program.FieldExpression{Target: program.ReferenceExpression{Name: "local"}, Field: "selectedTeam"}},
			},
			Projection: "TeamStatusProjection",
			View:       "TeamStatusView",
		},
		{
			Name:    "NilTarget",
			Slot:    "unknownSlot",
			Targets: nil,
			// unknown projection/view names remain representable
			Projection: "UnknownProjection",
			View:       "UnknownView",
		},
	}
	for i, original := range cases {
		raw, err := encodePresentationDeclaration("$", original)
		if err != nil {
			t.Fatalf("case %d: encode: %v", i, err)
		}
		decoded, err := decodePresentationDeclaration("$", raw)
		if err != nil {
			t.Fatalf("case %d: decode: %v", i, err)
		}
		if !reflect.DeepEqual(original, decoded) {
			t.Fatalf("case %d: round trip mismatch:\n  original = %#v\n  decoded  = %#v", i, original, decoded)
		}
	}
}

func TestExactJSON_PresentationDeclaration(t *testing.T) {
	raw, err := encodePresentationDeclaration("$", program.PresentationDeclaration{
		Name:       "MatchBoard",
		Slot:       "main",
		Targets:    program.ReferenceExpression{Name: "roomUsers"},
		Projection: "BoardProjection",
		View:       "BoardView",
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["name"] != "MatchBoard" || obj["slot"] != "main" {
		t.Fatalf("unexpected shape: %#v", obj)
	}
}

func TestDecode_UnknownField_PresentationDeclaration(t *testing.T) {
	_, err := decodePresentationDeclaration("$", json.RawMessage(`{
		"name":"x","slot":"main","targets":null,"projection":"p","projection_arguments":[],"view":"v","extra":true
	}`))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

func TestDecode_NestedPathFailure_PresentationProjectionArguments(t *testing.T) {
	data := json.RawMessage(`{
		"name": "x", "slot": "main",
		"targets": {"kind": "reference", "name": "roomUsers"},
		"projection": "p",
		"projection_arguments": [
			{"name": "a", "value": {"kind": "not_a_real_expression"}}
		],
		"view": "v"
	}`)
	_, err := decodePresentationDeclaration("$", data)
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$.projection_arguments[0].value" {
		t.Fatalf("expected path $.projection_arguments[0].value, got %q", decodeErr.Path)
	}
}

func TestDecode_PresentationDeclaration_Null(t *testing.T) {
	_, err := decodePresentationDeclaration("$", json.RawMessage("null"))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

func TestSemanticInvalidity_PresentationTargetingSingleUser(t *testing.T) {
	// Structurally valid: a bare User reference instead of list<User>.
	decoded, err := decodePresentationDeclaration("$", json.RawMessage(`{
		"name": "x", "slot": "main",
		"targets": {"kind": "reference", "name": "singleUser"},
		"projection": "p", "projection_arguments": [], "view": "v"
	}`))
	if err != nil {
		t.Fatalf("expected structural success, got %v", err)
	}
	if decoded.Targets.(program.ReferenceExpression).Name != "singleUser" {
		t.Fatalf("expected single-user target preserved, got %#v", decoded.Targets)
	}
}

// --- question presentation pointer ---

func TestQuestionPresentationDeclaration_NilPointer_EncodesToNull(t *testing.T) {
	raw, err := encodeQuestionPresentationDeclaration("$", nil)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if string(raw) != "null" {
		t.Fatalf("expected null, got %s", raw)
	}
}

func TestQuestionPresentationDeclaration_DecodeNull_ReturnsNilPointer(t *testing.T) {
	decoded, err := decodeQuestionPresentationDeclaration("$", json.RawMessage("null"))
	if err != nil || decoded != nil {
		t.Fatalf("expected nil, nil got %#v, %v", decoded, err)
	}
}

func TestQuestionPresentationDeclaration_RoundTrip_NonNil(t *testing.T) {
	original := &program.QuestionPresentationDeclaration{
		Slot:       "primaryInteraction",
		Projection: "ChooseMoveProjection",
		ProjectionArguments: []program.CallArgument{
			{Name: "plans", Value: program.ReferenceExpression{Name: "plans"}},
			{Name: "plans", Value: program.ReferenceExpression{Name: "plans"}},
		},
		View: "ChooseMoveView",
	}
	raw, err := encodeQuestionPresentationDeclaration("$", original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeQuestionPresentationDeclaration("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", original, decoded)
	}
}

func TestQuestionPresentationDeclaration_NilAndEmptyArguments(t *testing.T) {
	nilArgs := &program.QuestionPresentationDeclaration{Slot: "s", Projection: "p", View: "v", ProjectionArguments: nil}
	raw, err := encodeQuestionPresentationDeclaration("$", nilArgs)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeQuestionPresentationDeclaration("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.ProjectionArguments != nil {
		t.Fatalf("expected nil arguments preserved, got %#v", decoded.ProjectionArguments)
	}

	emptyArgs := &program.QuestionPresentationDeclaration{Slot: "s", Projection: "p", View: "v", ProjectionArguments: []program.CallArgument{}}
	raw, err = encodeQuestionPresentationDeclaration("$", emptyArgs)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err = decodeQuestionPresentationDeclaration("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.ProjectionArguments == nil || len(decoded.ProjectionArguments) != 0 {
		t.Fatalf("expected empty non-nil arguments preserved, got %#v", decoded.ProjectionArguments)
	}
}

func TestExactJSON_QuestionPresentationDeclaration(t *testing.T) {
	raw, err := encodeQuestionPresentationDeclaration("$", &program.QuestionPresentationDeclaration{
		Slot: "primaryInteraction", Projection: "ChooseMoveProjection", View: "ChooseMoveView",
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["slot"] != "primaryInteraction" {
		t.Fatalf("unexpected shape: %#v", obj)
	}
}

func TestDecode_UnknownField_QuestionPresentationDeclaration(t *testing.T) {
	_, err := decodeQuestionPresentationDeclaration("$", json.RawMessage(`{
		"slot":"s","projection":"p","projection_arguments":[],"view":"v","extra":true
	}`))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

func TestDecode_NestedPathFailure_QuestionPresentationArguments(t *testing.T) {
	data := json.RawMessage(`{
		"slot": "s", "projection": "p", "view": "v",
		"projection_arguments": [
			{"name": "a", "value": {"kind": "reference", "name": "a"}},
			{"name": "b", "value": {"kind": "not_a_real_expression"}}
		]
	}`)
	_, err := decodeQuestionPresentationDeclaration("$", data)
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$.projection_arguments[1].value" {
		t.Fatalf("expected path $.projection_arguments[1].value, got %q", decodeErr.Path)
	}
}

// --- interaction declarations ---

func TestUserIntentDeclaration_RoundTrip(t *testing.T) {
	original := program.UserIntentDeclaration{
		Name: "PlayCard",
		Parameters: []program.FieldDeclaration{
			{Name: "card", Type: program.NamedTypeReference{Name: "CardRef"}},
			{Name: "target", Type: program.OptionalTypeReference{Element: program.NamedTypeReference{Name: "TokenRef"}}},
			{Name: "card", Type: program.NamedTypeReference{Name: "CardRef"}},
		},
	}
	raw, err := encodeUserIntentDeclaration("$", original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeUserIntentDeclaration("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", original, decoded)
	}
}

func TestQuestionDeclaration_RoundTrip_Variants(t *testing.T) {
	cases := []program.QuestionDeclaration{
		{
			Name:         "ChooseCard",
			Parameters:   []program.FieldDeclaration{{Name: "options", Type: program.ListTypeReference{Element: program.NamedTypeReference{Name: "CardRef"}}}},
			ResponseType: program.NamedTypeReference{Name: "CardRef"},
			Validation: program.BinaryExpression{
				Operator: program.BinaryOperatorIn,
				Left:     program.ReferenceExpression{Name: "answer"},
				Right:    program.ReferenceExpression{Name: "options"},
			},
		},
		{Name: "NilResponseAndValidation", ResponseType: nil, Validation: nil},
		{
			// Structurally valid, semantically wrong validator type.
			Name:         "InvalidValidatorType",
			ResponseType: program.BuiltinTypeReference{Type: program.BuiltinTypeBool},
			Validation:   program.NumberLiteralExpression{Value: "1"},
		},
	}
	for i, original := range cases {
		raw, err := encodeQuestionDeclaration("$", original)
		if err != nil {
			t.Fatalf("case %d: encode: %v", i, err)
		}
		decoded, err := decodeQuestionDeclaration("$", raw)
		if err != nil {
			t.Fatalf("case %d: decode: %v", i, err)
		}
		if !reflect.DeepEqual(original, decoded) {
			t.Fatalf("case %d: round trip mismatch:\n  original = %#v\n  decoded  = %#v", i, original, decoded)
		}
	}
}

func TestExactJSON_QuestionDeclaration(t *testing.T) {
	raw, err := encodeQuestionDeclaration("$", program.QuestionDeclaration{
		Name:         "ChooseCard",
		ResponseType: program.NamedTypeReference{Name: "CardRef"},
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["name"] != "ChooseCard" {
		t.Fatalf("unexpected shape: %#v", obj)
	}
}

func TestDecode_UnknownField_QuestionDeclaration(t *testing.T) {
	_, err := decodeQuestionDeclaration("$", json.RawMessage(`{
		"name":"x","parameters":[],"response_type":null,"validation":null,"extra":true
	}`))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

func TestDecode_QuestionDeclaration_Null(t *testing.T) {
	_, err := decodeQuestionDeclaration("$", json.RawMessage("null"))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

func TestEffectDeclaration_RoundTrip(t *testing.T) {
	original := program.EffectDeclaration{
		Name: "DiceRolled",
		Parameters: []program.FieldDeclaration{
			{Name: "first", Type: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}},
			{Name: "second", Type: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}},
		},
	}
	raw, err := encodeEffectDeclaration("$", original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeEffectDeclaration("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", original, decoded)
	}
}

func TestDecode_EffectDeclaration_Null(t *testing.T) {
	_, err := decodeEffectDeclaration("$", json.RawMessage("null"))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

func TestDecode_UserIntentDeclaration_Null(t *testing.T) {
	_, err := decodeUserIntentDeclaration("$", json.RawMessage("null"))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

// --- workflow-owned slot declarations ---

func TestQuestionSlotDeclaration_RoundTrip(t *testing.T) {
	cases := []program.QuestionSlotDeclaration{
		{
			Name:     "moveRequest",
			Question: "ChooseMove",
			Presentation: &program.QuestionPresentationDeclaration{
				Slot:       "primaryInteraction",
				Projection: "ChooseMoveProjection",
				View:       "ChooseMoveView",
			},
		},
		{Name: "headless", Question: "ConfirmAction", Presentation: nil},
		{Name: "unknownQuestion", Question: "DoesNotExist", Presentation: nil},
	}
	for i, original := range cases {
		raw, err := encodeQuestionSlotDeclaration("$", original)
		if err != nil {
			t.Fatalf("case %d: encode: %v", i, err)
		}
		decoded, err := decodeQuestionSlotDeclaration("$", raw)
		if err != nil {
			t.Fatalf("case %d: decode: %v", i, err)
		}
		if !reflect.DeepEqual(original, decoded) {
			t.Fatalf("case %d: round trip mismatch:\n  original = %#v\n  decoded  = %#v", i, original, decoded)
		}
	}
}

func TestExactJSON_QuestionSlotDeclaration(t *testing.T) {
	raw, err := encodeQuestionSlotDeclaration("$", program.QuestionSlotDeclaration{
		Name:     "moveRequest",
		Question: "ChooseMove",
		Presentation: &program.QuestionPresentationDeclaration{
			Slot: "primaryInteraction", Projection: "ChooseMoveProjection", View: "ChooseMoveView",
		},
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["name"] != "moveRequest" || obj["question"] != "ChooseMove" {
		t.Fatalf("unexpected shape: %#v", obj)
	}
	presentation, ok := obj["presentation"].(map[string]any)
	if !ok || presentation["slot"] != "primaryInteraction" {
		t.Fatalf("unexpected presentation: %#v", obj["presentation"])
	}
}

func TestAskGroupSlotDeclaration_RoundTrip(t *testing.T) {
	original := program.AskGroupSlotDeclaration{
		Name:     "cardVotes",
		Question: "ChooseCard",
		Presentation: &program.QuestionPresentationDeclaration{
			Slot:       "primaryInteraction",
			Projection: "ChooseCardProjection",
			ProjectionArguments: []program.CallArgument{
				{Name: "options", Value: program.ReferenceExpression{Name: "options"}},
			},
			View: "ChooseCardView",
		},
	}
	raw, err := encodeAskGroupSlotDeclaration("$", original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeAskGroupSlotDeclaration("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", original, decoded)
	}
}

func TestTimerSlotDeclaration_RoundTrip(t *testing.T) {
	original := program.TimerSlotDeclaration{Name: "moveDeadline"}
	raw, err := encodeTimerSlotDeclaration("$", original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeTimerSlotDeclaration("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded != original {
		t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", original, decoded)
	}
}

func TestDecode_TimerSlotDeclaration_Null(t *testing.T) {
	_, err := decodeTimerSlotDeclaration("$", json.RawMessage("null"))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

func TestChildWorkflowSlotDeclaration_RoundTrip(t *testing.T) {
	original := program.ChildWorkflowSlotDeclaration{Name: "activeTurn", Workflow: "PlayTurn"}
	raw, err := encodeChildWorkflowSlotDeclaration("$", original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeChildWorkflowSlotDeclaration("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded != original {
		t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", original, decoded)
	}
}

func TestTaskGroupSlotDeclaration_RoundTrip_Variants(t *testing.T) {
	cases := []program.TaskGroupSlotDeclaration{
		{Name: "teamSelections", Workflow: "TeamChooseCard", KeyType: program.NamedTypeReference{Name: "TeamId"}},
		{Name: "unknownWorkflow", Workflow: "DoesNotExist", KeyType: nil},
		{Name: "duplicateSlotName", Workflow: "TeamChooseCard", KeyType: program.NamedTypeReference{Name: "TeamId"}},
	}
	for i, original := range cases {
		raw, err := encodeTaskGroupSlotDeclaration("$", original)
		if err != nil {
			t.Fatalf("case %d: encode: %v", i, err)
		}
		decoded, err := decodeTaskGroupSlotDeclaration("$", raw)
		if err != nil {
			t.Fatalf("case %d: decode: %v", i, err)
		}
		if !reflect.DeepEqual(original, decoded) {
			t.Fatalf("case %d: round trip mismatch:\n  original = %#v\n  decoded  = %#v", i, original, decoded)
		}
	}
}

func TestExactJSON_TaskGroupSlotDeclaration(t *testing.T) {
	raw, err := encodeTaskGroupSlotDeclaration("$", program.TaskGroupSlotDeclaration{
		Name: "teamSelections", Workflow: "TeamChooseCard", KeyType: program.NamedTypeReference{Name: "TeamId"},
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["name"] != "teamSelections" || obj["workflow"] != "TeamChooseCard" {
		t.Fatalf("unexpected shape: %#v", obj)
	}
}

func TestDecode_NestedPathFailure_TaskGroupSlotKeyType(t *testing.T) {
	data := json.RawMessage(`{"name":"x","workflow":"w","key_type":{"kind":"not_a_real_type"}}`)
	_, err := decodeTaskGroupSlotDeclaration("$", data)
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$.key_type" {
		t.Fatalf("expected path $.key_type, got %q", decodeErr.Path)
	}
}

func TestSemanticInvalidity_TaskGroupSlotNilKeyType(t *testing.T) {
	decoded, err := decodeTaskGroupSlotDeclaration("$", json.RawMessage(`{"name":"x","workflow":"w","key_type":null}`))
	if err != nil {
		t.Fatalf("expected structural success, got %v", err)
	}
	if decoded.KeyType != nil {
		t.Fatalf("expected nil key type preserved, got %#v", decoded.KeyType)
	}
}

func TestDuplicateSlotDeclarations_InHelperSlices(t *testing.T) {
	original := []program.TimerSlotDeclaration{{Name: "a"}, {Name: "a"}, {Name: "b"}}
	raw, err := encodeTimerSlotDeclarations("$.timer_slots", original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeTimerSlotDeclarations("$.timer_slots", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", original, decoded)
	}
}

func TestNilVsEmptySlices_QuestionSlotDeclarations(t *testing.T) {
	var nilSlots []program.QuestionSlotDeclaration
	raw, err := encodeQuestionSlotDeclarations("$.question_slots", nilSlots)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeQuestionSlotDeclarations("$.question_slots", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded != nil {
		t.Fatalf("expected nil preserved, got %#v", decoded)
	}

	emptySlots := []program.QuestionSlotDeclaration{}
	raw, err = encodeQuestionSlotDeclarations("$.question_slots", emptySlots)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err = decodeQuestionSlotDeclarations("$.question_slots", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded == nil || len(decoded) != 0 {
		t.Fatalf("expected empty non-nil preserved, got %#v", decoded)
	}
}
