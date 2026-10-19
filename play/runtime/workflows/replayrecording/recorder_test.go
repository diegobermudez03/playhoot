package replayrecording

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/play/runtime/replay"
	"github.com/diegobermudez03/playhoot/play/runtime/workflows/replayrecording/internal/chunk"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

const (
	sessionUUID = "5e55a000-0000-4000-8000-000000000001"
	versionUUID = "6a3e0000-0000-4000-8000-000000000001"
)

var startedAt = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestRecorder(store chunkStore) *Recorder {
	recorder := newRecorder(store, Config{StateEveryTicks: 1, ChunkDuration: time.Minute, MaxPendingChunks: 8}, time.Millisecond, quietLogger())
	recorder.now = func() time.Time { return startedAt }
	return recorder
}

func tick(sessionTimeMs int64, state string) replay.Capture {
	return replay.Capture{SessionUUID: sessionUUID, GameVersionUUID: versionUUID, SessionTimeMs: sessionTimeMs, State: []byte(state), Participants: []string{"P1"}}
}

// stored is chunk as the recorder hands it to storage.
func stored(t *testing.T, recordingID string, expected chunk.Chunk) sessionpkg.ReplayChunk {
	t.Helper()
	expected.SessionUUID, expected.GameVersionUUID, expected.RecordingUUID, expected.Participants = sessionUUID, versionUUID, recordingID, []string{"P1"}
	content, err := chunk.Encode(expected)
	require.NoError(t, err)
	return sessionpkg.ReplayChunk{
		RecordingUUID: recordingID, RecordingStartedAt: startedAt, Index: expected.Index, FormatVersion: chunk.FormatVersion,
		FromSessionTimeMs: expected.FromSessionTimeMs, ThroughSessionTimeMs: expected.ThroughSessionTimeMs, Final: expected.Final, Content: content,
	}
}

func deltaRecord(sessionTimeMs int64, order uint64, patch string) chunk.Record {
	return chunk.Record{Type: chunk.StateDelta, SessionTimeMs: sessionTimeMs, TimelineOrder: order, Value: json.RawMessage(patch)}
}

func TestFinish(t *testing.T) {
	type dependencies struct {
		store *MockchunkStore
	}
	type test struct {
		// run makes the calls before the one under test.
		run   func(t *testing.T, recorder *Recorder, recordingID string)
		final replay.Capture

		expectErr bool
	}

	ctx := context.Background()

	tests := map[string]func(t *testing.T, deps dependencies, recorder *Recorder, recordingID string) test{
		"final_chunk_is_stored_before_returning": func(t *testing.T, deps dependencies, recorder *Recorder, recordingID string) test {
			deps.store.EXPECT().StoreReplayChunk(ctx, sessionUUID, stored(t, recordingID, chunk.Chunk{
				FromSessionTimeMs: 0, ThroughSessionTimeMs: 100, Final: true, Base: json.RawMessage(`{"x":0}`),
				Records: []chunk.Record{deltaRecord(100, 0, `[{"op":"replace","path":"/x","value":"end"}]`)},
			})).Return(nil)
			return test{
				run: func(t *testing.T, recorder *Recorder, recordingID string) {
					recorder.Offer(ctx, recordingID, tick(0, `{"x":0}`))
				},
				final: tick(100, `{"x":"end"}`),
			}
		},
		"final_tick_past_the_chunk_end_closes_the_last_chunk": func(t *testing.T, deps dependencies, recorder *Recorder, recordingID string) test {
			deps.store.EXPECT().StoreReplayChunk(ctx, sessionUUID, stored(t, recordingID, chunk.Chunk{
				FromSessionTimeMs: 59_000, ThroughSessionTimeMs: 61_000, Final: true, Base: json.RawMessage(`{"x":0}`),
				Records: []chunk.Record{deltaRecord(61_000, 0, `[{"op":"replace","path":"/x","value":"end"}]`)},
			})).Return(nil)
			return test{
				run: func(t *testing.T, recorder *Recorder, recordingID string) {
					recorder.Offer(ctx, recordingID, tick(59_000, `{"x":0}`))
				},
				final: tick(61_000, `{"x":"end"}`),
			}
		},
		"failed_store_fails_and_the_next_call_stores_the_same_chunk": func(t *testing.T, deps dependencies, recorder *Recorder, recordingID string) test {
			final := stored(t, recordingID, chunk.Chunk{FromSessionTimeMs: 100, ThroughSessionTimeMs: 100, Final: true, Base: json.RawMessage(`{}`)})
			gomock.InOrder(
				deps.store.EXPECT().StoreReplayChunk(ctx, sessionUUID, final).Return(errors.New("storage down")),
				deps.store.EXPECT().StoreReplayChunk(ctx, sessionUUID, final).Return(nil),
			)
			return test{
				run: func(t *testing.T, recorder *Recorder, recordingID string) {
					require.Error(t, recorder.Finish(ctx, recordingID, tick(100, `{}`)))
				},
				final: tick(100, `{"ignored":"the first final tick is kept"}`),
			}
		},
		"stored_ending_answers_success_again_without_storing": func(t *testing.T, deps dependencies, recorder *Recorder, recordingID string) test {
			deps.store.EXPECT().StoreReplayChunk(ctx, sessionUUID, stored(t, recordingID, chunk.Chunk{
				FromSessionTimeMs: 100, ThroughSessionTimeMs: 100, Final: true, Base: json.RawMessage(`{}`),
			})).Return(nil)
			return test{
				run: func(t *testing.T, recorder *Recorder, recordingID string) {
					require.NoError(t, recorder.Finish(ctx, recordingID, tick(100, `{}`)))
				},
				final: tick(100, `{}`),
			}
		},
		"final_state_that_is_not_json_fails": func(t *testing.T, deps dependencies, recorder *Recorder, recordingID string) test {
			return test{final: tick(100, `{oops`), expectErr: true}
		},
		"stopped_recording_is_unknown": func(t *testing.T, deps dependencies, recorder *Recorder, recordingID string) test {
			return test{
				run:       func(t *testing.T, recorder *Recorder, recordingID string) { recorder.StopRecording(recordingID) },
				final:     tick(100, `{}`),
				expectErr: true,
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{store: NewMockchunkStore(ctrl)}
			recorder := newTestRecorder(deps.store)
			recordingID := recorder.StartRecording(sessionUUID)
			tc := setup(t, deps, recorder, recordingID)
			if tc.run != nil {
				tc.run(t, recorder, recordingID)
			}

			err := recorder.Finish(ctx, recordingID, tc.final)

			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			recorder.StopRecording(recordingID)
			recorder.Close()
		})
	}
}

