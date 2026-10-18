package inputs

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func number(value float64) *float64 { return &value }

func TestNewSchema(t *testing.T) {
	type test struct {
		declarations []Declaration

		expected  []Declaration
		expectErr bool
	}

	tests := map[string]func(t *testing.T) test{
		"natural_zeros_are_filled_in": func(t *testing.T) test {
			return test{
				declarations: []Declaration{
					{Name: "move", Type: Vector2},
					{Name: "attackHeld", Type: Boolean},
					{Name: "selectedCard", Type: String, Nullable: true},
					{Name: "label", Type: String},
					{Name: "throttle", Type: Number, Min: number(-1), Max: number(1)},
				},
				expected: []Declaration{
					{Name: "attackHeld", Type: Boolean, Default: json.RawMessage(`false`)},
					{Name: "label", Type: String, Default: json.RawMessage(`""`)},
					{Name: "move", Type: Vector2, Default: json.RawMessage(`[0,0]`)},
					{Name: "selectedCard", Type: String, Nullable: true, Default: json.RawMessage(`null`)},
					{Name: "throttle", Type: Number, Min: number(-1), Max: number(1), Default: json.RawMessage(`0`)},
				},
			}
		},
		"enum_with_its_default": func(t *testing.T) test {
			return test{
				declarations: []Declaration{{Name: "mode", Type: Enum, Values: []string{"walk", "run"}, Default: json.RawMessage(`"walk"`)}},
				expected:     []Declaration{{Name: "mode", Type: Enum, Values: []string{"walk", "run"}, Default: json.RawMessage(`"walk"`)}},
			}
		},
		"no_inputs": func(t *testing.T) test {
			return test{}
		},
		"enum_without_default": func(t *testing.T) test {
			return test{declarations: []Declaration{{Name: "mode", Type: Enum, Values: []string{"walk"}}}, expectErr: true}
		},
		"zero_outside_the_range_needs_a_default": func(t *testing.T) test {
			return test{declarations: []Declaration{{Name: "speed", Type: Number, Min: number(1), Max: number(5)}}, expectErr: true}
		},
		"declared_default_outside_the_range": func(t *testing.T) test {
			return test{declarations: []Declaration{{Name: "speed", Type: Number, Min: number(1), Max: number(5), Default: json.RawMessage(`9`)}}, expectErr: true}
		},
		"unknown_type": func(t *testing.T) test {
			return test{declarations: []Declaration{{Name: "x", Type: "matrix"}}, expectErr: true}
		},
		"invalid_name": func(t *testing.T) test {
			return test{declarations: []Declaration{{Name: "9lives", Type: Boolean}}, expectErr: true}
		},
		"nullable_number": func(t *testing.T) test {
			return test{declarations: []Declaration{{Name: "x", Type: Number, Nullable: true}}, expectErr: true}
		},
		"repeated_enum_values": func(t *testing.T) test {
			return test{declarations: []Declaration{{Name: "mode", Type: Enum, Values: []string{"a", "a"}, Default: json.RawMessage(`"a"`)}}, expectErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			schema, err := NewSchema(tc.declarations)

			if tc.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, schema.Declarations())
		})
	}
}

func TestNormalize(t *testing.T) {
	type test struct {
		raw string

		expected  Snapshot
		expectErr bool
	}

	schema, err := NewSchema([]Declaration{
		{Name: "move", Type: Vector2},
		{Name: "attackHeld", Type: Boolean},
		{Name: "selectedCard", Type: String, Nullable: true},
		{Name: "mode", Type: Enum, Values: []string{"walk", "run"}, Default: json.RawMessage(`"walk"`)},
		{Name: "throttle", Type: Number, Min: number(0), Max: number(1)},
	})
	require.NoError(t, err)

	tests := map[string]func(t *testing.T) test{
		"omitted_inputs_take_their_defaults": func(t *testing.T) test {
			return test{raw: `{"move":[1,0]}`, expected: `{"attackHeld":false,"mode":"walk","move":[1,0],"selectedCard":null,"throttle":0}`}
		},
		"empty_sample_is_all_defaults": func(t *testing.T) test {
			return test{raw: `{}`, expected: schema.Defaults()}
		},
		"equal_values_in_other_spellings_are_equal": func(t *testing.T) test {
			return test{raw: `{"throttle":0.50,"move":[1.0, 2e0]}`, expected: `{"attackHeld":false,"mode":"walk","move":[1,2],"selectedCard":null,"throttle":0.5}`}
		},
		"undeclared_input": func(t *testing.T) test {
			return test{raw: `{"jump":true}`, expectErr: true}
		},
		"wrong_type": func(t *testing.T) test {
			return test{raw: `{"attackHeld":"yes"}`, expectErr: true}
		},
		"out_of_range": func(t *testing.T) test {
			return test{raw: `{"throttle":2}`, expectErr: true}
		},
		"unknown_enum_value": func(t *testing.T) test {
			return test{raw: `{"mode":"fly"}`, expectErr: true}
		},
		"vector_of_three": func(t *testing.T) test {
			return test{raw: `{"move":[1,0,0]}`, expectErr: true}
		},
		"not_an_object": func(t *testing.T) test {
			return test{raw: `[1]`, expectErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			snapshot, err := schema.Normalize([]byte(tc.raw))

			if tc.expectErr {
				require.ErrorIs(t, err, ErrInvalidValue)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, snapshot)
		})
	}
}
