package manifest

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/internal/business/commands"
	"github.com/diegobermudez03/playhoot/play/internal/business/inputs"
)

func TestParse(t *testing.T) {
	type test struct {
		raw string

		expectedManifest Manifest
		expectedErr      bool
	}

	tests := map[string]func(t *testing.T) test{
		"valid": func(t *testing.T) test {
			return test{raw: `{"schema_version":1,"players":{"min":2,"max":8}}`, expectedManifest: Manifest{MinPlayers: 2, MaxPlayers: 8}}
		},
		"single_player_game": func(t *testing.T) test {
			return test{raw: `{"schema_version":1,"players":{"min":1,"max":1}}`, expectedManifest: Manifest{MinPlayers: 1, MaxPlayers: 1}}
		},
		"unknown_fields_are_ignored": func(t *testing.T) test {
			return test{raw: `{"schema_version":1,"players":{"min":2,"max":4},"entrypoints":[]}`, expectedManifest: Manifest{MinPlayers: 2, MaxPlayers: 4}}
		},
		"inputs_with_their_defaults": func(t *testing.T) test {
			return test{
				raw: `{"schema_version":1,"players":{"min":1,"max":4},"inputs":{"move":{"type":"vector2"},"mode":{"type":"enum","values":["walk","run"],"default":"run"}}}`,
				expectedManifest: Manifest{MinPlayers: 1, MaxPlayers: 4, Inputs: []inputs.Declaration{
					{Name: "mode", Type: inputs.Enum, Values: []string{"walk", "run"}, Default: json.RawMessage(`"run"`)},
					{Name: "move", Type: inputs.Vector2, Default: json.RawMessage(`[0,0]`)},
				}},
			}
		},
		"commands_with_their_payload_defaults": func(t *testing.T) test {
			return test{
				raw: `{"schema_version":1,"players":{"min":1,"max":4},"commands":{"pass":{},"play_card":{"payload":{"card":{"type":"string"}}}}}`,
				expectedManifest: Manifest{MinPlayers: 1, MaxPlayers: 4, Commands: []commands.Declaration{
					{Name: "pass"},
					{Name: "play_card", Payload: []inputs.Declaration{{Name: "card", Type: inputs.String, Default: json.RawMessage(`""`)}}},
				}},
			}
		},
		"invalid_command_payload": func(t *testing.T) test {
			return test{raw: `{"schema_version":1,"players":{"min":1,"max":4},"commands":{"pick":{"payload":{"mode":{"type":"enum","values":["a"]}}}}}`, expectedErr: true}
		},
		"invalid_command_name": func(t *testing.T) test {
			return test{raw: `{"schema_version":1,"players":{"min":1,"max":4},"commands":{"play card":{}}}`, expectedErr: true}
		},
		"invalid_input": func(t *testing.T) test {
			return test{raw: `{"schema_version":1,"players":{"min":1,"max":4},"inputs":{"mode":{"type":"enum","values":["walk"]}}}`, expectedErr: true}
		},
		"other_schema_version": func(t *testing.T) test {
			return test{raw: `{"schema_version":2,"players":{"min":2,"max":8}}`, expectedErr: true}
		},
		"missing_players": func(t *testing.T) test {
			return test{raw: `{"schema_version":1}`, expectedErr: true}
		},
		"min_below_one": func(t *testing.T) test {
			return test{raw: `{"schema_version":1,"players":{"min":0,"max":8}}`, expectedErr: true}
		},
		"max_below_min": func(t *testing.T) test {
			return test{raw: `{"schema_version":1,"players":{"min":4,"max":2}}`, expectedErr: true}
		},
		"not_json": func(t *testing.T) test {
			return test{raw: `not json`, expectedErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			manifest, err := Parse([]byte(tc.raw))

			if tc.expectedErr {
				require.ErrorIs(t, err, ErrInvalid)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expectedManifest, manifest)
		})
	}
}
