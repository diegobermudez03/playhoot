// Package repo is loadsessionlimits' persistence: it reads a Session's
// deadlines and decides nothing itself.
package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Repo is loadsessionlimits' persistence implementation.
type Repo struct {
	db *gorm.DB
}

// New constructs a Repo bound to db.
func New(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

// Limits are a Session's deadlines as stored.
type Limits struct {
	LobbyExpiresAt time.Time
	ExpiresAt      time.Time
}

// FindSessionLimits returns sessionUUID's deadlines, or nil when there is no
// such Session.
func (r *Repo) FindSessionLimits(ctx context.Context, sessionUUID string) (*Limits, error) {
	var limits Limits
	result := r.db.WithContext(ctx).Raw(`
		SELECT lobby_expires_at, expires_at
		FROM play_sessions
		WHERE uuid = ?
	`, sessionUUID).Scan(&limits)
	if result.Error != nil {
		return nil, fmt.Errorf("finding session limits: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &limits, nil
}
