// Package repo persists choosing an interface language: the User's current
// language and the change in the settings history.
package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/identity/internal/business/setting"
)

// Repo is choosing an interface language's persistence.
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

// User is a User, the language they read in, and whether they have an
// account.
type User struct {
	ID         int64
	Language   string
	HasAccount bool
}

// LockUser returns the User with userUUID locked for the rest of tx, or nil
// when there is none.
func (r *Repo) LockUser(ctx context.Context, tx *gorm.DB, userUUID string) (*User, error) {
	var users []User
	err := tx.WithContext(ctx).Raw(`
		SELECT users.id, users.interface_language AS language, account.id IS NOT NULL AS has_account
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

type settingChange struct {
	ID        int64
	UserID    int64
	Setting   string
	FromValue string
	ToValue   string
	ChangedAt time.Time
}

func (settingChange) TableName() string { return "identity_setting_changes" }

// ChangeLanguage puts the User in language within tx, keeping the change in
// their settings history.
func (r *Repo) ChangeLanguage(ctx context.Context, tx *gorm.DB, userID int64, fromLanguage string, language string, changedAt time.Time) error {
	if err := tx.WithContext(ctx).Exec(`UPDATE identity_users SET interface_language = ? WHERE id = ?`, language, userID).Error; err != nil {
		return fmt.Errorf("changing interface language: %s", err)
	}
	change := settingChange{UserID: userID, Setting: setting.Language, FromValue: fromLanguage, ToValue: language, ChangedAt: changedAt}
	if err := tx.WithContext(ctx).Create(&change).Error; err != nil {
		return fmt.Errorf("recording interface language change: %s", err)
	}
	return nil
}
