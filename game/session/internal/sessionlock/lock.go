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

// Row is the locked snapshot of a sessions row needed by lobby mutations.
type Row struct {
	ID                 uint
	UUID               string
	GameDefinitionUUID string
	HostActorID        *uint
	Phase              string
	LobbyExpiresAt     time.Time
	TerminalAt         *time.Time
	TerminalReason     *string
}

// LockByID selects the sessions row FOR UPDATE by its internal id,
// serializing every other lobby mutation attempted against the same
// Session. Callers must invoke this from within an already-open DB
// transaction. Returns nil, nil if no such Session exists.
func LockByID(ctx context.Context, tx *gorm.DB, sessionID uint) (*Row, error) {
	var row Row
	result := tx.WithContext(ctx).Raw(`
		SELECT id, uuid, game_definition_uuid, host_actor_id, phase, lobby_expires_at, terminal_at, terminal_reason
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
func LockByUUID(ctx context.Context, tx *gorm.DB, sessionUUID string) (*Row, error) {
	var row Row
	result := tx.WithContext(ctx).Raw(`
		SELECT id, uuid, game_definition_uuid, host_actor_id, phase, lobby_expires_at, terminal_at, terminal_reason
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
