package checkpoint

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncode(t *testing.T) {
	type test struct {
		checkpoint Checkpoint

		expected    string
		expectedErr bool
	}

	tests := map[string]func(t *testing.T) test{
		"every_field_is_stored_under_the_schema_version": func(t *testing.T) test {
			return test{
				checkpoint: Checkpoint{
					GameVersionUUID: "c1d2e3f4-0000-4000-8000-000000000001",
					ContextRevision: 1,
					State:           []byte(`{"round":1}`),
					PRNGState:       []byte{1, 2, 3},
					Participants:    []string{"P1", "P2"},
				},
				expected: `{"schema_version":1,"game_version_uuid":"c1d2e3f4-0000-4000-8000-000000000001","context_revision":1,` +
					`"state":{"round":1},"prng_state":"AQID","participants":["P1","P2"]}`,
			}
		},
		"state_that_is_not_json_fails": func(t *testing.T) test {
			return test{
				checkpoint:  Checkpoint{State: []byte(`not json`)},
				expectedErr: true,
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			encoded, err := Encode(tc.checkpoint)

			if tc.expectedErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.JSONEq(t, tc.expected, string(encoded))
		})
	}
}
