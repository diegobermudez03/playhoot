// Package repo persists choosing a display name: the User's current name and
// its history.
package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Repo is choosing a display name's persistence.
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

// User is a User's current name and whether they have an account.
type User struct {
	ID          int64
	DisplayName *string
	HasAccount  bool
}

// LockUser returns the User with userUUID locked for the rest of tx, or nil
// when there is none.
func (r *Repo) LockUser(ctx context.Context, tx *gorm.DB, userUUID string) (*User, error) {
	var users []User
	err := tx.WithContext(ctx).Raw(`
		SELECT users.id, users.display_name, account.id IS NOT NULL AS has_account
		FROM identity_users users
		LEFT JOIN identity_accounts account ON account.user_id = users.id
		WHERE users.user_uuid = ?
		FOR UPDATE OF users`,
		userUUID,
	).Scan(&users).Error
	if err != nil {
		return nil, fmt.Errorf("locking user: %s", err)
	}
	if len(users) == 0 {
		return nil, nil
	}
	return &users[0], nil
}

// ChangeDisplayName names the User displayName within tx, keeping the change
// in their history.
func (r *Repo) ChangeDisplayName(ctx context.Context, tx *gorm.DB, userID int64, fromName *string, displayName string, changedAt time.Time) error {
	if err := tx.WithContext(ctx).Exec(`UPDATE identity_users SET display_name = ? WHERE id = ?`, displayName, userID).Error; err != nil {
		return fmt.Errorf("changing display name: %s", err)
	}
	err := tx.WithContext(ctx).Exec(`
		INSERT INTO identity_display_name_changes (user_id, from_name, to_name, changed_at)
		VALUES (?, ?, ?, ?)`,
		userID, fromName, displayName, changedAt,
	).Error
	if err != nil {
		return fmt.Errorf("recording display name change: %s", err)
	}
	return nil
}
