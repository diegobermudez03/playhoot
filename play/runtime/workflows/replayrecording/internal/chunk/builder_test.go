package chunk

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/runtime/replay"
)

var header = Header{
	SessionUUID:     "5e55a000-0000-4000-8000-000000000001",
	GameVersionUUID: "6a3e0000-0000-4000-8000-000000000001",
	RecordingUUID:   "7ec00000-0000-4000-8000-000000000001",
	Participants:    []string{"P1", "P2"},
}

func mustState(t *testing.T, raw string) State {
	t.Helper()
	state, err := ParseState([]byte(raw))
	require.NoError(t, err)
	return state
}

func chunkOf(index int, from int64, through int64, base string, records ...Record) Chunk {
	return Chunk{
		SessionUUID: header.SessionUUID, GameVersionUUID: header.GameVersionUUID, RecordingUUID: header.RecordingUUID,
		Index: index, FromSessionTimeMs: from, ThroughSessionTimeMs: through,
		Participants: header.Participants, Base: json.RawMessage(base), Records: records,
	}
}

func delta(sessionTimeMs int64, order uint64, patch string) Record {
	return Record{Type: StateDelta, SessionTimeMs: sessionTimeMs, TimelineOrder: order, Value: json.RawMessage(patch)}
}

func TestBuilder(t *testing.T) {
	type test struct {
		// build appends to a chunk opened as index 2, at 120 s, ordered from 40.
		build func(t *testing.T, builder *Builder)
		final bool

		expected          Chunk
		expectedNextOrder uint64
	}

	tests := map[string]func(t *testing.T) test{
		"states_are_deltas_from_the_latest_one_and_unchanged_ones_add_nothing": func(t *testing.T) test {
			return test{
				build: func(t *testing.T, builder *Builder) {
					require.NoError(t, builder.AppendState(120_100, mustState(t, `{"x":1,"l":[1,2]}`)))
					require.NoError(t, builder.AppendState(120_200, mustState(t, `{"l":[1,2],"x":1}`)))
					require.NoError(t, builder.AppendState(120_300, mustState(t, `{"x":null,"l":[2]}`)))
				},
				expected: chunkOf(2, 120_000, 130_000, `{"x":0,"l":[1,2]}`,
					delta(120_100, 40, `[{"op":"replace","path":"/x","value":1}]`),
					delta(120_300, 41, `[{"op":"replace","path":"/l/0","value":2},{"op":"remove","path":"/l/1"},{"op":"replace","path":"/x","value":null}]`),
				),
				expectedNextOrder: 42,
			}
		},
		"records_follow_in_the_order_appended": func(t *testing.T) test {
			response := Record{Type: CommandResponse, SessionTimeMs: 120_050, Participants: []string{"P1"}, Value: json.RawMessage(`"ok"`)}
			event := Record{Type: Event, SessionTimeMs: 120_050, Value: json.RawMessage(`"boom"`)}
			ordered := func(record Record, order uint64) Record {
				record.TimelineOrder = order
				return record
			}
			return test{
				build: func(t *testing.T, builder *Builder) {
					builder.Append([]Record{response, event})
					require.NoError(t, builder.AppendState(120_100, mustState(t, `{"x":5,"l":[1,2]}`)))
				},
				final:             true,
				expected:          chunkOf(2, 120_000, 130_000, `{"x":0,"l":[1,2]}`, ordered(response, 40), ordered(event, 41), delta(120_100, 42, `[{"op":"replace","path":"/x","value":5}]`)),
				expectedNextOrder: 43,
			}
		},
		"chunk_with_only_its_base": func(t *testing.T) test {
			return test{build: func(t *testing.T, builder *Builder) {}, expected: chunkOf(2, 120_000, 130_000, `{"x":0,"l":[1,2]}`), expectedNextOrder: 40}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			builder := Open(header, 2, 40, 120_000, mustState(t, `{"x":0,"l":[1,2]}`))
			tc.build(t, builder)

			sealed := builder.Seal(130_000, tc.final)

			tc.expected.Final = tc.final
			require.Equal(t, tc.expected, sealed)
			require.Equal(t, tc.expectedNextOrder, builder.NextOrder())
		})
	}
}

func TestBuilderSize(t *testing.T) {
	builder := Open(header, 0, 0, 0, mustState(t, `{"x":0}`))
	require.Equal(t, 7, builder.Size())

	builder.Append([]Record{{Type: Event, Value: json.RawMessage(`"abc"`)}})

	require.Equal(t, 12, builder.Size())
}

func TestParseState(t *testing.T) {
	_, err := ParseState([]byte(`{"x":1}`))
	require.NoError(t, err)
	_, err = ParseState([]byte(`{oops`))
	require.Error(t, err)
}

func TestTransients(t *testing.T) {
	type test struct {
		capture replay.Capture

		expected  []Record
		expectErr bool
	}

	tests := map[string]func(t *testing.T) test{
		"same_time_transients_follow_the_timeline_and_update_comes_last": func(t *testing.T) test {
			return test{
				capture: replay.Capture{
					CommandResponses: []replay.CommandResponse{{ParticipantID: "P1", SessionTimeMs: 100, Order: 5, Response: []byte(`"r"`)}},
					Errors:           []replay.GameError{{ParticipantID: "P2", SessionTimeMs: 100, Order: 0, Error: []byte(`"late"`)}, {ParticipantID: "P1", SessionTimeMs: 100, Order: 5, Error: []byte(`"e"`)}},
					Events:           []replay.GameEvent{{SessionTimeMs: 100, Order: 0, Event: []byte(`"tick"`)}, {Participants: []string{}, SessionTimeMs: 100, Order: 3, Event: []byte(`"nobody"`)}, {SessionTimeMs: 50, Order: 9, Event: []byte(`"earlier"`)}},
				},
				expected: []Record{
					{Type: Event, SessionTimeMs: 50, Value: json.RawMessage(`"earlier"`)},
					{Type: Event, SessionTimeMs: 100, Participants: []string{}, Value: json.RawMessage(`"nobody"`)},
					{Type: CommandResponse, SessionTimeMs: 100, Participants: []string{"P1"}, Value: json.RawMessage(`"r"`)},
					{Type: Error, SessionTimeMs: 100, Participants: []string{"P1"}, Value: json.RawMessage(`"e"`)},
					{Type: Error, SessionTimeMs: 100, Participants: []string{"P2"}, Value: json.RawMessage(`"late"`)},
					{Type: Event, SessionTimeMs: 100, Value: json.RawMessage(`"tick"`)},
				},
			}
		},
		"tick_without_transients": func(t *testing.T) test {
			return test{expected: []Record{}}
		},
		"transient_that_is_not_json_fails": func(t *testing.T) test {
			return test{capture: replay.Capture{Events: []replay.GameEvent{{SessionTimeMs: 100, Event: []byte(`{oops`)}}}, expectErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			records, err := Transients(tc.capture)

			if tc.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, records)
		})
	}
}
