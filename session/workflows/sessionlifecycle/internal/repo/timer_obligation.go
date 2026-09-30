package repo

import (
	"context"
	"fmt"

	"github.com/diegobermudez03/playhoot/session"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TimerObligation is the persisted session_timer_obligations record for one
// scheduled timer instance. Timer is the opaque identifier a script's
// ScheduleTimer/CancelTimer commands address it by; Data is the opaque
// payload a script optionally attached at scheduling time, echoed back
// unchanged on expiration - it never participates in a timer's identity.
type TimerObligation struct {
	ID              uint
	UUID            string
	SessionID       uint
	Timer           string
	Data            []byte
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
		SELECT id, uuid, session_id, timer, data, delay_ms, state,
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

type timerObligationInsert struct {
	ID              uint   `gorm:"column:id"`
	UUID            string `gorm:"column:uuid"`
	SessionID       uint   `gorm:"column:session_id"`
	Timer           string `gorm:"column:timer"`
	Data            []byte `gorm:"column:data"`
	DelayMs         int64  `gorm:"column:delay_ms"`
	State           string `gorm:"column:state"`
	CreatedByTurnID uint   `gorm:"column:created_by_turn_id"`
}

func (timerObligationInsert) TableName() string { return "session_timer_obligations" }

// CreateTimerObligation persists a newly scheduled ACTIVE timer obligation
// for timer, requested by the committed RuntimeTurn createdByTurnID's own
// ScheduleTimer command. data is the command's own opaque payload, echoed
// back unchanged on expiration - it is never part of timer's identity.
func (r *Repo) CreateTimerObligation(ctx context.Context, tx *gorm.DB, sessionID uint, timer string, data []byte, delayMs int64, createdByTurnID uint) (uint, error) {
	row := timerObligationInsert{
		UUID:            uuid.NewString(),
		SessionID:       sessionID,
		Timer:           timer,
		Data:            data,
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
// matching (sessionID, timer) - the same identifier a CancelTimer command
// addresses. A CancelTimer with no currently-ACTIVE match is a no-op,
// matching that command's own documented "idempotent when nothing is
// currently scheduled" contract.
func (r *Repo) CancelActiveTimerObligation(ctx context.Context, tx *gorm.DB, sessionID uint, timer string, closedByTurnID uint) error {
	if err := tx.WithContext(ctx).Exec(`
		UPDATE session_timer_obligations
		SET state = ?, closed_by_turn_id = ?
		WHERE session_id = ? AND timer = ? AND state = ?
	`, session.TimerObligationStateCancelled, closedByTurnID, sessionID, timer, session.TimerObligationStateActive).Error; err != nil {
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
