package repo

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// runtimeTurnInsert is the persisted shape of one committed RuntimeTurn.
// source_interaction_id/source_timer_obligation_id/actor_id are left NULL by
// every caller in this WORK (Start) - Slices 3/5 populate them for their own
// causes (`game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md`'s Runtime History
// Tables).
type runtimeTurnInsert struct {
	ID                    uint   `gorm:"column:id"`
	SessionID             uint   `gorm:"column:session_id"`
	Sequence              uint64 `gorm:"column:sequence"`
	SourceKind            string `gorm:"column:source_kind"`
	SnapshotPayload       []byte `gorm:"column:snapshot_payload"`
	SnapshotFormatVersion int    `gorm:"column:snapshot_format_version"`
}

func (runtimeTurnInsert) TableName() string { return "session_runtime_turns" }

// CreateRuntimeTurn persists one committed RuntimeTurn - only the final
// Snapshot after the whole Turn, per GAME-ADR-0007; intermediate Snapshots
// are never separately authoritative. Returns the new row's internal id.
func (r *Repo) CreateRuntimeTurn(ctx context.Context, tx *gorm.DB, sessionID uint, sequence uint64, sourceKind string, snapshotPayload []byte, snapshotFormatVersion int) (uint, error) {
	row := runtimeTurnInsert{
		SessionID:             sessionID,
		Sequence:              sequence,
		SourceKind:            sourceKind,
		SnapshotPayload:       snapshotPayload,
		SnapshotFormatVersion: snapshotFormatVersion,
	}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, fmt.Errorf("creating runtime turn: %s", err)
	}
	return row.ID, nil
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
// a Session-state sequence (GAME-ADR-0007).
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
