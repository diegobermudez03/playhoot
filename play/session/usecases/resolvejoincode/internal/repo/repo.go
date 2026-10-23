// Package repo is resolvejoincode's persistence: it reads join codes and the
// Sessions they were given to, and decides nothing itself.
package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Repo is resolvejoincode's persistence implementation.
type Repo struct {
	db *gorm.DB
}

// New constructs a Repo bound to db.
func New(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

// JoinCode is a join code's assignment to a Session, with that Session's
// current status.
type JoinCode struct {
	SessionUUID   string
	SessionStatus string
	ExpiresAt     time.Time
}

// FindLatestJoinCode returns the most recent assignment of code, released or
// not, or nil when it was never given out. A code is only given again once
// its previous assignment was released, so the most recent one is current.
func (r *Repo) FindLatestJoinCode(ctx context.Context, code string) (*JoinCode, error) {
	var joinCode JoinCode
	result := r.db.WithContext(ctx).Raw(`
		SELECT s.uuid AS session_uuid, s.status AS session_status, j.expires_at
		FROM play_session_join_codes j
		JOIN play_sessions s ON s.id = j.session_id
		WHERE j.code = ?
		ORDER BY j.id DESC
		LIMIT 1
	`, code).Scan(&joinCode)
	if result.Error != nil {
		return nil, fmt.Errorf("finding join code: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &joinCode, nil
}
