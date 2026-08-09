package program

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

// --- UILayout ---

func TestUILayout_AllVariants_RoundTrip(t *testing.T) {
	variants := []UILayout{
		StackLayout{},
		AbsoluteLayout{},
		LinearLayout{Direction: LinearLayoutDirectionRow, Gap: NumberLiteralExpression{Value: "8"}},
		LinearLayout{Direction: LinearLayoutDirectionColumn},
		GridLayout{
			Columns:   NumberLiteralExpression{Value: "3"},
			RowGap:    NumberLiteralExpression{Value: "4"},
			ColumnGap: NumberLiteralExpression{Value: "4"},
		},
	}
	for _, original := range variants {
		raw, err := encodeUILayout("$", original)
		if err != nil {
			t.Fatalf("encode %#v: %v", original, err)
		}
		decoded, err := decodeUILayout("$", raw)
		if err != nil {
			t.Fatalf("decode %#v: %v", original, err)
		}
		if !reflect.DeepEqual(original, decoded) {
			t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", original, decoded)
		}
	}
}

func TestUILayout_PointerAndValueEquivalence(t *testing.T) {
	value := LinearLayout{Direction: LinearLayoutDirectionRow}
	valueJSON, err := encodeUILayout("$", value)
	if err != nil {
		t.Fatalf("encode value: %v", err)
	}
	pointerJSON, err := encodeUILayout("$", &value)
	if err != nil {
		t.Fatalf("encode pointer: %v", err)
	}
	assertSemanticJSONEqual(t, valueJSON, pointerJSON)
}

func TestUILayout_TypedNilEncodesToNull(t *testing.T) {
	var layout *GridLayout
	raw, err := encodeUILayout("$", layout)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if string(raw) != "null" {
		t.Fatalf("expected null, got %s", raw)
	}
}

// --- UIAction ---

func TestUIAction_AllVariants_RoundTrip(t *testing.T) {
	variants := []UIAction{
		SetLocalStateAction{
			Target: FieldTarget{Target: NameTarget{Name: "local"}, Field: "selectedCard"},
			Value:  FieldExpression{Target: ReferenceExpression{Name: "card"}, Field: "id"},
		},
		AnswerQuestionAction{Value: ReferenceExpression{Name: "selectedCard"}},
		EmitUserIntentAction{
			Intent: "SelectMove",
			Arguments: []CallArgument{
				{Name: "plan", Value: ReferenceExpression{Name: "selectedMove"}},
			},
		},
	}
	for _, original := range variants {
		t.Run(reflect.TypeOf(original).Name(), func(t *testing.T) {
			raw, err := encodeUIAction("$", original)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			decoded, err := decodeUIAction("$", raw)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if !reflect.DeepEqual(original, decoded) {
				t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", original, decoded)
			}
		})
	}
}

// --- UIElementConfiguration / UIEventHandler / UIProperty ---

func TestUIElementConfiguration_RoundTrip(t *testing.T) {
	original := UIElementConfiguration{
		Properties: []UIProperty{
			{Name: "width", Value: NumberLiteralExpression{Value: "100"}},
			{Name: "height", Value: NumberLiteralExpression{Value: "100"}},
			{Name: "width", Value: NumberLiteralExpression{Value: "200"}},
		},
		Events: []UIEventHandler{
			{Event: UIEventTypePointerEnter, Actions: []UIAction{SetLocalStateAction{Target: NameTarget{Name: "local"}, Value: BoolLiteralExpression{Value: true}}}},
			{Event: UIEventTypeClick, Actions: []UIAction{
				SetLocalStateAction{Target: NameTarget{Name: "local"}, Value: BoolLiteralExpression{Value: false}},
				EmitUserIntentAction{Intent: "Select"},
			}},
		},
	}
	raw, err := encodeUIElementConfiguration("$", original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeUIElementConfiguration("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", original, decoded)
	}
	if len(decoded.Properties) != 3 || decoded.Properties[0].Name != "width" || decoded.Properties[2].Name != "width" {
		t.Fatalf("expected duplicate property order preserved, got %#v", decoded.Properties)
	}
	if len(decoded.Events[1].Actions) != 2 {
		t.Fatalf("expected 2 actions in second handler, got %d", len(decoded.Events[1].Actions))
	}
}

