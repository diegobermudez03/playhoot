// Package replaychunk builds and encodes replay chunks: self-contained pieces
// of a Session's approximate history, each a full authoritative state
// followed by state deltas and transient records, in timeline order.
package replaychunk

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/klauspost/compress/zstd"
)

// FormatVersion is the chunk format this package writes. Stored chunks live
// for long, so a reader must check it before interpreting one.
const FormatVersion = 1

// RecordType is what a record holds.
type RecordType string

const (
	// StateDelta is a JSON Patch (RFC 6902) from the previous state of the
	// chunk, its base or the last delta, to the state at the record's time.
	StateDelta RecordType = "STATE_DELTA"
	// Event is a logical event; Participants are who saw it, nil meaning all.
	Event RecordType = "EVENT"
	// CommandResponse is a COMMAND's answer, seen only by its one participant.
	CommandResponse RecordType = "COMMAND_RESPONSE"
	// Error is a logical error, seen only by its one participant.
	Error RecordType = "ERROR"
)

// Chunk is a piece of one recording of a Session's history. It covers
// effective Session time from its base state's time through ThroughSessionTimeMs.
type Chunk struct {
	SessionUUID     string
	GameVersionUUID string
	// RecordingUUID is the run of the Runtime that recorded it; a later
	// recording of the same Session supersedes earlier ones from its start.
	RecordingUUID string
	Index         int
	Final         bool

	FromSessionTimeMs    int64
	ThroughSessionTimeMs int64

	// Participants are the Session's ParticipantIDs, in order.
	Participants []string

	// Base is the full authoritative state at FromSessionTimeMs, as JSON.
	Base json.RawMessage

	// Records follow Base in timeline order.
	Records []Record
}

// Record is one point of a chunk's timeline after its base.
type Record struct {
	Type          RecordType
	SessionTimeMs int64
	// TimelineOrder orders records within their recording, ties of
	// SessionTimeMs included.
	TimelineOrder uint64
	Participants  []string
	Value         json.RawMessage
}

type document struct {
	FormatVersion   int      `json:"format_version"`
	SessionUUID     string   `json:"session_uuid"`
	GameVersionUUID string   `json:"game_version_uuid"`
	RecordingUUID   string   `json:"recording_uuid"`
	Index           int      `json:"index"`
	Final           bool     `json:"final"`
	From            int64    `json:"from_session_time_ms"`
	Through         int64    `json:"through_session_time_ms"`
	Participants    []string `json:"participants"`

	Base    json.RawMessage `json:"base_state"`
	Records []recordJSON    `json:"records"`
}

type recordJSON struct {
	Type          RecordType `json:"type"`
	SessionTimeMs int64      `json:"session_time_ms"`
	TimelineOrder uint64     `json:"timeline_order"`
	// Participants keeps null (everyone) apart from an empty list (no one).
	Participants []string        `json:"participants"`
	Value        json.RawMessage `json:"value"`
}

var (
	encoder, _ = zstd.NewWriter(nil)
	decoder, _ = zstd.NewReader(nil)
)

// Encode returns chunk as stored: JSON under FormatVersion, compressed with
// zstd. A private record without exactly one participant is refused.
func Encode(chunk Chunk) ([]byte, error) {
	if err := validate(chunk); err != nil {
		return nil, err
	}
	stored := document{
		FormatVersion:   FormatVersion,
		SessionUUID:     chunk.SessionUUID,
		GameVersionUUID: chunk.GameVersionUUID,
		RecordingUUID:   chunk.RecordingUUID,
		Index:           chunk.Index,
		Final:           chunk.Final,
		From:            chunk.FromSessionTimeMs,
		Through:         chunk.ThroughSessionTimeMs,
		Participants:    chunk.Participants,
		Base:            chunk.Base,
		Records:         make([]recordJSON, len(chunk.Records)),
	}
	for i, record := range chunk.Records {
		stored.Records[i] = recordJSON(record)
	}
	encoded, err := json.Marshal(stored)
	if err != nil {
		return nil, fmt.Errorf("encoding replay chunk: %w", err)
	}
	return encoder.EncodeAll(encoded, nil), nil
}

// Decode reads a chunk Encode wrote, refusing other format versions.
func Decode(stored []byte) (Chunk, error) {
	raw, err := decoder.DecodeAll(stored, nil)
	if err != nil {
		return Chunk{}, fmt.Errorf("decompressing replay chunk: %w", err)
	}
	var decoded document
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return Chunk{}, fmt.Errorf("decoding replay chunk: %w", err)
	}
	if decoded.FormatVersion != FormatVersion {
		return Chunk{}, fmt.Errorf("replay chunk format version %d, want %d", decoded.FormatVersion, FormatVersion)
	}
	chunk := Chunk{
		SessionUUID:          decoded.SessionUUID,
		GameVersionUUID:      decoded.GameVersionUUID,
		RecordingUUID:        decoded.RecordingUUID,
		Index:                decoded.Index,
		Final:                decoded.Final,
		FromSessionTimeMs:    decoded.From,
		ThroughSessionTimeMs: decoded.Through,
		Participants:         decoded.Participants,
		Base:                 decoded.Base,
	}
	for _, record := range decoded.Records {
		chunk.Records = append(chunk.Records, Record(record))
	}
	return chunk, nil
}

func validate(chunk Chunk) error {
	if len(chunk.Base) == 0 {
		return errors.New("replay chunk has no base state")
	}
	for _, record := range chunk.Records {
		switch record.Type {
		case CommandResponse, Error:
			if len(record.Participants) != 1 {
				return fmt.Errorf("%s record must target exactly one participant, has %d", record.Type, len(record.Participants))
			}
		case StateDelta:
			if len(record.Participants) != 0 {
				return errors.New("STATE_DELTA record cannot target participants")
			}
		case Event:
		default:
			return fmt.Errorf("unknown replay record type %q", record.Type)
		}
	}
	return nil
}
