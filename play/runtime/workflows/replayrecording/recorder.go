// Package replayrecording records the approximate replay history of the
// games running on this instance. Each recording builds its chunks in memory
// and hands them to a bounded queue that stores them in the background.
package replayrecording

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/play/runtime/replay"
)

// Config tunes replay recording; a zero field takes its default. The three
// knobs are independent of each other and of the game's tick cadence.
type Config struct {
	// StateEveryTicks keeps the state of one committed tick in every
	// StateEveryTicks; 1 keeps every one.
	StateEveryTicks int
	// ChunkDuration is the effective Session time each chunk covers.
	ChunkDuration time.Duration
	// MaxPendingChunks bounds the sealed chunks waiting to be stored.
	MaxPendingChunks int
}

const (
	defaultStateEveryTicks  = 1
	defaultChunkDuration    = time.Minute
	defaultMaxPendingChunks = 64

	// maxChunkBytes seals a chunk early, bounding each recording's memory.
	maxChunkBytes = 16 << 20
	// inboxSize is how many offered ticks a recording may have waiting.
	inboxSize = 256

	storeWorkers  = 4
	storeAttempts = 5
	storeBackoff  = time.Second
	// drainTimeout is how long shutting down waits for pending chunks.
	drainTimeout = 10 * time.Second
)

var errUnknownRecording = errors.New("replay recording not found")

// Recorder records every running game of this instance. Its methods never
// make gameplay wait for storage, except Finish, which stores the ending.
type Recorder struct {
	store  chunkStore
	policy chunkingPolicy
	queue  *storeQueue
	now    func() time.Time
	logger *slog.Logger

	mu         sync.Mutex
	recordings map[string]*recording
	closed     bool
	running    sync.WaitGroup
}

// New returns a Recorder storing chunks through store.
func New(store chunkStore, config Config) *Recorder {
	return newRecorder(store, config, storeBackoff, slog.Default())
}

func newRecorder(store chunkStore, config Config, backoff time.Duration, logger *slog.Logger) *Recorder {
	if config.StateEveryTicks <= 0 {
		config.StateEveryTicks = defaultStateEveryTicks
	}
	if config.ChunkDuration <= 0 {
		config.ChunkDuration = defaultChunkDuration
	}
	if config.MaxPendingChunks <= 0 {
		config.MaxPendingChunks = defaultMaxPendingChunks
	}
	r := &Recorder{
		store:      store,
		policy:     chunkingPolicy{stateEveryTicks: config.StateEveryTicks, chunkDuration: config.ChunkDuration, maxChunkBytes: maxChunkBytes},
		now:        time.Now,
		logger:     logger,
		recordings: make(map[string]*recording),
	}
	r.queue = newStoreQueue(store, config.MaxPendingChunks, storeWorkers, storeAttempts, backoff, r.finishLog)
	return r
}

// StartRecording begins a recording of sessionUUID's running game and returns
// its ID; empty once the Recorder is closed.
func (r *Recorder) StartRecording(sessionUUID string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ""
	}
	id := uuid.NewString()
	recorded := &recording{
		id:          id,
		sessionUUID: sessionUUID,
		startedAt:   r.now(),
		chunks:      newChunking(id, r.policy),
		inbox:       make(chan message, inboxSize),
	}
	r.recordings[id] = recorded
	r.running.Add(1)
	go func() {
		defer r.running.Done()
		r.run(recorded)
	}()
	return id
}

// Offer hands a recording one committed tick. It never blocks: a tick that
// does not fit is dropped.
func (r *Recorder) Offer(ctx context.Context, recordingID string, capture replay.Capture) {
	recorded, found := r.recording(recordingID)
	if !found {
		return
	}
	select {
	case recorded.inbox <- message{kind: offerTick, capture: capture}:
	default:
		capturesTotal.Inc(ctx, captureOutcomeLabel.Value(captureDropped))
	}
}

// Finish records the game's final tick and stores the last chunk before
// returning. After a failure, calling it again retries the store.
func (r *Recorder) Finish(ctx context.Context, recordingID string, capture replay.Capture) error {
	recorded, found := r.recording(recordingID)
	if !found {
		return errUnknownRecording
	}
	done := make(chan error, 1)
	select {
	case recorded.inbox <- message{kind: finishGame, capture: capture, ctx: ctx, done: done}:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// StopRecording ends a recording; what it still holds is stored in the
// background.
func (r *Recorder) StopRecording(recordingID string) {
	r.mu.Lock()
	recorded, found := r.recordings[recordingID]
	delete(r.recordings, recordingID)
	r.mu.Unlock()
	if found {
		recorded.inbox <- message{kind: stopRecording}
	}
}

// Close stops every recording and waits, for a few seconds at most, for what
// they held to be stored.
func (r *Recorder) Close() {
	r.mu.Lock()
	r.closed = true
	stopping := r.recordings
	r.recordings = make(map[string]*recording)
	r.mu.Unlock()
	for _, recorded := range stopping {
		recorded.inbox <- message{kind: stopRecording}
	}
	r.running.Wait()
	r.queue.drain(drainTimeout)
}

func (r *Recorder) recording(recordingID string) (*recording, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	recorded, found := r.recordings[recordingID]
	return recorded, found
}

func (r *Recorder) finishLog(ctx context.Context, message string) {
	logging.FinishRequestLog(ctx, r.logger, message)
}
