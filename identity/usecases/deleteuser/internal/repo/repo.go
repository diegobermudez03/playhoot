// Package repo persists deleting an identity: reading the User under lock
// and, in one transaction, ending every way they are reached and every
// credential they hold.
package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/identity/internal/business/setting"
)

// Repo is deletion's persistence.
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

// User is who is being deleted, as the deletion needs them: their row, the
// account they may have, and whether they are already deleted.
type User struct {
	UserID      int64
	DisplayName string
	Language    string
	AccountID   *int64
	Mode        string
	DeletedAt   *time.Time
}

// LockUser returns the User with userUUID, locked for the rest of tx, or nil
// when there is no such User.
func (r *Repo) LockUser(ctx context.Context, tx *gorm.DB, userUUID string) (*User, error) {
	var users []User
	err := tx.WithContext(ctx).Raw(`
		SELECT users.id AS user_id, users.display_name, users.interface_language AS language,
			account.id AS account_id, COALESCE(account.mode, '') AS mode, users.deleted_at
		FROM identity_users users
		LEFT JOIN identity_accounts account ON account.user_id = users.id
		WHERE users.user_uuid = ?
		FOR UPDATE OF users`,
		userUUID,
	).Scan(&users).Error
	if err != nil {
		return nil, fmt.Errorf("locking the User to delete: %s", err)
	}
	if len(users) == 0 {
		return nil, nil
	}
	return &users[0], nil
}

type displayNameChange struct {
	ID        int64
	UserID    int64
	FromName  *string
	ToName    string
	ChangedAt time.Time
}

func (displayNameChange) TableName() string { return "identity_display_name_changes" }

type settingChange struct {
	ID        int64
	UserID    int64
	Setting   string
	FromValue string
	ToValue   string
	ChangedAt time.Time
}

func (settingChange) TableName() string { return "identity_setting_changes" }

// EndAccess closes, within tx, everything that lets userID in: their
// sessions, their sign-in methods, the links and grants they hold and the
// connections an AI holds for them.
func (r *Repo) EndAccess(ctx context.Context, tx *gorm.DB, userID int64, at time.Time) error {
	statements := []struct {
		what      string
		statement string
	}{
		{"deactivating the sessions", `UPDATE identity_sessions SET deactivated_at = ? WHERE user_id = ? AND deactivated_at IS NULL`},
		{"removing the sign-in methods", `UPDATE identity_external_identities SET removed_at = ? WHERE user_id = ? AND removed_at IS NULL`},
		{"revoking the resource links", `UPDATE identity_resource_links SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL`},
	}
	for _, statement := range statements {
		if err := tx.WithContext(ctx).Exec(statement.statement, at, userID).Error; err != nil {
			return fmt.Errorf("%s of a deleted User: %s", statement.what, err)
		}
	}
	err := tx.WithContext(ctx).Exec(`
		UPDATE identity_ai_sessions SET revoked_at = ?, revoke_reason = ?
		WHERE user_id = ? AND revoked_at IS NULL`,
		at, revokeReasonDeleted, userID,
	).Error
	if err != nil {
		return fmt.Errorf("revoking the AI connections of a deleted User: %s", err)
	}
	// A single-use grant is a live token and nothing else: it leaves no
	// history behind it, as the sweep that deletes the expired ones shows.
	err = tx.WithContext(ctx).Exec(`DELETE FROM identity_resource_grants WHERE user_id = ?`, userID).Error
	if err != nil {
		return fmt.Errorf("deleting the resource grants of a deleted User: %s", err)
	}
	// A removed method keeps only that it existed and when it went: the
	// address and the name it was known by are what deletion suppresses.
	err = tx.WithContext(ctx).Exec(`
		UPDATE identity_external_identities SET subject = '', email = NULL, name = NULL WHERE user_id = ?`,
		userID,
	).Error
	if err != nil {
		return fmt.Errorf("clearing the sign-in methods of a deleted User: %s", err)
	}
	return nil
}

// ClearGuestRequest empties, within tx, the name kept with the creation
// request of a guest, which is the other place Identity wrote it.
func (r *Repo) ClearGuestRequest(ctx context.Context, tx *gorm.DB, userUUID string) error {
	err := tx.WithContext(ctx).Exec(
		`UPDATE identity_guest_requests SET display_name = '' WHERE user_uuid = ?`, userUUID,
	).Error
	if err != nil {
		return fmt.Errorf("clearing the creation request of a deleted guest: %s", err)
	}
	return nil
}

// revokeReasonDeleted is why an AI connection ends with its User.
const revokeReasonDeleted = "user_deleted"

// ClearIdentifyingData empties, within tx, what names the User: their display
// name and their interface language, each kept in its own history, and the
// mode their account was in.
func (r *Repo) ClearIdentifyingData(ctx context.Context, tx *gorm.DB, user User, at time.Time) error {
	err := tx.WithContext(ctx).Exec(
		`UPDATE identity_users SET display_name = '', interface_language = '' WHERE id = ?`, user.UserID,
	).Error
	if err != nil {
		return fmt.Errorf("clearing the name of a deleted User: %s", err)
	}
	if user.DisplayName != "" {
		from := user.DisplayName
		change := displayNameChange{UserID: user.UserID, FromName: &from, ToName: "", ChangedAt: at}
		if err := tx.WithContext(ctx).Create(&change).Error; err != nil {
			return fmt.Errorf("recording the cleared name of a deleted User: %s", err)
		}
	}
	if user.Language != "" {
		change := settingChange{UserID: user.UserID, Setting: setting.Language, FromValue: user.Language, ToValue: "", ChangedAt: at}
		if err := tx.WithContext(ctx).Create(&change).Error; err != nil {
			return fmt.Errorf("recording the cleared language of a deleted User: %s", err)
		}
	}
	return nil
}

// ResetMode puts the account of a deleted User back to mode, keeping the
// change in the settings history. The account row stays: it is what says the
// User was never a guest.
func (r *Repo) ResetMode(ctx context.Context, tx *gorm.DB, user User, mode string, at time.Time) error {
	if user.AccountID == nil || user.Mode == mode {
		return nil
	}
	if err := tx.WithContext(ctx).Exec(`UPDATE identity_accounts SET mode = ? WHERE id = ?`, mode, *user.AccountID).Error; err != nil {
		return fmt.Errorf("resetting the mode of a deleted User: %s", err)
	}
	change := settingChange{UserID: user.UserID, Setting: setting.Mode, FromValue: user.Mode, ToValue: mode, ChangedAt: at}
	if err := tx.WithContext(ctx).Create(&change).Error; err != nil {
		return fmt.Errorf("recording the reset mode of a deleted User: %s", err)
	}
	return nil
}

// MarkDeleted writes, within tx, the deletion itself: when it happened and
// what caused it, once and never again.
func (r *Repo) MarkDeleted(ctx context.Context, tx *gorm.DB, userID int64, origin string, at time.Time) error {
	err := tx.WithContext(ctx).Exec(`
		UPDATE identity_users SET deleted_at = ?, deleted_origin = ?
		WHERE id = ? AND deleted_at IS NULL`,
		at, origin, userID,
	).Error
	if err != nil {
		return fmt.Errorf("marking a User deleted: %s", err)
	}
	return nil
}
