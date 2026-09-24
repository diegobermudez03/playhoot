package repo

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// runtimeTurnInsert is the persisted shape of one committed RuntimeTurn.
// SourceInteractionID/ActorID are nil for a Turn with no such cause. There
// is no Snapshot column: a Turn row identifies which durable cause happened
// and in what order, never the resulting state - current/historical Runtime
// state is always reconstructed by replaying those causes instead.
type runtimeTurnInsert struct {
	ID                  uint   `gorm:"column:id"`
	SessionID           uint   `gorm:"column:session_id"`
	Sequence            uint64 `gorm:"column:sequence"`
	SourceKind          string `gorm:"column:source_kind"`
	SourceInteractionID *uint  `gorm:"column:source_interaction_id"`
	ActorID             *uint  `gorm:"column:actor_id"`
}

func (runtimeTurnInsert) TableName() string { return "session_runtime_turns" }

// CreateRuntimeTurn persists one committed RuntimeTurn's replay-input
// envelope. sourceInteractionID/actorID are the Turn's own cause/actor
// references, nil when the Turn has no such cause. Returns the new row's
// internal id.
func (r *Repo) CreateRuntimeTurn(ctx context.Context, tx *gorm.DB, sessionID uint, sequence uint64, sourceKind string, sourceInteractionID *uint, actorID *uint) (uint, error) {
	row := runtimeTurnInsert{
		SessionID:           sessionID,
		Sequence:            sequence,
		SourceKind:          sourceKind,
		SourceInteractionID: sourceInteractionID,
		ActorID:             actorID,
	}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, fmt.Errorf("creating runtime turn: %s", err)
	}
	return row.ID, nil
}

// RuntimeTurn is the persisted facts of one committed RuntimeTurn needed to
// resume execution from it: its own sequence, to compute the next Turn's
// sequence.
type RuntimeTurn struct {
	ID       uint
	Sequence uint64
}

// GetRuntimeTurn loads turnID's persisted facts, for a RUNNING-phase
// mutation resuming execution from the Session's current authoritative
// Turn.
func (r *Repo) GetRuntimeTurn(ctx context.Context, tx *gorm.DB, turnID uint) (*RuntimeTurn, error) {
	var row RuntimeTurn
	result := tx.WithContext(ctx).Raw(`
		SELECT id, sequence
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

// RuntimeTurnRecord is one committed RuntimeTurn's replay-input envelope, as
// needed to reconstruct the engine.Signal that produced it.
type RuntimeTurnRecord struct {
	ID                  uint
	Sequence            uint64
	SourceKind          string
	SourceInteractionID *uint
	ActorID             *uint
}

// ListRuntimeTurns returns every committed RuntimeTurn for sessionID,
// ordered by sequence ascending - the complete, ordered replay-input log a
// deterministic replay folds over to reconstruct current authoritative
// Runtime state. sessions.current_turn_id always points at the last row
// this returns, since a RuntimeTurn is never persisted without immediately
// advancing that pointer in the same transaction.
func (r *Repo) ListRuntimeTurns(ctx context.Context, tx *gorm.DB, sessionID uint) ([]RuntimeTurnRecord, error) {
	var rows []RuntimeTurnRecord
	if err := tx.WithContext(ctx).Raw(`
		SELECT id, sequence, source_kind, source_interaction_id, actor_id
		FROM session_runtime_turns
		WHERE session_id = ?
		ORDER BY sequence ASC
	`, sessionID).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("listing runtime turns: %s", err)
	}
	return rows, nil
}
