// Package repo is resolvejoincode's persistence: it reads active join codes
// and decides nothing itself.
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

// JoinCode is an active join code and the Session it names.
type JoinCode struct {
	SessionUUID string
	ExpiresAt   time.Time
}

// FindActiveJoinCode returns the unreleased join code code, or nil when there
// is none. At most one unreleased row holds a code.
func (r *Repo) FindActiveJoinCode(ctx context.Context, code string) (*JoinCode, error) {
	var joinCode JoinCode
	result := r.db.WithContext(ctx).Raw(`
		SELECT s.uuid AS session_uuid, j.expires_at
		FROM play_session_join_codes j
		JOIN play_sessions s ON s.id = j.session_id
		WHERE j.code = ? AND j.released_at IS NULL
	`, code).Scan(&joinCode)
	if result.Error != nil {
		return nil, fmt.Errorf("finding join code: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &joinCode, nil
}
