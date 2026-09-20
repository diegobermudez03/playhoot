package sessionruntime

import (
	"testing"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/stretchr/testify/require"
)

func TestValueToAny(t *testing.T) {
	tests := map[string]struct {
		value engine.Value
		want  any
	}{
		"unit_becomes_nil": {
			value: engine.UnitValue{},
			want:  nil,
		},
		"bool": {
			value: engine.BoolValue{Value: true},
			want:  true,
		},
		"number": {
			value: engine.NumberValue{Value: 42},
			want:  float64(42),
		},
		"string": {
			value: engine.StringValue{Value: "hi"},
			want:  "hi",
		},
		"user_becomes_its_raw_engine_id": {
			value: engine.UserValue{ID: engine.UserID("7")},
			want:  "7",
		},
		"enum_becomes_its_value_name": {
			value: engine.EnumValue{TypeName: "Suit", ValueName: "HEARTS"},
			want:  "HEARTS",
		},
		"record_becomes_a_field_map": {
			value: engine.RecordValue{TypeName: "Point", Fields: []engine.FieldValue{
				{Name: "x", Value: engine.NumberValue{Value: 1}},
				{Name: "y", Value: engine.NumberValue{Value: 2}},
			}},
			want: map[string]any{"x": float64(1), "y": float64(2)},
		},
		"union_carries_its_variant_name": {
			value: engine.UnionValue{TypeName: "Shape", VariantName: "Circle", Fields: []engine.FieldValue{
				{Name: "radius", Value: engine.NumberValue{Value: 3}},
			}},
			want: map[string]any{"variant": "Circle", "radius": float64(3)},
		},
		"new_type_unwraps_to_its_underlying_value": {
			value: engine.NewTypeValue{TypeName: "Score", Underlying: engine.NumberValue{Value: 9}},
			want:  float64(9),
		},
		"absent_optional_becomes_nil": {
			value: engine.OptionalValue{ElementType: engine.NumberType{}, Value: nil},
			want:  nil,
		},
		"present_optional_unwraps": {
			value: engine.OptionalValue{ElementType: engine.NumberType{}, Value: engine.NumberValue{Value: 5}},
			want:  float64(5),
		},
		"list_becomes_a_slice": {
			value: engine.ListValue{ElementType: engine.NumberType{}, Elements: []engine.Value{
				engine.NumberValue{Value: 1}, engine.NumberValue{Value: 2},
			}},
			want: []any{float64(1), float64(2)},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, valueToAny(tc.value))
		})
	}
}

func TestDecodeInteractionPayload(t *testing.T) {
	t.Run("decodes_question_and_arguments", func(t *testing.T) {
		payload := []byte(`{"question":"PickNumber","arguments":{"kind":"record","fields":[{"name":"min","value":{"kind":"number","number":1}},{"name":"max","value":{"kind":"number","number":10}}]}}`)

		question, arguments, err := decodeInteractionPayload(payload)
		require.NoError(t, err)
		require.Equal(t, "PickNumber", question)
		require.Equal(t, map[string]any{"min": float64(1), "max": float64(10)}, arguments)
	})

	t.Run("rejects_malformed_json", func(t *testing.T) {
		_, _, err := decodeInteractionPayload([]byte(`not json`))
		require.Error(t, err)
	})
}
