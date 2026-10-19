package replayrecording

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/runtime/replay"
	"github.com/diegobermudez03/playhoot/play/runtime/workflows/replayrecording/internal/chunk"
)

const chunkingRecordingUUID = "7ec00000-0000-4000-8000-000000000001"

var chunkingParticipants = []string{"P1", "P2"}

func chunkingTick(sessionTimeMs int64, state string) replay.Capture {
	return replay.Capture{SessionUUID: sessionUUID, GameVersionUUID: versionUUID, SessionTimeMs: sessionTimeMs, State: []byte(state), Participants: chunkingParticipants}
}

func expectedChunk(index int, from int64, through int64, base string, records ...chunk.Record) chunk.Chunk {
	return chunk.Chunk{
		SessionUUID: sessionUUID, GameVersionUUID: versionUUID, RecordingUUID: chunkingRecordingUUID,
		Index: index, FromSessionTimeMs: from, ThroughSessionTimeMs: through,
		Participants: chunkingParticipants, Base: json.RawMessage(base), Records: records,
	}
}

func TestChunkingAdd(t *testing.T) {
	type test struct {
		policy chunkingPolicy
		ticks  []replay.Capture

		expectedSealed []chunk.Chunk
		expectedActive *chunk.Chunk
	}

	minute := chunkingPolicy{stateEveryTicks: 1, chunkDuration: time.Minute}

	tests := map[string]func(t *testing.T) test{
		"every_tick_kept_as_a_delta_from_the_previous": func(t *testing.T) test {
			active := expectedChunk(0, 0, 300, `{"x":0}`,
				deltaRecord(100, 0, `[{"op":"replace","path":"/x","value":1}]`),
				deltaRecord(300, 1, `[{"op":"replace","path":"/x","value":3}]`),
			)
			return test{
				policy:         minute,
				ticks:          []replay.Capture{chunkingTick(0, `{"x":0}`), chunkingTick(100, `{"x":1}`), chunkingTick(200, `{"x":1}`), chunkingTick(300, `{"x":3}`)},
				expectedActive: &active,
			}
		},
		"one_tick_in_three_keeps_its_state_and_every_transient": func(t *testing.T) test {
			explosion := chunkingTick(200, `{"x":2}`)
			explosion.Events = []replay.GameEvent{{SessionTimeMs: 150, Order: 4, Event: []byte(`"boom"`)}}
			answered := chunkingTick(400, `{"x":4}`)
			answered.CommandResponses = []replay.CommandResponse{{ParticipantID: "P2", SessionTimeMs: 380, Order: 9, Response: []byte(`{"ok":true}`)}}
			active := expectedChunk(0, 0, 500, `{"x":0}`,
				chunk.Record{Type: chunk.Event, SessionTimeMs: 150, TimelineOrder: 0, Value: json.RawMessage(`"boom"`)},
				deltaRecord(300, 1, `[{"op":"replace","path":"/x","value":3}]`),
				chunk.Record{Type: chunk.CommandResponse, SessionTimeMs: 380, TimelineOrder: 2, Participants: []string{"P2"}, Value: json.RawMessage(`{"ok":true}`)},
			)
			return test{
				policy:         chunkingPolicy{stateEveryTicks: 3, chunkDuration: time.Minute},
				ticks:          []replay.Capture{chunkingTick(0, `{"x":0}`), chunkingTick(100, `{"x":1}`), explosion, chunkingTick(300, `{"x":3}`), answered, chunkingTick(500, `{"x":5}`)},
				expectedActive: &active,
			}
		},
		"a_tick_past_the_chunk_end_seals_it_and_starts_the_next_with_a_full_state": func(t *testing.T) test {
			crossing := chunkingTick(60_050, `{"x":2}`)
			crossing.Events = []replay.GameEvent{{Participants: []string{"P1"}, SessionTimeMs: 59_990, Order: 7, Event: []byte(`1`)}}
			sealed := expectedChunk(0, 59_900, 60_050, `{"x":1}`,
				chunk.Record{Type: chunk.Event, SessionTimeMs: 59_990, TimelineOrder: 0, Participants: []string{"P1"}, Value: json.RawMessage(`1`)},
			)
			active := expectedChunk(1, 60_050, 60_150, `{"x":2}`, deltaRecord(60_150, 1, `[{"op":"add","path":"/y","value":null}]`))
			return test{
				policy:         minute,
				ticks:          []replay.Capture{chunkingTick(59_900, `{"x":1}`), crossing, chunkingTick(60_150, `{"x":2,"y":null}`)},
				expectedSealed: []chunk.Chunk{sealed},
				expectedActive: &active,
			}
		},
		"a_chunk_reaching_its_size_limit_is_sealed_early": func(t *testing.T) test {
			sealed := expectedChunk(0, 0, 100, `{"x":"aaaaaaaaaa"}`)
			active := expectedChunk(1, 100, 100, `{"x":"bbbbbbbbbb"}`)
			return test{
				policy:         chunkingPolicy{stateEveryTicks: 1, chunkDuration: time.Minute, maxChunkBytes: 10},
				ticks:          []replay.Capture{chunkingTick(0, `{"x":"aaaaaaaaaa"}`), chunkingTick(100, `{"x":"bbbbbbbbbb"}`)},
				expectedSealed: []chunk.Chunk{sealed},
				expectedActive: &active,
			}
		},
		"a_tick_that_is_not_json_leaves_no_trace": func(t *testing.T) test {
			bad := chunkingTick(100, `{"x":1}`)
			bad.Events = []replay.GameEvent{{SessionTimeMs: 100, Event: []byte(`{oops`)}}
			active := expectedChunk(0, 0, 0, `{}`)
			return test{policy: minute, ticks: []replay.Capture{chunkingTick(0, `{}`), bad, chunkingTick(200, `{not json`)}, expectedActive: &active}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			chunks := newChunking(chunkingRecordingUUID, tc.policy)

			var sealed []chunk.Chunk
			for _, capture := range tc.ticks {
				closed, err := chunks.add(capture)
				if err != nil {
					continue
				}
				if closed != nil {
					sealed = append(sealed, *closed)
				}
			}

			require.Equal(t, tc.expectedSealed, sealed)
			require.Equal(t, tc.expectedActive, chunks.seal())
		})
	}
}

