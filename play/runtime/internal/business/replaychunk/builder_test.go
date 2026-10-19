package replaychunk

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/runtime/replay"
)

const (
	sessionUUID   = "5e55a000-0000-4000-8000-000000000001"
	versionUUID   = "6a3e0000-0000-4000-8000-000000000001"
	recordingUUID = "7ec00000-0000-4000-8000-000000000001"
)

var participants = []string{"P1", "P2"}

func tick(sessionTimeMs int64, state string) replay.Capture {
	return replay.Capture{SessionUUID: sessionUUID, GameVersionUUID: versionUUID, SessionTimeMs: sessionTimeMs, State: []byte(state), Participants: participants}
}

func chunk(index int, from int64, through int64, base string, records ...Record) Chunk {
	return Chunk{
		SessionUUID: sessionUUID, GameVersionUUID: versionUUID, RecordingUUID: recordingUUID,
		Index: index, FromSessionTimeMs: from, ThroughSessionTimeMs: through,
		Participants: participants, Base: json.RawMessage(base), Records: records,
	}
}

func delta(sessionTimeMs int64, order uint64, patch string) Record {
	return Record{Type: StateDelta, SessionTimeMs: sessionTimeMs, TimelineOrder: order, Value: json.RawMessage(patch)}
}

func TestAdd(t *testing.T) {
	type test struct {
		settings Settings
		captures []replay.Capture

		expectedSealed []Chunk
		expectedActive *Chunk
	}

	minute := Settings{StateEveryTicks: 1, ChunkDuration: time.Minute}

	tests := map[string]func(t *testing.T) test{
		"every_tick_kept_as_a_delta_from_the_previous": func(t *testing.T) test {
			active := chunk(0, 0, 300, `{"x":0}`,
				delta(100, 0, `[{"op":"replace","path":"/x","value":1}]`),
				delta(300, 1, `[{"op":"replace","path":"/x","value":3}]`),
			)
			return test{
				settings:       minute,
				captures:       []replay.Capture{tick(0, `{"x":0}`), tick(100, `{"x":1}`), tick(200, `{"x":1}`), tick(300, `{"x":3}`)},
				expectedActive: &active,
			}
		},
		"one_tick_in_three_keeps_its_state_and_every_transient": func(t *testing.T) test {
			explosion := tick(200, `{"x":2}`)
			explosion.Events = []replay.GameEvent{{SessionTimeMs: 150, Order: 4, Event: []byte(`"boom"`)}}
			answered := tick(400, `{"x":4}`)
			answered.CommandResponses = []replay.CommandResponse{{ParticipantID: "P2", SessionTimeMs: 380, Order: 9, Response: []byte(`{"ok":true}`)}}
			active := chunk(0, 0, 500, `{"x":0}`,
				Record{Type: Event, SessionTimeMs: 150, TimelineOrder: 0, Value: json.RawMessage(`"boom"`)},
				delta(300, 1, `[{"op":"replace","path":"/x","value":3}]`),
				Record{Type: CommandResponse, SessionTimeMs: 380, TimelineOrder: 2, Participants: []string{"P2"}, Value: json.RawMessage(`{"ok":true}`)},
			)
			return test{
				settings:       Settings{StateEveryTicks: 3, ChunkDuration: time.Minute},
				captures:       []replay.Capture{tick(0, `{"x":0}`), tick(100, `{"x":1}`), explosion, tick(300, `{"x":3}`), answered, tick(500, `{"x":5}`)},
				expectedActive: &active,
			}
		},
		"a_tick_past_the_chunk_end_seals_it_and_starts_the_next_with_a_full_state": func(t *testing.T) test {
			crossing := tick(60_050, `{"x":2}`)
			crossing.Events = []replay.GameEvent{{Participants: []string{"P1"}, SessionTimeMs: 59_990, Order: 7, Event: []byte(`1`)}}
			sealed := chunk(0, 59_900, 60_050, `{"x":1}`,
				Record{Type: Event, SessionTimeMs: 59_990, TimelineOrder: 0, Participants: []string{"P1"}, Value: json.RawMessage(`1`)},
			)
			active := chunk(1, 60_050, 60_150, `{"x":2}`, delta(60_150, 1, `[{"op":"add","path":"/y","value":null}]`))
			return test{
				settings:       minute,
				captures:       []replay.Capture{tick(59_900, `{"x":1}`), crossing, tick(60_150, `{"x":2,"y":null}`)},
				expectedSealed: []Chunk{sealed},
				expectedActive: &active,
			}
		},
		"a_chunk_reaching_its_size_limit_is_sealed_early": func(t *testing.T) test {
			sealed := chunk(0, 0, 100, `{"x":"aaaaaaaaaa"}`)
			active := chunk(1, 100, 100, `{"x":"bbbbbbbbbb"}`)
			return test{
				settings:       Settings{StateEveryTicks: 1, ChunkDuration: time.Minute, MaxChunkBytes: 10},
				captures:       []replay.Capture{tick(0, `{"x":"aaaaaaaaaa"}`), tick(100, `{"x":"bbbbbbbbbb"}`)},
				expectedSealed: []Chunk{sealed},
				expectedActive: &active,
			}
		},
		"same_time_transients_follow_the_timeline_and_update_comes_last": func(t *testing.T) test {
			busy := tick(100, `{}`)
			busy.CommandResponses = []replay.CommandResponse{{ParticipantID: "P1", SessionTimeMs: 100, Order: 5, Response: []byte(`"r"`)}}
			busy.Errors = []replay.GameError{{ParticipantID: "P2", SessionTimeMs: 100, Order: 0, Error: []byte(`"late"`)}, {ParticipantID: "P1", SessionTimeMs: 100, Order: 5, Error: []byte(`"e"`)}}
			busy.Events = []replay.GameEvent{{SessionTimeMs: 100, Order: 0, Event: []byte(`"tick"`)}, {Participants: []string{}, SessionTimeMs: 100, Order: 3, Event: []byte(`"nobody"`)}}
			active := chunk(0, 0, 100, `{}`,
				Record{Type: Event, SessionTimeMs: 100, TimelineOrder: 0, Participants: []string{}, Value: json.RawMessage(`"nobody"`)},
				Record{Type: CommandResponse, SessionTimeMs: 100, TimelineOrder: 1, Participants: []string{"P1"}, Value: json.RawMessage(`"r"`)},
				Record{Type: Error, SessionTimeMs: 100, TimelineOrder: 2, Participants: []string{"P1"}, Value: json.RawMessage(`"e"`)},
				Record{Type: Error, SessionTimeMs: 100, TimelineOrder: 3, Participants: []string{"P2"}, Value: json.RawMessage(`"late"`)},
				Record{Type: Event, SessionTimeMs: 100, TimelineOrder: 4, Value: json.RawMessage(`"tick"`)},
			)
			return test{settings: minute, captures: []replay.Capture{tick(0, `{}`), busy}, expectedActive: &active}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			builder := NewBuilder(recordingUUID, tc.settings)

			var sealed []Chunk
			for _, capture := range tc.captures {
				closed, err := builder.Add(capture)
				require.NoError(t, err)
				if closed != nil {
					sealed = append(sealed, *closed)
				}
			}

			require.Equal(t, tc.expectedSealed, sealed)
			require.Equal(t, tc.expectedActive, builder.Seal())
		})
	}
}

