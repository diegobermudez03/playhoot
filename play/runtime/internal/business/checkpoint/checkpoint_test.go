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
					`"executed_through_session_time_ms":0,"state":{"round":1},"prng_state":"AQID","participants":["P1","P2"]}`,
			}
		},
		"processed_command_tokens_are_stored_per_participant": func(t *testing.T) test {
			return test{
				checkpoint: Checkpoint{
					ContextRevision:        7,
					State:                  []byte(`{}`),
					Participants:           []string{"P1"},
					ProcessedCommandTokens: map[string][]string{"P1": {"t1", "t2"}},
				},
				expected: `{"schema_version":1,"game_version_uuid":"","context_revision":7,"executed_through_session_time_ms":0,` +
					`"state":{},"prng_state":null,"participants":["P1"],"processed_command_tokens":{"P1":["t1","t2"]}}`,
			}
		},
		"disconnected_participants_are_stored": func(t *testing.T) test {
			return test{
				checkpoint: Checkpoint{ContextRevision: 3, State: []byte(`{}`), Participants: []string{"P1", "P2"}, Disconnected: []string{"P2"}},
				expected: `{"schema_version":1,"game_version_uuid":"","context_revision":3,"executed_through_session_time_ms":0,` +
					`"state":{},"prng_state":null,"participants":["P1","P2"],"disconnected":["P2"]}`,
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

func TestDecode(t *testing.T) {
	type test struct {
		encoded string

		expected    Checkpoint
		expectedErr bool
	}

	tests := map[string]func(t *testing.T) test{
		"what_encode_wrote": func(t *testing.T) test {
			checkpoint := Checkpoint{GameVersionUUID: "c1d2e3f4-0000-4000-8000-000000000001", ContextRevision: 4, ExecutedThroughSessionTimeMs: 1500, State: []byte(`{"round":2}`), PRNGState: []byte{1, 2, 3, 4}, Participants: []string{"P1", "P2"}, ProcessedCommandTokens: map[string][]string{"P2": {"t1"}}, Disconnected: []string{"P1"}}
			encoded, err := Encode(checkpoint)
			require.NoError(t, err)
			return test{encoded: string(encoded), expected: checkpoint}
		},
		"another_schema_version": func(t *testing.T) test {
			return test{encoded: `{"schema_version":2,"state":{}}`, expectedErr: true}
		},
		"not_json": func(t *testing.T) test {
			return test{encoded: `nope`, expectedErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			checkpoint, err := Decode([]byte(tc.encoded))

			if tc.expectedErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.JSONEq(t, string(tc.expected.State), string(checkpoint.State))
			checkpoint.State = tc.expected.State
			require.Equal(t, tc.expected, checkpoint)
		})
	}
}
