package replayrecording

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

//go:generate mockgen -package=replayrecording -destination=step_store_chunks_mock_test.go . chunkStore

// chunkStore stores a Session's replay chunks durably.
type chunkStore interface {
	StoreReplayChunk(ctx context.Context, sessionUUID string, chunk sessionpkg.ReplayChunk) error
}

// sealedChunk is an encoded chunk waiting to be stored.
type sealedChunk struct {
	sessionUUID string
	chunk       sessionpkg.ReplayChunk
}

// storeQueue holds the sealed chunks of every recording until its workers
// store them. It never holds more than its limit: the oldest chunk is dropped
// to make room, so gameplay never waits for storage.
type storeQueue struct {
	store    chunkStore
	limit    int
	attempts int
	backoff  time.Duration
	logger   loggerFunc

	mu      sync.Mutex
	pending []sealedChunk
	signal  chan struct{}
	// draining is closed when no more chunks come; ctx is cancelled when
	// the drain deadline passes.
	draining chan struct{}
	ctx      context.Context
	cancel   context.CancelFunc
	workers  sync.WaitGroup
}

// loggerFunc finishes a request log; it lets tests keep logs quiet.
type loggerFunc func(ctx context.Context, message string)

func newStoreQueue(store chunkStore, limit int, workers int, attempts int, backoff time.Duration, logger loggerFunc) *storeQueue {
	ctx, cancel := context.WithCancel(context.Background())
	q := &storeQueue{
		store: store, limit: limit, attempts: attempts, backoff: backoff, logger: logger,
		signal: make(chan struct{}, 1), draining: make(chan struct{}), ctx: ctx, cancel: cancel,
	}
	q.workers.Add(workers)
	for range workers {
		go func() {
			defer q.workers.Done()
			q.work()
		}()
	}
	return q
}

// add queues sealed for storing, dropping the oldest pending chunk when the
// queue is full. Never blocks.
func (q *storeQueue) add(sealed sealedChunk) {
	q.mu.Lock()
	var dropped *sealedChunk
	if len(q.pending) >= q.limit {
		oldest := q.pending[0]
		dropped = &oldest
		q.pending = q.pending[1:]
	} else {
		pendingChunks.Inc(q.ctx)
	}
	q.pending = append(q.pending, sealed)
	q.mu.Unlock()
	if dropped != nil {
		q.drop(*dropped, chunkDroppedBacklog, nil)
	}
	select {
	case q.signal <- struct{}{}:
	default:
	}
}

func (q *storeQueue) take() (sealedChunk, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.pending) == 0 {
		return sealedChunk{}, false
	}
	sealed := q.pending[0]
	q.pending = q.pending[1:]
	pendingChunks.Dec(q.ctx)
	return sealed, true
}

// work stores queued chunks until the queue drained or its deadline passed.
func (q *storeQueue) work() {
	for {
		if q.ctx.Err() != nil {
			// Past the drain deadline: drain drops what is left.
			return
		}
		if sealed, ok := q.take(); ok {
			q.storeWithRetries(sealed)
			continue
		}
		select {
		case <-q.signal:
		case <-q.draining:
			// One last look: a chunk may have come with the drain.
			if sealed, ok := q.take(); ok {
				q.storeWithRetries(sealed)
				continue
			}
			return
		}
	}
}

// storeWithRetries tries a few times, waiting longer each time, then drops it.
func (q *storeQueue) storeWithRetries(sealed sealedChunk) {
	wait := q.backoff
	for attempt := 1; ; attempt++ {
		err := storeChunk(q.ctx, q.store, sealed)
		if err == nil {
			chunksTotal.Inc(q.ctx, chunkOutcomeLabel.Value(chunkStored))
			return
		}
		if q.ctx.Err() != nil {
			q.drop(sealed, chunkDroppedShutdown, err)
			return
		}
		if attempt >= q.attempts {
			q.drop(sealed, chunkDroppedFailures, err)
			return
		}
		select {
		case <-time.After(wait):
		case <-q.ctx.Done():
			q.drop(sealed, chunkDroppedShutdown, err)
			return
		}
		wait *= 2
	}
}

// drain stops taking chunks and waits for the queue to empty, until
// deadline; what is still queued then is dropped.
func (q *storeQueue) drain(deadline time.Duration) {
	close(q.draining)
	done := make(chan struct{})
	go func() {
		q.workers.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(deadline):
		q.cancel()
		<-done
	}
	q.cancel()
	for {
		sealed, ok := q.take()
		if !ok {
			return
		}
		q.drop(sealed, chunkDroppedShutdown, nil)
	}
}

func (q *storeQueue) drop(sealed sealedChunk, reason string, cause error) {
	ctx := logging.Start(context.WithoutCancel(q.ctx))
	chunksTotal.Inc(ctx, chunkOutcomeLabel.Value(reason))
	monitoring.Alert(ctx, "replay chunk dropped")
	if cause != nil {
		logging.LogError(ctx, cause)
	}
	logging.LogFields(ctx,
		logging.Field("session_uuid", sealed.sessionUUID),
		logging.Field("recording_uuid", sealed.chunk.RecordingUUID),
		logging.Field("chunk_index", sealed.chunk.Index),
		logging.Field("drop_reason", reason),
	)
	q.logger(ctx, "replay chunk dropped")
}

// storeChunk makes one attempt to store sealed, measured.
func storeChunk(ctx context.Context, store chunkStore, sealed sealedChunk) error {
	began := time.Now()
	err := store.StoreReplayChunk(ctx, sealed.sessionUUID, sealed.chunk)
	outcome := storeSucceeded
	if err != nil {
		outcome = storeFailed
		err = fmt.Errorf("storing replay chunk %d: %w", sealed.chunk.Index, err)
	}
	storeDuration.ObserveDuration(ctx, time.Since(began), storeOutcomeLabel.Value(outcome))
	return err
}
