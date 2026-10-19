package chunk

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/runtime/internal/business/jsonpatch"
)

func TestEncode(t *testing.T) {
	type test struct {
		chunk Chunk

		expectErr bool
	}

	tests := map[string]func(t *testing.T) test{
		"chunk_round_trips_with_who_sees_each_record": func(t *testing.T) test {
			c := chunkOf(3, 180_000, 240_000, `{"x":[1,null]}`,
				delta(180_100, 10, `[{"op":"remove","path":"/x/1"}]`),
				Record{Type: Event, SessionTimeMs: 180_150, TimelineOrder: 11, Value: json.RawMessage(`"all"`)},
				Record{Type: Event, SessionTimeMs: 180_150, TimelineOrder: 12, Participants: []string{}, Value: json.RawMessage(`"none"`)},
				Record{Type: CommandResponse, SessionTimeMs: 180_200, TimelineOrder: 13, Participants: []string{"P1"}, Value: json.RawMessage(`{"ok":1}`)},
				Record{Type: Error, SessionTimeMs: 180_200, TimelineOrder: 14, Participants: []string{"P2"}, Value: json.RawMessage(`"COOLDOWN"`)},
			)
			c.Final = true
			return test{chunk: c}
		},
		"response_without_its_participant_is_refused": func(t *testing.T) test {
			return test{chunk: chunkOf(0, 0, 0, `{}`, Record{Type: CommandResponse, Value: json.RawMessage(`1`)}), expectErr: true}
		},
		"error_for_two_participants_is_refused": func(t *testing.T) test {
			return test{chunk: chunkOf(0, 0, 0, `{}`, Record{Type: Error, Participants: []string{"P1", "P2"}, Value: json.RawMessage(`1`)}), expectErr: true}
		},
		"delta_with_participants_is_refused": func(t *testing.T) test {
			return test{chunk: chunkOf(0, 0, 0, `{}`, Record{Type: StateDelta, Participants: []string{"P1"}, Value: json.RawMessage(`[]`)}), expectErr: true}
		},
		"unknown_record_type_is_refused": func(t *testing.T) test {
			return test{chunk: chunkOf(0, 0, 0, `{}`, Record{Type: "TIMER", Value: json.RawMessage(`1`)}), expectErr: true}
		},
		"chunk_without_base_state_is_refused": func(t *testing.T) test {
			return test{chunk: Chunk{SessionUUID: header.SessionUUID}, expectErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			stored, err := Encode(tc.chunk)

			if tc.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			decoded, err := Decode(stored)
			require.NoError(t, err)
			require.Equal(t, tc.chunk, decoded)
		})
	}
}

func TestDecodeRefusesOtherFormatVersions(t *testing.T) {
	stored := encoder.EncodeAll([]byte(`{"format_version":2,"base_state":{}}`), nil)

	_, err := Decode(stored)

	require.Error(t, err)
	_, err = Decode([]byte("not zstd"))
	require.Error(t, err)
}

// TestStoredChunkRebuildsEveryKeptState checks the whole path: states built
// into a chunk, stored, read back, and its deltas applied to its base.
func TestStoredChunkRebuildsEveryKeptState(t *testing.T) {
	states := []string{
		`{"ships":[{"id":"a","x":1},{"id":"b","x":2}],"score":{"P1":0}}`,
		`{"ships":[{"id":"b","x":3},{"id":"a","x":1}],"score":{"P1":1,"P2":null}}`,
		`{"ships":[{"id":"b","x":4}],"score":{"P2":5}}`,
		`{"ships":[],"score":{},"over":true}`,
	}
	builder := Open(header, 0, 0, 0, mustState(t, states[0]))
	for i, state := range states[1:] {
		require.NoError(t, builder.AppendState(int64(i+1)*100, mustState(t, state)))
	}
	stored, err := Encode(builder.Seal(300, false))
	require.NoError(t, err)

	decoded, err := Decode(stored)

	require.NoError(t, err)
	current, err := jsonpatch.Parse(decoded.Base)
	require.NoError(t, err)
	rebuilt := []string{mustJSON(t, current)}
	for _, record := range decoded.Records {
		var operations []jsonpatch.Operation
		require.NoError(t, json.Unmarshal(record.Value, &operations))
		current, err = jsonpatch.Apply(current, operations)
		require.NoError(t, err)
		rebuilt = append(rebuilt, mustJSON(t, current))
	}
	require.Len(t, rebuilt, len(states))
	for i := range states {
		require.JSONEq(t, states[i], rebuilt[i])
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return string(encoded)
}
