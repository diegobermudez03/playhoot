package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20261107000000IdentityResourceGrants keeps single-use grants
// (business/resourcegrant): only each token's hash, what it allows (a
// resource and a scope the caller wrote), when it expires and when it was
// used. A use the caller could not complete is released, so used_at may be
// cleared again.
func migration20261107000000IdentityResourceGrants() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20261107000000_identity_resource_grants",
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`CREATE TABLE identity_resource_grants (
					id BIGSERIAL PRIMARY KEY,
					uuid UUID NOT NULL UNIQUE,
					user_id BIGINT NOT NULL,
					resource_kind TEXT NOT NULL,
					resource_id TEXT NOT NULL,
					scope TEXT NOT NULL,
					token_sha256 TEXT NOT NULL UNIQUE,
					created_at TIMESTAMPTZ NOT NULL,
					expires_at TIMESTAMPTZ NOT NULL,
					used_at TIMESTAMPTZ NULL
				)`,
				// Counting a User's live grants, and deleting their expired ones.
				`CREATE INDEX identity_resource_grants_user_idx ON identity_resource_grants (user_id, expires_at)`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE identity_resource_grants`).Error
		},
	}
}
