// Package repo is checkplayerjoin's persistence: it reads the Session facts
// the eligibility decision needs and decides nothing itself.
package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Repo is checkplayerjoin's persistence implementation.
type Repo struct {
	db *gorm.DB
}

// New constructs a Repo bound to db.
func New(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

// Session is what player-join eligibility needs to know about a Session.
type Session struct {
	Status         string
	LobbyExpiresAt time.Time
}

// FindSession returns the Session identified by sessionUUID, or nil when there
// is none.
func (r *Repo) FindSession(ctx context.Context, sessionUUID string) (*Session, error) {
	var session Session
	result := r.db.WithContext(ctx).Raw(`
		SELECT status, lobby_expires_at
		FROM play_sessions
		WHERE uuid = ?
	`, sessionUUID).Scan(&session)
	if result.Error != nil {
		return nil, fmt.Errorf("finding session: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &session, nil
}