func TestChunkingFinish(t *testing.T) {
	type test struct {
		ticks []replay.Capture
		final replay.Capture

		expectedFinal chunk.Chunk
		expectErr     bool
	}

	tests := map[string]func(t *testing.T) test{
		"final_state_is_kept_even_on_a_skipped_tick": func(t *testing.T) test {
			final := expectedChunk(0, 0, 100, `{"x":0}`, deltaRecord(100, 0, `[{"op":"replace","path":"/x","value":"end"}]`))
			final.Final = true
			return test{ticks: []replay.Capture{chunkingTick(0, `{"x":0}`)}, final: chunkingTick(100, `{"x":"end"}`), expectedFinal: final}
		},
		"final_tick_past_the_chunk_end_closes_the_last_chunk_with_its_transients": func(t *testing.T) test {
			over := chunkingTick(61_000, `{"x":"end"}`)
			over.Events = []replay.GameEvent{{SessionTimeMs: 61_000, Event: []byte(`"GAME_OVER"`)}}
			final := expectedChunk(0, 59_000, 61_000, `{"x":0}`,
				chunk.Record{Type: chunk.Event, SessionTimeMs: 61_000, TimelineOrder: 0, Value: json.RawMessage(`"GAME_OVER"`)},
				deltaRecord(61_000, 1, `[{"op":"replace","path":"/x","value":"end"}]`),
			)
			final.Final = true
			return test{ticks: []replay.Capture{chunkingTick(59_000, `{"x":0}`)}, final: over, expectedFinal: final}
		},
		"game_finished_on_its_first_recorded_tick": func(t *testing.T) test {
			final := expectedChunk(0, 137_400, 137_400, `{"x":"end"}`)
			final.Final = true
			return test{final: chunkingTick(137_400, `{"x":"end"}`), expectedFinal: final}
		},
		"final_state_that_is_not_json_fails": func(t *testing.T) test {
			return test{ticks: []replay.Capture{chunkingTick(0, `{}`)}, final: chunkingTick(100, `{oops`), expectErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			chunks := newChunking(chunkingRecordingUUID, chunkingPolicy{stateEveryTicks: 5, chunkDuration: time.Minute})
			for _, capture := range tc.ticks {
				_, err := chunks.add(capture)
				require.NoError(t, err)
			}

			final, err := chunks.finish(tc.final)

			if tc.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expectedFinal, final)
			require.Nil(t, chunks.seal())
		})
	}
}
