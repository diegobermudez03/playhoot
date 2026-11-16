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
func (r *Repo) OpenSession(ctx context.Context, tx *gorm.DB, opening NewSession) error {
	opened := session{
		SessionUUID:  opening.SessionUUID,
		UserID:       opening.UserID,
		RefreshToken: opening.RefreshToken,
		IssuedAt:     opening.IssuedAt,
		StartedAt:    opening.IssuedAt,
		ExpiresAt:    opening.ExpiresAt,
	}
	if err := tx.WithContext(ctx).Create(&opened).Error; err != nil {
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
	return endUntilNoneActive(ctx, tx,
		`UPDATE identity_sessions SET deactivated_at = ? WHERE session_uuid = ? AND deactivated_at IS NULL`,
		`SELECT count(*) FROM identity_sessions WHERE session_uuid = ? AND deactivated_at IS NULL`,
		endedAt, sessionUUIDs[0])
}

// EndSessionsOf deactivates, within tx, every active session of userID.
func (r *Repo) EndSessionsOf(ctx context.Context, tx *gorm.DB, userID int64, endedAt time.Time) error {
	return endUntilNoneActive(ctx, tx,
		`UPDATE identity_sessions SET deactivated_at = ? WHERE user_id = ? AND deactivated_at IS NULL`,
		`SELECT count(*) FROM identity_sessions WHERE user_id = ? AND deactivated_at IS NULL`,
		endedAt, userID)
}

// endUntilNoneActive runs the deactivation until nothing active is left: a
// rotation committing while the update waits on its row leaves a new active
// row the update could not see.
func endUntilNoneActive(ctx context.Context, tx *gorm.DB, deactivate string, count string, endedAt time.Time, key any) error {
	for {
		if err := tx.WithContext(ctx).Exec(deactivate, endedAt, key).Error; err != nil {
			return fmt.Errorf("ending sessions: %s", err)
		}
		var active int64
		if err := tx.WithContext(ctx).Raw(count, key).Scan(&active).Error; err != nil {
			return fmt.Errorf("checking sessions ended: %s", err)
		}
		if active == 0 {
			return nil
		}
	}
}