func TestStopRecording(t *testing.T) {
	type dependencies struct {
		store *MockchunkStore
	}
	type test struct {
		ticks []replay.Capture
	}

	ctx := context.Background()

	tests := map[string]func(t *testing.T, deps dependencies, recorder *Recorder, recordingID string) test{
		"active_chunk_is_stored_in_the_background": func(t *testing.T, deps dependencies, recorder *Recorder, recordingID string) test {
			deps.store.EXPECT().StoreReplayChunk(recorder.queue.ctx, sessionUUID, stored(t, recordingID, chunk.Chunk{
				FromSessionTimeMs: 0, ThroughSessionTimeMs: 200, Base: json.RawMessage(`{"x":0}`),
				Records: []chunk.Record{deltaRecord(200, 0, `[{"op":"replace","path":"/x","value":2}]`)},
			})).Return(nil)
			return test{ticks: []replay.Capture{tick(0, `{"x":0}`), tick(100, `{"x":0}`), tick(200, `{"x":2}`)}}
		},
		"chunks_sealed_by_game_time_and_the_last_one_are_all_stored": func(t *testing.T, deps dependencies, recorder *Recorder, recordingID string) test {
			deps.store.EXPECT().StoreReplayChunk(recorder.queue.ctx, sessionUUID, stored(t, recordingID, chunk.Chunk{
				FromSessionTimeMs: 0, ThroughSessionTimeMs: 60_000, Base: json.RawMessage(`{"x":0}`),
			})).Return(nil)
			deps.store.EXPECT().StoreReplayChunk(recorder.queue.ctx, sessionUUID, stored(t, recordingID, chunk.Chunk{
				Index: 1, FromSessionTimeMs: 60_000, ThroughSessionTimeMs: 120_000, Base: json.RawMessage(`{"x":1}`),
			})).Return(nil)
			deps.store.EXPECT().StoreReplayChunk(recorder.queue.ctx, sessionUUID, stored(t, recordingID, chunk.Chunk{
				Index: 2, FromSessionTimeMs: 120_000, ThroughSessionTimeMs: 137_400, Base: json.RawMessage(`{"x":2}`),
				Records: []chunk.Record{deltaRecord(137_400, 0, `[{"op":"replace","path":"/x","value":3}]`)},
			})).Return(nil)
			return test{ticks: []replay.Capture{tick(0, `{"x":0}`), tick(60_000, `{"x":1}`), tick(120_000, `{"x":2}`), tick(137_400, `{"x":3}`)}}
		},
		"final_chunk_never_stored_is_stored_in_the_background": func(t *testing.T, deps dependencies, recorder *Recorder, recordingID string) test {
			final := stored(t, recordingID, chunk.Chunk{FromSessionTimeMs: 0, ThroughSessionTimeMs: 0, Final: true, Base: json.RawMessage(`{}`)})
			gomock.InOrder(
				deps.store.EXPECT().StoreReplayChunk(ctx, sessionUUID, final).Return(errors.New("storage down")),
				deps.store.EXPECT().StoreReplayChunk(recorder.queue.ctx, sessionUUID, final).Return(nil),
			)
			require.Error(t, recorder.Finish(ctx, recordingID, tick(0, `{}`)))
			return test{}
		},
		"recording_without_ticks_stores_nothing": func(t *testing.T, deps dependencies, recorder *Recorder, recordingID string) test {
			return test{}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{store: NewMockchunkStore(ctrl)}
			recorder := newTestRecorder(deps.store)
			recordingID := recorder.StartRecording(sessionUUID)
			tc := setup(t, deps, recorder, recordingID)
			for _, capture := range tc.ticks {
				recorder.Offer(ctx, recordingID, capture)
			}

			recorder.StopRecording(recordingID)

			// Close waits for the background stores the mock expects.
			recorder.Close()
		})
	}
}

func TestStartRecordingAfterClose(t *testing.T) {
	recorder := newTestRecorder(NewMockchunkStore(gomock.NewController(t)))
	recorder.Close()

	recordingID := recorder.StartRecording(sessionUUID)

	require.Empty(t, recordingID)
	recorder.Offer(context.Background(), recordingID, tick(0, `{}`))
	require.ErrorIs(t, recorder.Finish(context.Background(), recordingID, tick(0, `{}`)), errUnknownRecording)
}
