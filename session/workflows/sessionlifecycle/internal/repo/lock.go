package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Session is the locked snapshot of a sessions row needed by lobby and
// RUNNING-phase mutations. CurrentTurnID is sessions.current_turn_id, the
// current-authoritative-RuntimeTurn pointer. ActivityExpiresAt is
// sessions.activity_expires_at, the deadline by which RUNNING-phase activity
// must renew before the Session is considered inactive - nil before Start
// and meaningless once TERMINAL.
type Session struct {
	ID                 uint
	UUID               string
	GameDefinitionUUID string
	HostActorID        *uint
	Phase              string
	LobbyExpiresAt     time.Time
	ActivityExpiresAt  *time.Time
	CurrentTurnID      *uint
	TerminalAt         *time.Time
	TerminalReason     *string
}

// LockSessionByID and LockSessionByUUID are the one shared per-Session
// DB-locking primitive that every LOBBY mutation (Join, Leave, Start, and
// lazy lobby/activity-expiration materialization) must use rather than each
// inventing its own locking query, so at most one mutation against a given
// Session ever executes at a time. The same primitive is reused unchanged
// for RUNNING-phase mutations (AnswerInteraction, SubmitUserIntent,
// ExpireTimer, CancelSession).
//
// These methods report the locked row's current facts only; they do not
// decide lobby/activity-expiration policy themselves - that decision (and
// the resulting mutations) belongs to the workflow/Manager layer that calls
// them (see internal/expiration, internal/activity).

// LockSessionByID selects the sessions row FOR UPDATE by its internal id,
// serializing every other mutation attempted against the same Session.
// Callers must invoke this from within an already-open DB transaction.
// Returns nil, nil if no such Session exists.
func (r *Repo) LockSessionByID(ctx context.Context, tx *gorm.DB, sessionID uint) (*Session, error) {
	var row Session
	result := tx.WithContext(ctx).Raw(`
		SELECT id, uuid, game_definition_uuid, host_actor_id, phase, lobby_expires_at, activity_expires_at, current_turn_id, terminal_at, terminal_reason
		FROM sessions
		WHERE id = ?
		FOR UPDATE
	`, sessionID).Scan(&row)
	if result.Error != nil {
		return nil, fmt.Errorf("locking session by id: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &row, nil
}

// LockSessionByUUID selects the sessions row FOR UPDATE by its public uuid.
// Returns nil, nil if no such Session exists.
func (r *Repo) LockSessionByUUID(ctx context.Context, tx *gorm.DB, sessionUUID string) (*Session, error) {
	var row Session
	result := tx.WithContext(ctx).Raw(`
		SELECT id, uuid, game_definition_uuid, host_actor_id, phase, lobby_expires_at, activity_expires_at, current_turn_id, terminal_at, terminal_reason
		FROM sessions
		WHERE uuid = ?
		FOR UPDATE
	`, sessionUUID).Scan(&row)
	if result.Error != nil {
		return nil, fmt.Errorf("locking session by uuid: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &row, nil
}