func TestUIElementConfiguration_DecodeNull_IsStructuralError(t *testing.T) {
	_, err := decodeUIElementConfiguration("$", json.RawMessage("null"))
	if err == nil {
		t.Fatal("expected an error decoding null as UIElementConfiguration")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

// --- UIElement: exhaustive table ---

func allUIElementVariants() []UIElement {
	config := UIElementConfiguration{
		Properties: []UIProperty{{Name: "width", Value: NumberLiteralExpression{Value: "100"}}},
	}
	return []UIElement{
		EmptyElement{},
		ContainerElement{
			Configuration: config,
			Layout:        StackLayout{},
			Children:      []UIElement{EmptyElement{}, TextElement{Configuration: config, Value: StringLiteralExpression{Value: "hi"}}},
		},
		TextElement{Configuration: config, Value: StringLiteralExpression{Value: "hello"}},
		ImageElement{
			Configuration:   config,
			Source:          ReferenceExpression{Name: "icon"},
			AlternativeText: StringLiteralExpression{Value: "an icon"},
		},
		ButtonElement{
			Configuration: config,
			Children:      []UIElement{TextElement{Configuration: config, Value: StringLiteralExpression{Value: "Confirm"}}},
		},
		RepeatElement{
			Collection: FieldExpression{Target: ReferenceExpression{Name: "model"}, Field: "cards"},
			ItemName:   "card",
			IndexName:  "index",
			Key:        FieldExpression{Target: ReferenceExpression{Name: "card"}, Field: "id"},
			Body:       TextElement{Configuration: config, Value: ReferenceExpression{Name: "card"}},
		},
		ConditionalElement{
			Condition: FieldExpression{Target: ReferenceExpression{Name: "model"}, Field: "hasWinner"},
			Then:      TextElement{Configuration: config, Value: StringLiteralExpression{Value: "Winner"}},
			Else:      EmptyElement{},
		},
	}
}

func TestUIElement_AllVariants_RoundTrip(t *testing.T) {
	for _, original := range allUIElementVariants() {
		t.Run(reflect.TypeOf(original).Name(), func(t *testing.T) {
			raw, err := encodeUIElement("$", original)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			decoded, err := decodeUIElement("$", raw)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if reflect.TypeOf(decoded) != reflect.TypeOf(original) {
				t.Fatalf("expected decoded type %T, got %T", original, decoded)
			}
			if !reflect.DeepEqual(original, decoded) {
				t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", original, decoded)
			}
			reencoded, err := encodeUIElement("$", decoded)
			if err != nil {
				t.Fatalf("re-encode: %v", err)
			}
			assertSemanticJSONEqual(t, raw, reencoded)
		})
	}
}

func TestUIElement_PointerAndValueEquivalence(t *testing.T) {
	value := TextElement{Value: StringLiteralExpression{Value: "hi"}}
	valueJSON, err := encodeUIElement("$", value)
	if err != nil {
		t.Fatalf("encode value: %v", err)
	}
	pointerJSON, err := encodeUIElement("$", &value)
	if err != nil {
		t.Fatalf("encode pointer: %v", err)
	}
	assertSemanticJSONEqual(t, valueJSON, pointerJSON)
}

func TestUIElement_TypedNilEncodesToNull(t *testing.T) {
	var element *EmptyElement
	raw, err := encodeUIElement("$", element)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if string(raw) != "null" {
		t.Fatalf("expected null, got %s", raw)
	}
}

func TestUIElement_DecodeNull(t *testing.T) {
	decoded, err := decodeUIElement("$", json.RawMessage("null"))
	if err != nil || decoded != nil {
		t.Fatalf("expected nil, nil got %#v, %v", decoded, err)
	}
}

// --- error handling ---

func TestDecode_UnknownDiscriminator_UIFamilies(t *testing.T) {
	if _, err := decodeUILayout("$", json.RawMessage(`{"kind":"not_a_real_layout"}`)); err == nil {
		t.Fatal("expected an error for UILayout")
	} else {
		var decodeErr *DecodeError
		if !errors.As(err, &decodeErr) || decodeErr.Path != "$" {
			t.Fatalf("expected *DecodeError at $, got %v", err)
		}
	}

	if _, err := decodeUIElement("$", json.RawMessage(`{"kind":"not_a_real_element"}`)); err == nil {
		t.Fatal("expected an error for UIElement")
	} else {
		var decodeErr *DecodeError
		if !errors.As(err, &decodeErr) || decodeErr.Path != "$" {
			t.Fatalf("expected *DecodeError at $, got %v", err)
		}
	}

	if _, err := decodeUIAction("$", json.RawMessage(`{"kind":"not_a_real_action"}`)); err == nil {
		t.Fatal("expected an error for UIAction")
	} else {
		var decodeErr *DecodeError
		if !errors.As(err, &decodeErr) || decodeErr.Path != "$" {
			t.Fatalf("expected *DecodeError at $, got %v", err)
		}
	}
}

func TestDecode_UnknownField_UIElement(t *testing.T) {
	_, err := decodeUIElement("$", json.RawMessage(`{"kind":"empty","unexpected":true}`))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

func TestDecode_NestedPathFailure_ContainerChildren(t *testing.T) {
	data := json.RawMessage(`{
		"kind": "container",
		"configuration": {"properties": [], "events": []},
		"layout": {"kind": "stack"},
		"children": [
			{"kind": "empty"},
			{"kind": "not_a_real_element"}
		]
	}`)
	_, err := decodeUIElement("$", data)
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if decodeErr.Path != "$.children[1]" {
		t.Fatalf("expected path $.children[1], got %q", decodeErr.Path)
	}
}

func TestDecode_NestedPathFailure_RepeatBody(t *testing.T) {
	data := json.RawMessage(`{
		"kind": "repeat",
		"collection": {"kind": "reference", "name": "cards"},
		"item_name": "card",
		"index_name": "",
		"key": null,
		"body": {"kind": "not_a_real_element"}
	}`)
	_, err := decodeUIElement("$", data)
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

// --- semantic invalidity preserved ---

func TestUISemanticInvalidity_InvalidLinearDirection(t *testing.T) {
	decoded, err := decodeUILayout("$", json.RawMessage(`{"kind":"linear","direction":"diagonal","gap":null}`))
	if err != nil {
		t.Fatalf("expected structural success, got %v", err)
	}
	if decoded.(LinearLayout).Direction != LinearLayoutDirection("diagonal") {
		t.Fatalf("expected unknown direction preserved, got %q", decoded.(LinearLayout).Direction)
	}
}

func TestUISemanticInvalidity_InvalidEventType(t *testing.T) {
	decoded, err := decodeUIElementConfiguration("$", json.RawMessage(`{
		"properties": [],
		"events": [{"event": "long_press", "actions": []}]
	}`))
	if err != nil {
		t.Fatalf("expected structural success, got %v", err)
	}
	if decoded.Events[0].Event != UIEventType("long_press") {
		t.Fatalf("expected unknown event type preserved, got %q", decoded.Events[0].Event)
	}
}
