// Package sessionlock implements the one shared per-Session DB-locking
// primitive that every LOBBY mutation (Join, Leave, and lazy lobby-expiration
// materialization) must use rather than each inventing its own locking
// query, per the Session Runtime Lobby Lifecycle Contract's serialization
// rule. It is intentionally shaped so a later RUNNING-phase slice can reuse
// it unchanged (GAME-ADR-0018).
//
// This package owns locking only. It reports the locked row's current facts
// (phase, lobby_expires_at, ...); it does not decide lobby-expiration policy
// itself - that decision (and the resulting mutations) belongs to the
// workflow/Manager layer that calls it (see
// `game/session/workflows/sessionlifecycle`'s expiration ownership).
package sessionlock

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Session is the locked snapshot of a sessions row needed by lobby and
// RUNNING-phase mutations. CurrentTurnID is sessions.current_turn_id - the
// current-authoritative-RuntimeTurn pointer colocated on sessions rather
// than a separate session_runtime_state table (GAME-ADR-0023, refining
// GAME-ADR-0007), since every caller reading it here already holds this
// same locked row for per-Session serialization.
type Session struct {
	ID                 uint
	UUID               string
	GameDefinitionUUID string
	HostActorID        *uint
	Phase              string
	LobbyExpiresAt     time.Time
	CurrentTurnID      *uint
	TerminalAt         *time.Time
	TerminalReason     *string
}

// LockByID selects the sessions row FOR UPDATE by its internal id,
// serializing every other lobby mutation attempted against the same
// Session. Callers must invoke this from within an already-open DB
// transaction. Returns nil, nil if no such Session exists.
func LockByID(ctx context.Context, tx *gorm.DB, sessionID uint) (*Session, error) {
	var row Session
	result := tx.WithContext(ctx).Raw(`
		SELECT id, uuid, game_definition_uuid, host_actor_id, phase, lobby_expires_at, current_turn_id, terminal_at, terminal_reason
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

// LockByUUID selects the sessions row FOR UPDATE by its public uuid. Returns
// nil, nil if no such Session exists.
func LockByUUID(ctx context.Context, tx *gorm.DB, sessionUUID string) (*Session, error) {
	var row Session
	result := tx.WithContext(ctx).Raw(`
		SELECT id, uuid, game_definition_uuid, host_actor_id, phase, lobby_expires_at, current_turn_id, terminal_at, terminal_reason
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
