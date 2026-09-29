package repo

import (
	"context"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"
)

// runtimeTurnInsert is the persisted shape of one committed RuntimeTurn.
// SourceTimerObligationID/SourceCauseEventID/ActorID are nil for a Turn
// with no such cause. NewState is the authoritative state immediately after
// this Turn committed, stored opaque - Session Runtime never decodes/
// interprets it beyond passing it through to the Executor as the next
// call's PreviousState.
type runtimeTurnInsert struct {
	ID                      uint            `gorm:"column:id"`
	SessionID               uint            `gorm:"column:session_id"`
	Sequence                uint64          `gorm:"column:sequence"`
	SourceKind              string          `gorm:"column:source_kind"`
	SourceTimerObligationID *uint           `gorm:"column:source_timer_obligation_id"`
	SourceCauseEventID      *uint           `gorm:"column:source_cause_event_id"`
	ActorID                 *uint           `gorm:"column:actor_id"`
	NewState                json.RawMessage `gorm:"column:new_state"`
}

func (runtimeTurnInsert) TableName() string { return "session_runtime_turns" }

// CreateRuntimeTurn persists one committed RuntimeTurn: newState is
// ExecutionOutput.NewState, stored opaque. sourceTimerObligationID/
// sourceCauseEventID/actorID are the Turn's own cause/actor references, nil
// when the Turn has no such cause - at most one of sourceTimerObligationID/
// sourceCauseEventID is ever non-nil (Start's own Turn has neither).
// sourceCauseEventID is always nil at creation time even for a Turn a cause
// event will own: session_cause_events.runtime_turn_id is NOT NULL, so the
// cause event row can only be created after this Turn exists - see
// SetRuntimeTurnCauseEvent, which backfills this column once that row is
// created. Returns the new row's internal id.
func (r *Repo) CreateRuntimeTurn(ctx context.Context, tx *gorm.DB, sessionID uint, sequence uint64, sourceKind string, sourceTimerObligationID *uint, sourceCauseEventID *uint, actorID *uint, newState json.RawMessage) (uint, error) {
	row := runtimeTurnInsert{
		SessionID:               sessionID,
		Sequence:                sequence,
		SourceKind:              sourceKind,
		SourceTimerObligationID: sourceTimerObligationID,
		SourceCauseEventID:      sourceCauseEventID,
		ActorID:                 actorID,
		NewState:                newState,
	}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, fmt.Errorf("creating runtime turn: %s", err)
	}
	return row.ID, nil
}

// SetRuntimeTurnCauseEvent backfills turnID's source_cause_event_id once
// its owning session_cause_events row has been created - see
// CreateRuntimeTurn's own doc comment for why this is a separate step
// rather than supplied at creation time.
func (r *Repo) SetRuntimeTurnCauseEvent(ctx context.Context, tx *gorm.DB, turnID uint, causeEventID uint) error {
	if err := tx.WithContext(ctx).Exec(`
		UPDATE session_runtime_turns SET source_cause_event_id = ? WHERE id = ?
	`, causeEventID, turnID).Error; err != nil {
		return fmt.Errorf("setting runtime turn cause event: %s", err)
	}
	return nil
}

// RuntimeTurn is the persisted facts of one committed RuntimeTurn needed to
// resume execution from it: its own sequence, to compute the next Turn's
// sequence, and NewState, the authoritative state a subsequent Execute call
// passes as PreviousState - "current state" for any operation is simply this
// column for the row sessions.current_turn_id already points at.
type RuntimeTurn struct {
	ID       uint
	Sequence uint64
	NewState json.RawMessage
}

// GetRuntimeTurn loads turnID's persisted facts, for a RUNNING-phase
// mutation resuming execution from the Session's current authoritative
// Turn.
func (r *Repo) GetRuntimeTurn(ctx context.Context, tx *gorm.DB, turnID uint) (*RuntimeTurn, error) {
	var row RuntimeTurn
	result := tx.WithContext(ctx).Raw(`
		SELECT id, sequence, new_state
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
