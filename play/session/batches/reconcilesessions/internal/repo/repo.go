// Package repo is reconcilesessions' persistence: it finds and locks the
// Sessions past a deadline and ends the ones the service decides to, and
// decides nothing itself.
package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Repo is reconcilesessions' persistence implementation. Its mutations take
// the tx the service opened; its unlocked reads use the plain db handle.
type Repo struct {
	db *gorm.DB
}

// New constructs a Repo bound to db.
func New(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

// GetDB satisfies utils.DBServicer: the service opens its transactions on it.
func (r *Repo) GetDB() *gorm.DB {
	return r.db
}

// Session is a locked Session as reconciliation judges it.
type Session struct {
	ID             uint
	Status         string
	LobbyExpiresAt time.Time
	ExpiresAt      time.Time
	// OwnerLeaseExpiresAt is when its owner lease runs out; nil if it has
	// none.
	OwnerLeaseExpiresAt *time.Time
	// ReplayFinalized means its replay's final chunk is stored.
	ReplayFinalized bool
}

// FindDue returns up to limit Sessions past a deadline by now, the longest
// overdue first: in lobbyStatus past their lobby expiry, or in runningStatus
// past their lifetime or their owner lease. Each part reads one partial index,
// which only holds Sessions still in that status. It locks nothing.
func (r *Repo) FindDue(ctx context.Context, lobbyStatus string, runningStatus string, now time.Time, limit int) ([]string, error) {
	var uuids []string
	result := r.db.WithContext(ctx).Raw(`
		SELECT uuid FROM (
			(SELECT uuid, lobby_expires_at AS due_at FROM play_sessions
				WHERE status = ? AND lobby_expires_at <= ? ORDER BY lobby_expires_at LIMIT ?)
			UNION ALL
			(SELECT uuid, expires_at FROM play_sessions
				WHERE status = ? AND expires_at <= ? ORDER BY expires_at LIMIT ?)
			UNION ALL
			(SELECT uuid, owner_lease_expires_at FROM play_sessions
				WHERE status = ? AND owner_lease_expires_at <= ? ORDER BY owner_lease_expires_at LIMIT ?)
		) due
		GROUP BY uuid
		ORDER BY MIN(due_at)
		LIMIT ?
	`, lobbyStatus, now, limit, runningStatus, now, limit, runningStatus, now, limit, limit).Scan(&uuids)
	if result.Error != nil {
		return nil, fmt.Errorf("finding sessions past a deadline: %w", result.Error)
	}
	return uuids, nil
}

// LockSession locks sessionUUID's row until tx ends, waiting at most
// lockTimeout for a transaction holding it, and returns it; nil when there is
// no such Session.
func (r *Repo) LockSession(ctx context.Context, tx *gorm.DB, sessionUUID string, lockTimeout time.Duration) (*Session, error) {
	// SET cannot take parameters; the value is an integer the service set.
	if err := tx.WithContext(ctx).Exec(fmt.Sprintf(`SET LOCAL lock_timeout = '%dms'`, lockTimeout.Milliseconds())).Error; err != nil {
		return nil, fmt.Errorf("setting lock timeout: %w", err)
	}
	var locked Session
	result := tx.WithContext(ctx).Raw(`
		SELECT s.id, s.status, s.lobby_expires_at, s.expires_at, s.owner_lease_expires_at,
			EXISTS (SELECT 1 FROM play_replays r WHERE r.session_id = s.id AND r.finalized_at IS NOT NULL) AS replay_finalized
		FROM play_sessions s
		WHERE s.uuid = ?
		FOR UPDATE OF s
	`, sessionUUID).Scan(&locked)
	if result.Error != nil {
		return nil, fmt.Errorf("locking session to reconcile: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &locked, nil
}

// EndSession moves sessionID from fromStatus to toStatus for endCause, as the
// platform's change, appends it to the Session's status history and releases
// its join codes. It fails when the Session is not in fromStatus.
func (r *Repo) EndSession(ctx context.Context, tx *gorm.DB, sessionID uint, fromStatus string, toStatus string, endCause string, at time.Time) error {
	result := tx.WithContext(ctx).Exec(`
		UPDATE play_sessions
		SET status = ?, end_cause = ?, updated_at = ?
		WHERE id = ? AND status = ?
	`, toStatus, endCause, at, sessionID, fromStatus)
	if result.Error != nil {
		return fmt.Errorf("ending session: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("session %d not found in status %s", sessionID, fromStatus)
	}
	// The history is written in the same transaction as every status change.
	if err := tx.WithContext(ctx).Exec(`
		INSERT INTO play_session_status_changes (session_id, from_status, to_status, end_cause, changed_by_user_uuid, changed_at)
		VALUES (?, ?, ?, ?, NULL, ?)
	`, sessionID, fromStatus, toStatus, endCause, at).Error; err != nil {
		return fmt.Errorf("recording status change: %w", err)
	}
	if err := tx.WithContext(ctx).Exec(`
		UPDATE play_session_join_codes SET released_at = ? WHERE session_id = ? AND released_at IS NULL
	`, at, sessionID).Error; err != nil {
		return fmt.Errorf("releasing join codes: %w", err)
	}
	return nil
}
