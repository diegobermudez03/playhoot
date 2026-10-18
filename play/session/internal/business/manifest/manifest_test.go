package manifest

import (
	"testing"

	"github.com/stretchr/testify/require"
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
			return test{raw: `{"schema_version":1,"players":{"min":2,"max":4},"commands":[]}`, expectedManifest: Manifest{MinPlayers: 2, MaxPlayers: 4}}
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