func TestAddRejectsWhatIsNotJSON(t *testing.T) {
	builder := NewBuilder(recordingUUID, Settings{StateEveryTicks: 1, ChunkDuration: time.Minute})
	_, err := builder.Add(tick(0, `{}`))
	require.NoError(t, err)
	bad := tick(100, `{"x":1}`)
	bad.Events = []replay.GameEvent{{SessionTimeMs: 100, Event: []byte(`{oops`)}}

	_, err = builder.Add(bad)
	require.Error(t, err)
	_, err = builder.Add(tick(200, `{not json`))
	require.Error(t, err)

	// A rejected tick leaves no trace.
	expected := chunk(0, 0, 0, `{}`)
	require.Equal(t, &expected, builder.Seal())
}

func TestFinish(t *testing.T) {
	type test struct {
		captures []replay.Capture
		final    replay.Capture

		expectedFinal Chunk
	}

	tests := map[string]func(t *testing.T) test{
		"final_state_is_kept_even_on_a_skipped_tick": func(t *testing.T) test {
			final := chunk(0, 0, 100, `{"x":0}`, delta(100, 0, `[{"op":"replace","path":"/x","value":"end"}]`))
			final.Final = true
			return test{captures: []replay.Capture{tick(0, `{"x":0}`)}, final: tick(100, `{"x":"end"}`), expectedFinal: final}
		},
		"final_tick_past_the_chunk_end_closes_the_last_chunk_with_its_transients": func(t *testing.T) test {
			over := tick(61_000, `{"x":"end"}`)
			over.Events = []replay.GameEvent{{SessionTimeMs: 61_000, Event: []byte(`"GAME_OVER"`)}}
			final := chunk(0, 59_000, 61_000, `{"x":0}`,
				Record{Type: Event, SessionTimeMs: 61_000, TimelineOrder: 0, Value: json.RawMessage(`"GAME_OVER"`)},
				delta(61_000, 1, `[{"op":"replace","path":"/x","value":"end"}]`),
			)
			final.Final = true
			return test{captures: []replay.Capture{tick(59_000, `{"x":0}`)}, final: over, expectedFinal: final}
		},
		"game_finished_on_its_first_recorded_tick": func(t *testing.T) test {
			final := chunk(0, 137_400, 137_400, `{"x":"end"}`)
			final.Final = true
			return test{final: tick(137_400, `{"x":"end"}`), expectedFinal: final}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			builder := NewBuilder(recordingUUID, Settings{StateEveryTicks: 5, ChunkDuration: time.Minute})
			for _, capture := range tc.captures {
				_, err := builder.Add(capture)
				require.NoError(t, err)
			}

			final, err := builder.Finish(tc.final)

			require.NoError(t, err)
			require.Equal(t, tc.expectedFinal, final)
			require.Nil(t, builder.Seal())
		})
	}
}
