package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20261029030000IdentityGuestLink records, on a login attempt, the
// guest who started it: if the login creates an account, the account is that
// guest's, keeping their User.
func migration20261029030000IdentityGuestLink() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20261029030000_identity_guest_link",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`ALTER TABLE identity_login_attempts ADD COLUMN guest_user_id BIGINT`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`ALTER TABLE identity_login_attempts DROP COLUMN guest_user_id`).Error
		},
	}
}
