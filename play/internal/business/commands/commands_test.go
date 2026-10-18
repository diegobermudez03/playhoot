package commands

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/internal/business/inputs"
)

func number(value float64) *float64 { return &value }

func TestNewCatalog(t *testing.T) {
	type test struct {
		declarations []Declaration

		expected  []Declaration
		expectErr bool
	}

	tests := map[string]func(t *testing.T) test{
		"payload_defaults_are_filled_in_and_names_sorted": func(t *testing.T) test {
			return test{
				declarations: []Declaration{
					{Name: "play_card", Payload: []inputs.Declaration{{Name: "card", Type: inputs.String}}},
					{Name: "pass"},
				},
				expected: []Declaration{
					{Name: "pass"},
					{Name: "play_card", Payload: []inputs.Declaration{{Name: "card", Type: inputs.String, Default: json.RawMessage(`""`)}}},
				},
			}
		},
		"no_commands": func(t *testing.T) test {
			return test{}
		},
		"invalid_name": func(t *testing.T) test {
			return test{declarations: []Declaration{{Name: "play card"}}, expectErr: true}
		},
		"declared_twice": func(t *testing.T) test {
			return test{declarations: []Declaration{{Name: "pass"}, {Name: "pass"}}, expectErr: true}
		},
		"invalid_payload_field": func(t *testing.T) test {
			return test{declarations: []Declaration{{Name: "pick", Payload: []inputs.Declaration{{Name: "mode", Type: inputs.Enum, Values: []string{"a"}}}}}, expectErr: true}
		},
		"too_many_commands": func(t *testing.T) test {
			declarations := make([]Declaration, MaxCommands+1)
			for i := range declarations {
				declarations[i] = Declaration{Name: fmt.Sprintf("c%d", i)}
			}
			return test{declarations: declarations, expectErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			catalog, err := NewCatalog(tc.declarations)

			if tc.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, catalog.Declarations())
		})
	}
}

func TestNormalize(t *testing.T) {
	type test struct {
		name    string
		payload []byte

		expected        string
		expectedErrorIs error
	}

	tests := map[string]func(t *testing.T) test{
		"omitted_fields_take_their_defaults": func(t *testing.T) test {
			return test{name: "buy", payload: []byte(`{"item":"sword"}`), expected: `{"item":"sword","quantity":1}`}
		},
		"no_payload_is_every_default": func(t *testing.T) test {
			return test{name: "buy", expected: `{"item":"","quantity":1}`}
		},
		"command_without_payload_fields": func(t *testing.T) test {
			return test{name: "pass", expected: `{}`}
		},
		"undeclared_command": func(t *testing.T) test {
			return test{name: "fly", expectedErrorIs: ErrUndeclared}
		},
		"undeclared_field": func(t *testing.T) test {
			return test{name: "pass", payload: []byte(`{"why":"tired"}`), expectedErrorIs: inputs.ErrInvalidValue}
		},
		"value_out_of_range": func(t *testing.T) test {
			return test{name: "buy", payload: []byte(`{"quantity":0}`), expectedErrorIs: inputs.ErrInvalidValue}
		},
		"payload_that_is_not_an_object": func(t *testing.T) test {
			return test{name: "buy", payload: []byte(`[1]`), expectedErrorIs: inputs.ErrInvalidValue}
		},
	}

	catalog, err := NewCatalog([]Declaration{
		{Name: "buy", Payload: []inputs.Declaration{
			{Name: "item", Type: inputs.String},
			{Name: "quantity", Type: inputs.Number, Min: number(1), Max: number(10), Default: json.RawMessage(`1`)},
		}},
		{Name: "pass"},
	})
	require.NoError(t, err)
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			payload, err := catalog.Normalize(tc.name, tc.payload)

			if tc.expectedErrorIs != nil {
				require.ErrorIs(t, err, tc.expectedErrorIs)
				return
			}
			require.NoError(t, err)
			require.JSONEq(t, tc.expected, string(payload))
			require.Equal(t, tc.expected, string(payload), "canonical encoding")
		})
	}
}
