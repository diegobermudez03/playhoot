package chunk

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/diegobermudez03/playhoot/play/runtime/internal/business/jsonpatch"
	"github.com/diegobermudez03/playhoot/play/runtime/replay"
)

// Header is what every chunk of a recording repeats.
type Header struct {
	SessionUUID     string
	GameVersionUUID string
	RecordingUUID   string
	Participants    []string
}

// State is an authoritative state as JSON, parsed once so deltas between
// states can be computed.
type State struct {
	raw  []byte
	tree any
}

// ParseState parses raw, failing when it is not JSON.
func ParseState(raw []byte) (State, error) {
	tree, err := jsonpatch.Parse(raw)
	if err != nil {
		return State{}, fmt.Errorf("captured state: %w", err)
	}
	return State{raw: raw, tree: tree}, nil
}

// Builder builds one chunk: its base state, then records appended in
// timeline order. It is not safe for concurrent use.
type Builder struct {
	chunk     Chunk
	last      any
	size      int
	nextOrder uint64
}

// Open starts the chunk number index of header's recording with base, at
// sessionTimeMs; its records are ordered from firstOrder.
func Open(header Header, index int, firstOrder uint64, sessionTimeMs int64, base State) *Builder {
	return &Builder{
		chunk: Chunk{
			SessionUUID:       header.SessionUUID,
			GameVersionUUID:   header.GameVersionUUID,
			RecordingUUID:     header.RecordingUUID,
			Index:             index,
			FromSessionTimeMs: sessionTimeMs,
			Participants:      header.Participants,
			Base:              base.raw,
		},
		last:      base.tree,
		size:      len(base.raw),
		nextOrder: firstOrder,
	}
}

// Append adds records after the chunk's last one, in order.
func (b *Builder) Append(records []Record) {
	for _, record := range records {
		record.TimelineOrder = b.nextOrder
		b.nextOrder++
		b.size += len(record.Value)
		b.chunk.Records = append(b.chunk.Records, record)
	}
}

// AppendState adds the delta from the chunk's latest state to state, at
// sessionTimeMs; nothing when it did not change.
func (b *Builder) AppendState(sessionTimeMs int64, state State) error {
	operations := jsonpatch.Diff(b.last, state.tree)
	if len(operations) == 0 {
		return nil
	}
	patch, err := json.Marshal(operations)
	if err != nil {
		return fmt.Errorf("encoding state delta: %w", err)
	}
	b.last = state.tree
	b.Append([]Record{{Type: StateDelta, SessionTimeMs: sessionTimeMs, Value: patch}})
	return nil
}

// Size is roughly how many bytes the chunk holds before encoding.
func (b *Builder) Size() int {
	return b.size
}

// NextOrder is the timeline order the record after this chunk's last takes.
func (b *Builder) NextOrder() uint64 {
	return b.nextOrder
}

// Seal returns the chunk, covering Session time through throughSessionTimeMs.
func (b *Builder) Seal(throughSessionTimeMs int64, final bool) Chunk {
	sealed := b.chunk
	sealed.ThroughSessionTimeMs = throughSessionTimeMs
	sealed.Final = final
	return sealed
}

// Transients returns capture's responses, errors and events as records in
// timeline order (at the same time and order a response comes first, an
// event last), failing when one is not JSON.
func Transients(capture replay.Capture) ([]Record, error) {
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
	slices.SortStableFunc(all, func(a, b placed) int {
		if a.record.SessionTimeMs != b.record.SessionTimeMs {
			return cmp.Compare(a.record.SessionTimeMs, b.record.SessionTimeMs)
		}
		return cmp.Compare(runOrder(a.order), runOrder(b.order))
	})
	records := make([]Record, 0, len(all))
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
