package replayrecording

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

func chunkNumber(index int) sealedChunk {
	return sealedChunk{sessionUUID: sessionUUID, chunk: sessionpkg.ReplayChunk{RecordingUUID: "r", Index: index, Content: []byte{byte(index)}}}
}

func quietLog(context.Context, string) {}

func TestAdd(t *testing.T) {
	type test struct {
		added []sealedChunk

		expectedPending []sealedChunk
	}

	tests := map[string]func(t *testing.T) test{
		"chunks_wait_in_order": func(t *testing.T) test {
			return test{added: []sealedChunk{chunkNumber(0), chunkNumber(1)}, expectedPending: []sealedChunk{chunkNumber(0), chunkNumber(1)}}
		},
		"full_queue_drops_its_oldest_chunk": func(t *testing.T) test {
			return test{
				added:           []sealedChunk{chunkNumber(0), chunkNumber(1), chunkNumber(2), chunkNumber(3)},
				expectedPending: []sealedChunk{chunkNumber(1), chunkNumber(2), chunkNumber(3)},
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			// Without workers nothing is taken, so the queue only fills.
			queue := newStoreQueue(NewMockchunkStore(gomock.NewController(t)), 3, 0, 1, time.Millisecond, quietLog)

			for _, sealed := range tc.added {
				queue.add(sealed)
			}

			require.Equal(t, tc.expectedPending, queue.pending)
			queue.drain(time.Second)
			require.Empty(t, queue.pending)
		})
	}
}

func TestStoreWithRetries(t *testing.T) {
	type dependencies struct {
		store *MockchunkStore
	}

	sealed := chunkNumber(4)

	tests := map[string]func(t *testing.T, deps dependencies, queue *storeQueue){
		"stored_at_once": func(t *testing.T, deps dependencies, queue *storeQueue) {
			deps.store.EXPECT().StoreReplayChunk(queue.ctx, sessionUUID, sealed.chunk).Return(nil)
		},
		"stored_after_failures": func(t *testing.T, deps dependencies, queue *storeQueue) {
			gomock.InOrder(
				deps.store.EXPECT().StoreReplayChunk(queue.ctx, sessionUUID, sealed.chunk).Return(errors.New("storage down")),
				deps.store.EXPECT().StoreReplayChunk(queue.ctx, sessionUUID, sealed.chunk).Return(errors.New("storage down")),
				deps.store.EXPECT().StoreReplayChunk(queue.ctx, sessionUUID, sealed.chunk).Return(nil),
			)
		},
		"dropped_after_its_last_attempt": func(t *testing.T, deps dependencies, queue *storeQueue) {
			deps.store.EXPECT().StoreReplayChunk(queue.ctx, sessionUUID, sealed.chunk).Return(errors.New("storage down")).Times(3)
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{store: NewMockchunkStore(ctrl)}
			queue := newStoreQueue(deps.store, 3, 0, 3, time.Millisecond, quietLog)
			setup(t, deps, queue)

			queue.storeWithRetries(sealed)

			queue.drain(time.Second)
		})
	}
}

func TestDrain(t *testing.T) {
	ctrl := gomock.NewController(t)
	store := NewMockchunkStore(ctrl)
	queue := newStoreQueue(store, 8, 1, 3, time.Hour, quietLog)
	blocked := make(chan struct{})
	// The first chunk fails, so its worker waits an hour before retrying:
	// the drain deadline cuts it short and the rest are dropped.
	store.EXPECT().StoreReplayChunk(queue.ctx, sessionUUID, chunkNumber(0).chunk).DoAndReturn(func(context.Context, string, sessionpkg.ReplayChunk) error {
		close(blocked)
		return errors.New("storage down")
	})
	queue.add(chunkNumber(0))
	<-blocked
	queue.add(chunkNumber(1))

	began := time.Now()
	queue.drain(50 * time.Millisecond)

	require.Less(t, time.Since(began), time.Second)
	require.Empty(t, queue.pending)
	require.Error(t, queue.ctx.Err())
}
