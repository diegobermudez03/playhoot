package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// NewSession is an account's first refresh token on a device.
type NewSession struct {
	SessionUUID  string
	UserID       int64
	RefreshToken string
	IssuedAt     time.Time
	ExpiresAt    *time.Time
}

// OpenSession creates the session within tx.
func (r *Repo) OpenSession(ctx context.Context, tx *gorm.DB, session NewSession) error {
	err := tx.WithContext(ctx).Exec(`
		INSERT INTO identity_sessions (session_uuid, user_id, refresh_token, issued_at, started_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		session.SessionUUID, session.UserID, session.RefreshToken, session.IssuedAt, session.IssuedAt, session.ExpiresAt,
	).Error
	if err != nil {
		return fmt.Errorf("opening account session: %s", err)
	}
	return nil
}

// EndSessionOf deactivates, within tx, the active row of the session
// refreshToken belongs to. Nothing when the token is unknown or its session
// already ended.
func (r *Repo) EndSessionOf(ctx context.Context, tx *gorm.DB, refreshToken string, endedAt time.Time) error {
	var sessionUUIDs []string
	err := tx.WithContext(ctx).Raw(`SELECT session_uuid FROM identity_sessions WHERE refresh_token = ?`, refreshToken).
		Scan(&sessionUUIDs).Error
	if err != nil {
		return fmt.Errorf("finding replaced session: %s", err)
	}
	if len(sessionUUIDs) == 0 {
		return nil
	}
	// A rotation committing while the update waits on its row leaves a new
	// active row the update could not see: repeat until none is left.
	for {
		result := tx.WithContext(ctx).Exec(`
			UPDATE identity_sessions SET deactivated_at = ?
			WHERE session_uuid = ? AND deactivated_at IS NULL`,
			endedAt, sessionUUIDs[0],
		)
		if result.Error != nil {
			return fmt.Errorf("ending replaced session: %s", result.Error)
		}
		var active int64
		err := tx.WithContext(ctx).Raw(`
			SELECT count(*) FROM identity_sessions
			WHERE session_uuid = ? AND deactivated_at IS NULL`, sessionUUIDs[0]).Scan(&active).Error
		if err != nil {
			return fmt.Errorf("checking replaced session ended: %s", err)
		}
		if active == 0 {
			return nil
		}
	}
}
