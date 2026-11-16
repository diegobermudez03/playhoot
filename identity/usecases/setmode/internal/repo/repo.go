// Package repo persists switching mode: the account's current mode and the
// change in the settings history.
package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/identity/internal/business/setting"
)

// Repo is switching mode's persistence.
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

// Account is a User and the mode of their account, when they have one.
type Account struct {
	UserID      int64
	DisplayName string
	AccountID   *int64
	Mode        string
}

// LockAccount returns the User with userUUID and their account locked for
// the rest of tx, or nil when there is no such User.
func (r *Repo) LockAccount(ctx context.Context, tx *gorm.DB, userUUID string) (*Account, error) {
	var accounts []Account
	err := tx.WithContext(ctx).Raw(`
		SELECT users.id AS user_id, COALESCE(users.display_name, '') AS display_name,
			account.id AS account_id, COALESCE(account.mode, '') AS mode
		FROM identity_users users
		LEFT JOIN identity_accounts account ON account.user_id = users.id
		WHERE users.user_uuid = ?
		FOR UPDATE OF users`,
		userUUID,
	).Scan(&accounts).Error
	if err != nil {
		return nil, fmt.Errorf("locking account: %s", err)
	}
	if len(accounts) == 0 {
		return nil, nil
	}
	return &accounts[0], nil
}

type settingChange struct {
	ID        int64
	UserID    int64
	Setting   string
	FromValue string
	ToValue   string
	ChangedAt time.Time
}

func (settingChange) TableName() string { return "identity_setting_changes" }

// ChangeMode puts the account in mode within tx, keeping the change in the
// User's settings history.
func (r *Repo) ChangeMode(ctx context.Context, tx *gorm.DB, accountID int64, userID int64, fromMode string, mode string, changedAt time.Time) error {
	if err := tx.WithContext(ctx).Exec(`UPDATE identity_accounts SET mode = ? WHERE id = ?`, mode, accountID).Error; err != nil {
		return fmt.Errorf("changing mode: %s", err)
	}
	change := settingChange{UserID: userID, Setting: setting.Mode, FromValue: fromMode, ToValue: mode, ChangedAt: changedAt}
	if err := tx.WithContext(ctx).Create(&change).Error; err != nil {
		return fmt.Errorf("recording mode change: %s", err)
	}
	return nil
}
