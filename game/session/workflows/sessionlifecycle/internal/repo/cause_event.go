package repo

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// CauseEvent is the persisted session_cause_events record for one
// RuntimeTurn-producing cause that is a pure immutable occurrence with no
// existing normalized home (a submitted user intent today; a manual
// cancellation or a disconnect/reconnect occurrence later) - see the
// session_cause_events migration's own doc comment for why
// session_interactions/session_timer_obligations are not reused for this
// instead.
type CauseEvent struct {
	ID            uint
	SessionID     uint
	RuntimeTurnID uint
	CauseKind     string
	ActorID       *uint
	Payload       []byte
}

type causeEventInsert struct {
	ID            uint   `gorm:"column:id"`
	SessionID     uint   `gorm:"column:session_id"`
	RuntimeTurnID uint   `gorm:"column:runtime_turn_id"`
	CauseKind     string `gorm:"column:cause_kind"`
	ActorID       *uint  `gorm:"column:actor_id"`
	Payload       []byte `gorm:"column:payload"`
}

func (causeEventInsert) TableName() string { return "session_cause_events" }

// CreateCauseEvent persists a new session_cause_events row for
// runtimeTurnID, which must already exist (session_cause_events.
// runtime_turn_id is NOT NULL) - see
// (*Repo).SetRuntimeTurnCauseEvent for backfilling the Turn's own pointer
// back to this row afterward. Returns the new row's internal id.
func (r *Repo) CreateCauseEvent(ctx context.Context, tx *gorm.DB, sessionID uint, runtimeTurnID uint, causeKind string, actorID *uint, payload []byte) (uint, error) {
	row := causeEventInsert{
		SessionID:     sessionID,
		RuntimeTurnID: runtimeTurnID,
		CauseKind:     causeKind,
		ActorID:       actorID,
		Payload:       payload,
	}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, fmt.Errorf("creating cause event: %s", err)
	}
	return row.ID, nil
}

// GetCauseEventByID loads causeEventID's persisted facts by internal id -
// the id a RuntimeTurn's own source_cause_event_id references - for
// rebuilding the engine.Signal that cause drove, during replay
// reconstruction.
func (r *Repo) GetCauseEventByID(ctx context.Context, tx *gorm.DB, causeEventID uint) (*CauseEvent, error) {
	var row CauseEvent
	result := tx.WithContext(ctx).Raw(`
		SELECT id, session_id, runtime_turn_id, cause_kind, actor_id, payload
		FROM session_cause_events
		WHERE id = ?
	`, causeEventID).Scan(&row)
	if result.Error != nil {
		return nil, fmt.Errorf("getting cause event: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &row, nil
}
