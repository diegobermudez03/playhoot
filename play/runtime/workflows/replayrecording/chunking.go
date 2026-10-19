package replayrecording

import (
	"math"
	"slices"
	"time"

	"github.com/diegobermudez03/playhoot/play/runtime/replay"
	"github.com/diegobermudez03/playhoot/play/runtime/workflows/replayrecording/internal/chunk"
)

// chunkingPolicy is when a recording keeps a tick's state and ends a chunk.
type chunkingPolicy struct {
	// stateEveryTicks keeps the state of one committed tick in every
	// stateEveryTicks; 1 keeps every one.
	stateEveryTicks int
	// chunkDuration is the effective Session time a chunk covers; chunks
	// end at multiples of it.
	chunkDuration time.Duration
	// maxChunkBytes ends a chunk early once its content reaches it.
	maxChunkBytes int
}

// chunking turns one recording's ticks into chunks under its policy. Each
// chunk starts with a full state; transients are always kept.
type chunking struct {
	recordingUUID string
	policy        chunkingPolicy
	header        *chunk.Header

	active    *chunk.Builder
	activeEnd int64
	nextIndex int
	nextOrder uint64
	// ticksSkipped counts the ticks since the active chunk last kept a state.
	ticksSkipped int
	lastTimeMs   int64
}

func newChunking(recordingUUID string, policy chunkingPolicy) *chunking {
	policy.stateEveryTicks = max(policy.stateEveryTicks, 1)
	if policy.chunkDuration < time.Millisecond {
		policy.chunkDuration = time.Millisecond
	}
	if policy.maxChunkBytes <= 0 {
		policy.maxChunkBytes = math.MaxInt
	}
	return &chunking{recordingUUID: recordingUUID, policy: policy}
}

// add records one committed tick, and returns the chunk it ended when the
// tick is at or past the chunk's end, or the chunk is full.
func (c *chunking) add(capture replay.Capture) (*chunk.Chunk, error) {
	return c.record(capture, false)
}

// finish records the game's final tick, its state always kept, in the last
// chunk, even past its end, and ends that chunk as final.
func (c *chunking) finish(capture replay.Capture) (chunk.Chunk, error) {
	if _, err := c.record(capture, true); err != nil {
		return chunk.Chunk{}, err
	}
	return c.end(capture.SessionTimeMs, true), nil
}

// seal ends the active chunk through the last tick recorded; nil when there
// is none.
func (c *chunking) seal() *chunk.Chunk {
	if c.active == nil {
		return nil
	}
	sealed := c.end(c.lastTimeMs, false)
	return &sealed
}

// record checks the whole tick before changing anything, so a rejected tick
// leaves no trace.
func (c *chunking) record(capture replay.Capture, final bool) (*chunk.Chunk, error) {
	transients, err := chunk.Transients(capture)
	if err != nil {
		return nil, err
	}
	// The final tick stays in the last chunk, so its transients are as
	// durable as its state.
	rollover := !final && c.active != nil && (capture.SessionTimeMs >= c.activeEnd || c.active.Size() >= c.policy.maxChunkBytes)
	keepState := c.active == nil || rollover || final || c.ticksSkipped+1 >= c.policy.stateEveryTicks
	var state chunk.State
	if keepState {
		if state, err = chunk.ParseState(capture.State); err != nil {
			return nil, err
		}
	}

	c.lastTimeMs = max(c.lastTimeMs, capture.SessionTimeMs)
	switch {
	case c.active == nil:
		c.open(capture, state)
		// Only a recording's first tick has transients after its base.
		c.active.Append(transients)
		return nil, nil
	case rollover:
		// Whatever led to the new base belongs to the chunk before it.
		c.active.Append(transients)
		sealed := c.end(capture.SessionTimeMs, false)
		c.open(capture, state)
		return &sealed, nil
	default:
		c.active.Append(transients)
		if !keepState {
			c.ticksSkipped++
			return nil, nil
		}
		c.ticksSkipped = 0
		return nil, c.active.AppendState(capture.SessionTimeMs, state)
	}
}

// open starts the next chunk with state as its base, ending at the next
// multiple of the chunk duration.
func (c *chunking) open(capture replay.Capture, state chunk.State) {
	if c.header == nil {
		c.header = &chunk.Header{
			SessionUUID:     capture.SessionUUID,
			GameVersionUUID: capture.GameVersionUUID,
			RecordingUUID:   c.recordingUUID,
			Participants:    slices.Clone(capture.Participants),
		}
	}
	c.active = chunk.Open(*c.header, c.nextIndex, c.nextOrder, capture.SessionTimeMs, state)
	c.nextIndex++
	duration := c.policy.chunkDuration.Milliseconds()
	c.activeEnd = (capture.SessionTimeMs/duration + 1) * duration
	c.ticksSkipped = 0
}

func (c *chunking) end(throughMs int64, final bool) chunk.Chunk {
	sealed := c.active.Seal(throughMs, final)
	c.nextOrder = c.active.NextOrder()
	c.active = nil
	return sealed
}
