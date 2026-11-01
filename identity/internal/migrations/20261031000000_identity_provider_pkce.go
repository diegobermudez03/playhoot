package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20261031000000IdentityProviderPKCE keeps, on a login attempt, the
// PKCE verifier the server uses toward the provider (Google), written when the
// attempt starts and sent when its code is exchanged.
func migration20261031000000IdentityProviderPKCE() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20261031000000_identity_provider_pkce",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`ALTER TABLE identity_login_attempts ADD COLUMN provider_code_verifier TEXT NOT NULL DEFAULT ''`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`ALTER TABLE identity_login_attempts DROP COLUMN provider_code_verifier`).Error
		},
	}
}
