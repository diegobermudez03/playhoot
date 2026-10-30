// Package repo persists refreshing: reading a session's refresh token under
// lock and rotating it.
package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Repo is refreshing's persistence.
type Repo struct {
	db *gorm.DB
}

// New constructs a Repo on db.
func New(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

// GetDB returns the database, for transactions.
func (r *Repo) GetDB() *gorm.DB {
	return r.db
}

// Session is one refresh token of a session, with its User.
type Session struct {
	ID            int64
	SessionUUID   string
	RefreshToken  string
	IssuedAt      time.Time
	StartedAt     time.Time
	ExpiresAt     *time.Time
	DeactivatedAt *time.Time
	ReplacedByID  *int64

	UserID      int64
	UserUUID    string
	DisplayName string
	// HasAccount is true when the User registered an account.
	HasAccount bool
}

const sessionColumns = `
	session.id, session.session_uuid, session.refresh_token, session.issued_at, session.started_at,
	session.expires_at, session.deactivated_at, session.replaced_by_id,
	session.user_id, users.user_uuid, COALESCE(users.display_name, '') AS display_name, account.id IS NOT NULL AS has_account`

const sessionJoins = `
	FROM identity_sessions session
	JOIN identity_users users ON users.id = session.user_id
	LEFT JOIN identity_accounts account ON account.user_id = session.user_id`

// LockSessionByRefreshToken returns the session row of refreshToken locked for
// the rest of tx, or nil when there is none.
func (r *Repo) LockSessionByRefreshToken(ctx context.Context, tx *gorm.DB, refreshToken string) (*Session, error) {
	return lockSession(ctx, tx, `session.refresh_token = ?`, refreshToken)
}

// LockSessionByID returns the session row id locked for the rest of tx, or nil
// when there is none.
func (r *Repo) LockSessionByID(ctx context.Context, tx *gorm.DB, id int64) (*Session, error) {
	return lockSession(ctx, tx, `session.id = ?`, id)
}

func lockSession(ctx context.Context, tx *gorm.DB, condition string, value any) (*Session, error) {
	var sessions []Session
	err := tx.WithContext(ctx).Raw(`SELECT`+sessionColumns+sessionJoins+` WHERE `+condition+` FOR UPDATE OF session`, value).
		Scan(&sessions).Error
	if err != nil {
		return nil, fmt.Errorf("locking session: %s", err)
	}
	if len(sessions) == 0 {
		return nil, nil
	}
	return &sessions[0], nil
}

// RotateSession replaces current, which must be its session's active row, by
// a new row with refreshToken issued at issuedAt and expiring at expiresAt
// (nil: never); current is deactivated at issuedAt and points to it.
func (r *Repo) RotateSession(ctx context.Context, tx *gorm.DB, current Session, refreshToken string, issuedAt time.Time, expiresAt *time.Time) error {
	// Deactivated first: only one row per session may be active.
	if err := tx.WithContext(ctx).Exec(`UPDATE identity_sessions SET deactivated_at = ? WHERE id = ?`, issuedAt, current.ID).Error; err != nil {
		return fmt.Errorf("deactivating rotated session row: %s", err)
	}
	var newID int64
	err := tx.WithContext(ctx).Raw(`
		INSERT INTO identity_sessions (session_uuid, user_id, refresh_token, issued_at, started_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING id`,
		current.SessionUUID, current.UserID, refreshToken, issuedAt, current.StartedAt, expiresAt,
	).Scan(&newID).Error
	if err != nil {
		return fmt.Errorf("creating rotated session row: %s", err)
	}
	if err := tx.WithContext(ctx).Exec(`UPDATE identity_sessions SET replaced_by_id = ? WHERE id = ?`, newID, current.ID).Error; err != nil {
		return fmt.Errorf("linking rotated session row: %s", err)
	}
	return nil
}
