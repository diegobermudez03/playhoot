package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20261102000000IdentityResourceLinks keeps the secret links that let
// whoever holds one act for a User on one resource (business/resourcelink):
// only each token's hash, at most one live link per User and resource, and
// revocation written once.
func migration20261102000000IdentityResourceLinks() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20261102000000_identity_resource_links",
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`CREATE TABLE identity_resource_links (
					id BIGSERIAL PRIMARY KEY,
					user_id BIGINT NOT NULL,
					resource_kind TEXT NOT NULL,
					resource_id TEXT NOT NULL,
					token_sha256 TEXT NOT NULL UNIQUE,
					created_at TIMESTAMPTZ NOT NULL,
					expires_at TIMESTAMPTZ NOT NULL,
					revoked_at TIMESTAMPTZ
				)`,
				`CREATE UNIQUE INDEX identity_resource_links_one_open_idx
					ON identity_resource_links (user_id, resource_kind, resource_id) WHERE revoked_at IS NULL`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE identity_resource_links`).Error
		},
	}
}
