package repo

import (
	"context"
	"fmt"

	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TimerObligation is the persisted session_timer_obligations record for one
// scheduled timer instance - the existing single-pending-timer TimerSlot
// shape (EngineKey nil) or a KeyedTimerSlot instance (EngineKey the
// engineservice-encoded authored key).
type TimerObligation struct {
	ID              uint
	UUID            string
	SessionID       uint
	EngineSlot      string
	EngineKey       []byte
	DelayMs         int64
	State           string
	CreatedByTurnID uint
	ClosedByTurnID  *uint
}

// ResolveSessionForTimerObligation is an unlocked, pre-transaction lookup:
// it lets the Manager learn which Session to lock before opening the
// mutation transaction, without itself committing to the obligation row's
// full current state - that is re-read under lock, by
// FindTimerObligationByUUID, once serialization is actually acquired.
// Mirrors ResolveSessionForInteraction exactly.
func (r *Repo) ResolveSessionForTimerObligation(ctx context.Context, timerObligationUUID string) (*uint, error) {
	var sessionID uint
	result := r.db.WithContext(ctx).Raw(`
		SELECT session_id FROM session_timer_obligations WHERE uuid = ?
	`, timerObligationUUID).Scan(&sessionID)
	if result.Error != nil {
		return nil, fmt.Errorf("resolving session for timer obligation: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &sessionID, nil
}

// FindTimerObligationByUUID re-reads timerObligationUUID's full current row
// inside an already-open transaction. Safe as a plain (non-FOR-UPDATE) read
// once the owning Session's row lock is held, mirroring
// FindInteractionByUUID's own reasoning exactly.
func (r *Repo) FindTimerObligationByUUID(ctx context.Context, tx *gorm.DB, timerObligationUUID string) (*TimerObligation, error) {
	var row TimerObligation
	result := tx.WithContext(ctx).Raw(`
		SELECT id, uuid, session_id, engine_slot, engine_key, delay_ms, state,
			created_by_turn_id, closed_by_turn_id
		FROM session_timer_obligations
		WHERE uuid = ?
	`, timerObligationUUID).Scan(&row)
	if result.Error != nil {
		return nil, fmt.Errorf("finding timer obligation: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &row, nil
}

// GetTimerObligationByID loads timerObligationID's persisted facts by
// internal id - the id a RuntimeTurn's own source_timer_obligation_id
// references - for rebuilding the engine.Signal that obligation's expiration
// drove, during replay reconstruction.
func (r *Repo) GetTimerObligationByID(ctx context.Context, tx *gorm.DB, timerObligationID uint) (*TimerObligation, error) {
	var row TimerObligation
	result := tx.WithContext(ctx).Raw(`
		SELECT id, uuid, session_id, engine_slot, engine_key, delay_ms, state,
			created_by_turn_id, closed_by_turn_id
		FROM session_timer_obligations
		WHERE id = ?
	`, timerObligationID).Scan(&row)
	if result.Error != nil {
		return nil, fmt.Errorf("getting timer obligation: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &row, nil
}

type timerObligationInsert struct {
	ID              uint   `gorm:"column:id"`
	UUID            string `gorm:"column:uuid"`
	SessionID       uint   `gorm:"column:session_id"`
	EngineSlot      string `gorm:"column:engine_slot"`
	EngineKey       []byte `gorm:"column:engine_key"`
	DelayMs         int64  `gorm:"column:delay_ms"`
	State           string `gorm:"column:state"`
	CreatedByTurnID uint   `gorm:"column:created_by_turn_id"`
}

func (timerObligationInsert) TableName() string { return "session_timer_obligations" }

// CreateTimerObligation persists a newly scheduled ACTIVE timer obligation,
// captured from an engine.ScheduleTimerOutput/ScheduleKeyedTimerOutput
// belonging to the committed RuntimeTurn createdByTurnID. engineKey is nil
// for an ordinary TimerSlot timer, or the engineservice-encoded authored key
// for a KeyedTimerSlot timer.
func (r *Repo) CreateTimerObligation(ctx context.Context, tx *gorm.DB, sessionID uint, engineSlot string, engineKey []byte, delayMs int64, createdByTurnID uint) (uint, error) {
	row := timerObligationInsert{
		UUID:            uuid.NewString(),
		SessionID:       sessionID,
		EngineSlot:      engineSlot,
		EngineKey:       engineKey,
		DelayMs:         delayMs,
		State:           session.TimerObligationStateActive,
		CreatedByTurnID: createdByTurnID,
	}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, fmt.Errorf("creating timer obligation: %s", err)
	}
	return row.ID, nil
}

// CancelActiveTimerObligation cancels the currently ACTIVE obligation
// matching (sessionID, engineSlot, engineKey) - the same tuple an
// engine.CancelTimerOutput/CancelKeyedTimerOutput addresses. engineKey uses
// the same COALESCE-normalized comparison as this table's own partial unique
// index, so an ordinary timer's NULL key matches correctly. A
// CancelTimerOutput/CancelKeyedTimerOutput with no currently-ACTIVE match is
// a no-op (CancelTimerOperation's own documented "idempotent when the slot
// is already empty").
func (r *Repo) CancelActiveTimerObligation(ctx context.Context, tx *gorm.DB, sessionID uint, engineSlot string, engineKey []byte, closedByTurnID uint) error {
	if err := tx.WithContext(ctx).Exec(`
		UPDATE session_timer_obligations
		SET state = ?, closed_by_turn_id = ?
		WHERE session_id = ? AND engine_slot = ? AND state = ?
			AND COALESCE(engine_key, 'null'::jsonb) = COALESCE(?::jsonb, 'null'::jsonb)
	`, session.TimerObligationStateCancelled, closedByTurnID, sessionID, engineSlot, session.TimerObligationStateActive, engineKey).Error; err != nil {
		return fmt.Errorf("cancelling active timer obligation: %s", err)
	}
	return nil
}

// CloseTimerObligation closes timerObligationID as consumed by its own
// expiration (Manager.ExpireTimer), as Turn-produced closure - distinct from
// CancelActiveTimerObligation, which closes a *different* obligation an
// authored Cancel(Keyed)TimerOperation targets.
func (r *Repo) CloseTimerObligation(ctx context.Context, tx *gorm.DB, timerObligationID uint, closedByTurnID uint) error {
	if err := tx.WithContext(ctx).Exec(`
		UPDATE session_timer_obligations SET state = ?, closed_by_turn_id = ? WHERE id = ?
	`, session.TimerObligationStateConsumed, closedByTurnID, timerObligationID).Error; err != nil {
		return fmt.Errorf("closing timer obligation: %s", err)
	}
	return nil
}

// CancelAllActiveTimerObligationsForSession atomically cancels every
// still-ACTIVE timer obligation for sessionID as terminal cleanup:
// closed_by_turn_id stays NULL and closure_reason records reason, mirroring
// CloseAllActiveInteractionsForSession exactly - this is Session lifecycle
// cleanup, not gameplay, so a TERMINAL Session never retains an obligation
// that could still fire.
func (r *Repo) CancelAllActiveTimerObligationsForSession(ctx context.Context, tx *gorm.DB, sessionID uint, reason string) error {
	if err := tx.WithContext(ctx).Exec(`
		UPDATE session_timer_obligations
		SET state = ?, closure_reason = ?
		WHERE session_id = ? AND state = ?
	`, session.TimerObligationStateCancelled, reason, sessionID, session.TimerObligationStateActive).Error; err != nil {
		return fmt.Errorf("cancelling active timer obligations for session: %s", err)
	}
	return nil
}
