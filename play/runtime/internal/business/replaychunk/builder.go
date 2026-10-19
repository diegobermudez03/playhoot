package replaychunk

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/diegobermudez03/playhoot/play/runtime/internal/business/jsonpatch"
	"github.com/diegobermudez03/playhoot/play/runtime/replay"
)

// Settings tune a recording. They are independent of each other and of the
// game's own tick cadence.
type Settings struct {
	// StateEveryTicks keeps the state of one committed tick in every
	// StateEveryTicks; 1 keeps every one.
	StateEveryTicks int
	// ChunkDuration is how much effective Session time a chunk covers; chunks
	// end at multiples of it.
	ChunkDuration time.Duration
	// MaxChunkBytes seals a chunk early once its content reaches it.
	MaxChunkBytes int
}

// Builder turns one recording's captures into chunks. Each chunk starts with
// a full state; the states sampled after it are deltas from the previous one,
// and transient records are always kept. It is not safe for concurrent use.
type Builder struct {
	recordingUUID string
	settings      Settings

	active      *Chunk
	activeEnd   int64
	activeBytes int
	nextIndex   int

	// lastState is the active chunk's latest state point, decoded.
	lastState      any
	lastTimeMs     int64
	ticksSkipped   int
	nextOrder      uint64
	sessionUUID    string
	gameVersion    string
	participantIDs []string
}

// NewBuilder returns a Builder for the recording recordingUUID.
func NewBuilder(recordingUUID string, settings Settings) *Builder {
	settings.StateEveryTicks = max(settings.StateEveryTicks, 1)
	if settings.ChunkDuration < time.Millisecond {
		settings.ChunkDuration = time.Millisecond
	}
	if settings.MaxChunkBytes <= 0 {
		settings.MaxChunkBytes = math.MaxInt
	}
	return &Builder{recordingUUID: recordingUUID, settings: settings}
}

// Add records one committed tick, and returns the chunk it sealed when the
// tick started a new one.
func (b *Builder) Add(capture replay.Capture) (*Chunk, error) {
	return b.add(capture, false)
}

// Finish records the game's final tick, whose state is always kept, in the
// last chunk, even past its end, and seals that chunk as final.
func (b *Builder) Finish(capture replay.Capture) (Chunk, error) {
	if _, err := b.add(capture, true); err != nil {
		return Chunk{}, err
	}
	last := b.seal(capture.SessionTimeMs)
	last.Final = true
	return *last, nil
}

// Seal closes the active chunk through the last tick recorded; nil when
// there is none.
func (b *Builder) Seal() *Chunk {
	if b.active == nil {
		return nil
	}
	return b.seal(b.lastTimeMs)
}

func (b *Builder) add(capture replay.Capture, final bool) (*Chunk, error) {
	transients, err := transientsOf(capture)
	if err != nil {
		return nil, err
	}
	// The final tick stays in the last chunk, so its transients are as
	// durable as its state.
	rollover := !final && b.active != nil && (capture.SessionTimeMs >= b.activeEnd || b.activeBytes >= b.settings.MaxChunkBytes)
	keepState := b.active == nil || rollover || final || b.ticksSkipped+1 >= b.settings.StateEveryTicks
	var state any
	if keepState {
		if state, err = jsonpatch.Parse(capture.State); err != nil {
			return nil, fmt.Errorf("captured state: %w", err)
		}
	}

	var sealed *Chunk
	b.lastTimeMs = max(b.lastTimeMs, capture.SessionTimeMs)
	switch {
	case b.active == nil:
		b.open(capture, state)
		// Only a recording's first tick has transients after its base.
		b.append(transients)
	case rollover:
		// Whatever led to the new base belongs to the chunk before it.
		b.append(transients)
		sealed = b.seal(capture.SessionTimeMs)
		b.open(capture, state)
	default:
		b.append(transients)
		if !keepState {
			b.ticksSkipped++
			return nil, nil
		}
		if err := b.appendState(capture.SessionTimeMs, state); err != nil {
			return nil, err
		}
	}
	return sealed, nil
}

func (b *Builder) open(capture replay.Capture, state any) {
	if b.sessionUUID == "" {
		b.sessionUUID, b.gameVersion = capture.SessionUUID, capture.GameVersionUUID
		b.participantIDs = slices.Clone(capture.Participants)
	}
	b.active = &Chunk{
		SessionUUID:       b.sessionUUID,
		GameVersionUUID:   b.gameVersion,
		RecordingUUID:     b.recordingUUID,
		Index:             b.nextIndex,
		FromSessionTimeMs: capture.SessionTimeMs,
		Participants:      b.participantIDs,
		Base:              capture.State,
	}
	b.nextIndex++
	duration := b.settings.ChunkDuration.Milliseconds()
	b.activeEnd = (capture.SessionTimeMs/duration + 1) * duration
	b.activeBytes = len(capture.State)
	b.lastState = state
	b.ticksSkipped = 0
}

func (b *Builder) seal(throughMs int64) *Chunk {
	chunk := b.active
	chunk.ThroughSessionTimeMs = throughMs
	b.active, b.lastState = nil, nil
	return chunk
}

// appendState records the delta to state, nothing when it did not change.
func (b *Builder) appendState(sessionTimeMs int64, state any) error {
	operations := jsonpatch.Diff(b.lastState, state)
	b.lastState = state
	b.ticksSkipped = 0
	if len(operations) == 0 {
		return nil
	}
	patch, err := json.Marshal(operations)
	if err != nil {
		return fmt.Errorf("encoding state delta: %w", err)
	}
	b.append([]Record{{Type: StateDelta, SessionTimeMs: sessionTimeMs, Value: patch}})
	return nil
}

func (b *Builder) append(records []Record) {
	for _, record := range records {
		record.TimelineOrder = b.nextOrder
		b.nextOrder++
		b.activeBytes += len(record.Value)
		b.active.Records = append(b.active.Records, record)
	}
}

// transientsOf returns capture's responses, errors and events in timeline
// order; at the same time and order a response comes first, an event last.
func transientsOf(capture replay.Capture) ([]Record, error) {
	type placed struct {
		record Record
		order  uint64
	}
	var all []placed
	for _, response := range capture.CommandResponses {
		all = append(all, placed{Record{Type: CommandResponse, SessionTimeMs: response.SessionTimeMs, Participants: []string{response.ParticipantID}, Value: response.Response}, response.Order})
	}
	for _, gameError := range capture.Errors {
		all = append(all, placed{Record{Type: Error, SessionTimeMs: gameError.SessionTimeMs, Participants: []string{gameError.ParticipantID}, Value: gameError.Error}, gameError.Order})
	}
	for _, event := range capture.Events {
		all = append(all, placed{Record{Type: Event, SessionTimeMs: event.SessionTimeMs, Participants: slices.Clone(event.Participants), Value: event.Event}, event.Order})
	}
	records := make([]Record, 0, len(all))
	slices.SortStableFunc(all, func(a, b placed) int {
		if a.record.SessionTimeMs != b.record.SessionTimeMs {
			return cmp.Compare(a.record.SessionTimeMs, b.record.SessionTimeMs)
		}
		return cmp.Compare(runOrder(a.order), runOrder(b.order))
	})
	for _, transient := range all {
		if !json.Valid(transient.record.Value) {
			return nil, errors.New("captured " + string(transient.record.Type) + " is not JSON")
		}
		records = append(records, transient.record)
	}
	return records, nil
}

// runOrder places update(), whose order is 0, after every operation of its
// time.
func runOrder(order uint64) uint64 {
	if order == 0 {
		return math.MaxUint64
	}
	return order
}
