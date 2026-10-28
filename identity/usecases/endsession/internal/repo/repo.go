// Package repo persists signing out: deactivating a session.
package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Repo is signing out's persistence.
type Repo struct {
	db *gorm.DB
}

// New constructs a Repo on db.
func New(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

// DeactivateSessionOf deactivates, at endedAt, the active row of the session
// refreshToken belongs to (that token or one it replaced). False when there
// was no active row: unknown token, or a session already ended.
func (r *Repo) DeactivateSessionOf(ctx context.Context, refreshToken string, endedAt time.Time) (bool, error) {
	var sessionUUID string
	err := r.db.WithContext(ctx).Raw(`SELECT session_uuid FROM identity_sessions WHERE refresh_token = ?`, refreshToken).
		Scan(&sessionUUID).Error
	if err != nil {
		return false, fmt.Errorf("finding the session: %s", err)
	}
	if sessionUUID == "" {
		return false, nil
	}
	ended := false
	// A rotation committing while the update waits on its row leaves a new
	// active row the update could not see: repeat until none is left.
	for {
		result := r.db.WithContext(ctx).Exec(`
			UPDATE identity_sessions SET deactivated_at = ?
			WHERE session_uuid = ? AND deactivated_at IS NULL`,
			endedAt, sessionUUID,
		)
		if result.Error != nil {
			return false, fmt.Errorf("deactivating session: %s", result.Error)
		}
		ended = ended || result.RowsAffected > 0
		var active int64
		err := r.db.WithContext(ctx).Raw(`
			SELECT count(*) FROM identity_sessions
			WHERE session_uuid = ? AND deactivated_at IS NULL`, sessionUUID).Scan(&active).Error
		if err != nil {
			return false, fmt.Errorf("checking the session ended: %s", err)
		}
		if active == 0 {
			return ended, nil
		}
	}
}
