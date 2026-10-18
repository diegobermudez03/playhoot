// Package checkpoint encodes recovery checkpoints: what the Runtime needs to
// resume a Session from one execution boundary after losing its owner.
package checkpoint

import (
	"encoding/json"
	"fmt"
)

// SchemaVersion is the checkpoint schema this package writes.
const SchemaVersion = 1

// Checkpoint is the Runtime's authoritative state at one execution boundary.
type Checkpoint struct {
	GameVersionUUID string
	ContextRevision int64

	// ExecutedThroughSessionTimeMs is the effective Session time State was
	// materialized through, where a recovered game's clock resumes.
	ExecutedThroughSessionTimeMs int64

	// State is the authoritative game state, as JSON.
	State []byte

	// PRNGState is the opaque state of the script's randomness.
	PRNGState []byte

	// Participants are the Session's ParticipantIDs, in order.
	Participants []string
}

type document struct {
	SchemaVersion   int             `json:"schema_version"`
	GameVersionUUID string          `json:"game_version_uuid"`
	ContextRevision int64           `json:"context_revision"`
	ExecutedThrough int64           `json:"executed_through_session_time_ms"`
	State           json.RawMessage `json:"state"`
	PRNGState       []byte          `json:"prng_state"`
	Participants    []string        `json:"participants"`
}

// Encode returns checkpoint as stored, under SchemaVersion.
func Encode(checkpoint Checkpoint) ([]byte, error) {
	encoded, err := json.Marshal(document{
		SchemaVersion:   SchemaVersion,
		GameVersionUUID: checkpoint.GameVersionUUID,
		ContextRevision: checkpoint.ContextRevision,
		ExecutedThrough: checkpoint.ExecutedThroughSessionTimeMs,
		State:           checkpoint.State,
		PRNGState:       checkpoint.PRNGState,
		Participants:    checkpoint.Participants,
	})
	if err != nil {
		return nil, fmt.Errorf("encoding checkpoint: %w", err)
	}
	return encoded, nil
}

// Decode reads a checkpoint Encode wrote; one of another schema version is an
// error.
func Decode(encoded []byte) (Checkpoint, error) {
	var doc document
	if err := json.Unmarshal(encoded, &doc); err != nil {
		return Checkpoint{}, fmt.Errorf("decoding checkpoint: %w", err)
	}
	if doc.SchemaVersion != SchemaVersion {
		return Checkpoint{}, fmt.Errorf("checkpoint schema_version %d is not %d", doc.SchemaVersion, SchemaVersion)
	}
	return Checkpoint{
		GameVersionUUID:              doc.GameVersionUUID,
		ContextRevision:              doc.ContextRevision,
		ExecutedThroughSessionTimeMs: doc.ExecutedThrough,
		State:                        doc.State,
		PRNGState:                    doc.PRNGState,
		Participants:                 doc.Participants,
	}, nil
}
