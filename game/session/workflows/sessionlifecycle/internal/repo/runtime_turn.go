package repo

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// runtimeTurnInsert is the persisted shape of one committed RuntimeTurn.
// SourceInteractionID/ActorID are nil for a Turn with no such cause.
type runtimeTurnInsert struct {
	ID                    uint   `gorm:"column:id"`
	SessionID             uint   `gorm:"column:session_id"`
	Sequence              uint64 `gorm:"column:sequence"`
	SourceKind            string `gorm:"column:source_kind"`
	SourceInteractionID   *uint  `gorm:"column:source_interaction_id"`
	ActorID               *uint  `gorm:"column:actor_id"`
	SnapshotPayload       []byte `gorm:"column:snapshot_payload"`
	SnapshotFormatVersion int    `gorm:"column:snapshot_format_version"`
}

func (runtimeTurnInsert) TableName() string { return "session_runtime_turns" }

// CreateRuntimeTurn persists one committed RuntimeTurn - only the final
// Snapshot after the whole Turn; intermediate Snapshots are never
// separately authoritative. sourceInteractionID/actorID are the
// Turn's own cause/actor references, nil when the Turn has no such cause.
// Returns the new row's internal id.
func (r *Repo) CreateRuntimeTurn(ctx context.Context, tx *gorm.DB, sessionID uint, sequence uint64, sourceKind string, sourceInteractionID *uint, actorID *uint, snapshotPayload []byte, snapshotFormatVersion int) (uint, error) {
	row := runtimeTurnInsert{
		SessionID:             sessionID,
		Sequence:              sequence,
		SourceKind:            sourceKind,
		SourceInteractionID:   sourceInteractionID,
		ActorID:               actorID,
		SnapshotPayload:       snapshotPayload,
		SnapshotFormatVersion: snapshotFormatVersion,
	}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, fmt.Errorf("creating runtime turn: %s", err)
	}
	return row.ID, nil
}

// RuntimeTurn is the persisted facts of one committed RuntimeTurn needed to
// resume execution from it: its own sequence (to compute the next Turn's
// sequence) and its final authoritative Snapshot.
type RuntimeTurn struct {
	ID                    uint
	Sequence              uint64
	SnapshotPayload       []byte
	SnapshotFormatVersion int
}

// GetRuntimeTurn loads turnID's persisted facts, for a RUNNING-phase
// mutation resuming execution from the Session's current authoritative
// Turn and Snapshot.
func (r *Repo) GetRuntimeTurn(ctx context.Context, tx *gorm.DB, turnID uint) (*RuntimeTurn, error) {
	var row RuntimeTurn
	result := tx.WithContext(ctx).Raw(`
		SELECT id, sequence, snapshot_payload, snapshot_format_version
		FROM session_runtime_turns
		WHERE id = ?
	`, turnID).Scan(&row)
	if result.Error != nil {
		return nil, fmt.Errorf("getting runtime turn: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &row, nil
}

type runtimeStepInsert struct {
	ID            uint   `gorm:"column:id"`
	RuntimeTurnID uint   `gorm:"column:runtime_turn_id"`
	StepIndex     int    `gorm:"column:step_index"`
	CommitPayload []byte `gorm:"column:commit_payload"`
}

func (runtimeStepInsert) TableName() string { return "session_runtime_steps" }

// CreateRuntimeStep persists one RuntimeStep - technical execution-history
// trace beneath runtimeTurnID, never itself authoritative and never owning
// a Session-state sequence.
func (r *Repo) CreateRuntimeStep(ctx context.Context, tx *gorm.DB, runtimeTurnID uint, stepIndex int, commitPayload []byte) error {
	row := runtimeStepInsert{
		RuntimeTurnID: runtimeTurnID,
		StepIndex:     stepIndex,
		CommitPayload: commitPayload,
	}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("creating runtime step: %s", err)
	}
	return nil
}
