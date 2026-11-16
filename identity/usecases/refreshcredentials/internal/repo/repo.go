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

// sessionRow is a new refresh token of a session, as inserted.
type sessionRow struct {
	ID           int64
	SessionUUID  string
	UserID       int64
	RefreshToken string
	IssuedAt     time.Time
	StartedAt    time.Time
	ExpiresAt    *time.Time
	LastUsedAt   *time.Time
}

func (sessionRow) TableName() string { return "identity_sessions" }

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
	// LastUsedAt is when this session was last noted as used. It is written
	// rarely on purpose: a refresh every few minutes must not be a write.
	LastUsedAt *time.Time

	UserID      int64
	UserUUID    string
	DisplayName string
	// HasAccount is true when the User registered an account.
	HasAccount bool
	// Mode is the account's mode, empty for a guest.
	Mode string
}

const sessionColumns = `
	session.id, session.session_uuid, session.refresh_token, session.issued_at, session.started_at,
	session.expires_at, session.deactivated_at, session.replaced_by_id, session.last_used_at,
	session.user_id, users.user_uuid, COALESCE(users.display_name, '') AS display_name,
	account.id IS NOT NULL AS has_account, COALESCE(account.mode, '') AS mode`

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
	successor := sessionRow{
		SessionUUID:  current.SessionUUID,
		UserID:       current.UserID,
		RefreshToken: refreshToken,
		IssuedAt:     issuedAt,
		StartedAt:    current.StartedAt,
		ExpiresAt:    expiresAt,
		// The rotation is itself a use: the new row carries it, so no
		// separate statement notes it.
		LastUsedAt: &issuedAt,
	}
	if err := tx.WithContext(ctx).Create(&successor).Error; err != nil {
		return fmt.Errorf("creating rotated session row: %s", err)
	}
	if err := tx.WithContext(ctx).Exec(`UPDATE identity_sessions SET replaced_by_id = ? WHERE id = ?`, successor.ID, current.ID).Error; err != nil {
		return fmt.Errorf("linking rotated session row: %s", err)
	}
	return nil
}

// NoteSessionUsed records that the session row id was used at usedAt. The
// caller writes it only when the session has not been noted today: the
// column measures how long a guest has been gone, not every request.
func (r *Repo) NoteSessionUsed(ctx context.Context, tx *gorm.DB, id int64, usedAt time.Time) error {
	if err := tx.WithContext(ctx).Exec(`UPDATE identity_sessions SET last_used_at = ? WHERE id = ?`, usedAt, id).Error; err != nil {
		return fmt.Errorf("noting the session as used: %s", err)
	}
	return nil
}
