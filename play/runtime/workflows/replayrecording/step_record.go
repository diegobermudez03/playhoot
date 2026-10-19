package replayrecording

import (
	"context"
	"fmt"
	"time"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	"github.com/diegobermudez03/playhoot/play/runtime/replay"
	"github.com/diegobermudez03/playhoot/play/runtime/workflows/replayrecording/internal/chunk"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

type messageKind int

const (
	offerTick messageKind = iota
	finishGame
	stopRecording
)

// message is what a recording's goroutine handles, in the order sent.
type message struct {
	kind    messageKind
	capture replay.Capture
	// ctx and done belong to a Finish call waiting for its answer.
	ctx  context.Context
	done chan<- error
}

// recording is one Runtime run's history of a Session. Only its own
// goroutine touches it after it starts.
type recording struct {
	id          string
	sessionUUID string
	startedAt   time.Time
	chunks      *chunking
	inbox       chan message

	// final is the sealed final chunk until it is stored; then finished.
	final    *sessionpkg.ReplayChunk
	finished bool
}

// run handles the recording's messages until it is stopped.
func (r *Recorder) run(recorded *recording) {
	for received := range recorded.inbox {
		switch received.kind {
		case offerTick:
			r.record(recorded, received.capture)
		case finishGame:
			received.done <- r.finish(received.ctx, recorded, received.capture)
		case stopRecording:
			r.stop(recorded)
			return
		}
	}
}

func (r *Recorder) record(recorded *recording, capture replay.Capture) {
	if recorded.finished || recorded.final != nil {
		return
	}
	sealed, err := recorded.chunks.add(capture)
	if err != nil {
		r.reject(recorded, err)
		return
	}
	capturesTotal.Inc(context.Background(), captureOutcomeLabel.Value(captureRecorded))
	if sealed != nil {
		r.enqueue(recorded, *sealed)
	}
}

// finish seals the final chunk once, then stores it.
func (r *Recorder) finish(ctx context.Context, recorded *recording, capture replay.Capture) error {
	if recorded.finished {
		return nil
	}
	if recorded.final == nil {
		final, err := recorded.chunks.finish(capture)
		if err != nil {
			r.reject(recorded, err)
			return fmt.Errorf("recording the final tick: %w", err)
		}
		capturesTotal.Inc(ctx, captureOutcomeLabel.Value(captureRecorded))
		encoded, err := r.encode(recorded, final)
		if err != nil {
			return err
		}
		recorded.final = &encoded
	}
	err := storeChunk(ctx, r.store, sealedChunk{sessionUUID: recorded.sessionUUID, chunk: *recorded.final})
	if err != nil {
		finalsTotal.Inc(ctx, storeOutcomeLabel.Value(storeFailed))
		return err
	}
	finalsTotal.Inc(ctx, storeOutcomeLabel.Value(storeSucceeded))
	recorded.final, recorded.finished = nil, true
	return nil
}

// stop hands whatever the recording still holds to the queue.
func (r *Recorder) stop(recorded *recording) {
	switch {
	case recorded.final != nil:
		r.queue.add(sealedChunk{sessionUUID: recorded.sessionUUID, chunk: *recorded.final})
	case !recorded.finished:
		if sealed := recorded.chunks.seal(); sealed != nil {
			r.enqueue(recorded, *sealed)
		}
	}
}

func (r *Recorder) enqueue(recorded *recording, sealed chunk.Chunk) {
	encoded, err := r.encode(recorded, sealed)
	if err != nil {
		ctx := logging.Start(context.Background())
		chunksTotal.Inc(ctx, chunkOutcomeLabel.Value(chunkDroppedEncoding))
		monitoring.Alert(ctx, "replay chunk dropped")
		logging.LogError(ctx, err)
		logging.LogFields(ctx, logging.Field("session_uuid", recorded.sessionUUID), logging.Field("recording_uuid", recorded.id))
		r.finishLog(ctx, "replay chunk dropped")
		return
	}
	r.queue.add(sealedChunk{sessionUUID: recorded.sessionUUID, chunk: encoded})
}

func (r *Recorder) encode(recorded *recording, sealed chunk.Chunk) (sessionpkg.ReplayChunk, error) {
	content, err := chunk.Encode(sealed)
	if err != nil {
		return sessionpkg.ReplayChunk{}, err
	}
	ctx := context.Background()
	chunkBytes.Observe(ctx, float64(len(content)))
	chunkSpan.Observe(ctx, float64(sealed.ThroughSessionTimeMs-sealed.FromSessionTimeMs)/1000)
	return sessionpkg.ReplayChunk{
		RecordingUUID:        recorded.id,
		RecordingStartedAt:   recorded.startedAt,
		Index:                sealed.Index,
		FormatVersion:        chunk.FormatVersion,
		FromSessionTimeMs:    sealed.FromSessionTimeMs,
		ThroughSessionTimeMs: sealed.ThroughSessionTimeMs,
		Final:                sealed.Final,
		Content:              content,
	}, nil
}

// reject drops a tick the game produced but replay cannot record.
func (r *Recorder) reject(recorded *recording, err error) {
	ctx := logging.Start(context.Background())
	capturesTotal.Inc(ctx, captureOutcomeLabel.Value(captureRejected))
	logging.LogError(ctx, err)
	logging.LogFields(ctx, logging.Field("session_uuid", recorded.sessionUUID), logging.Field("recording_uuid", recorded.id))
	r.finishLog(ctx, "replay capture rejected")
}
