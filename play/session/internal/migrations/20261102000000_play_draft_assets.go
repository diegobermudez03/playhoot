package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20261102000000PlayDraftAssets creates the registry of every asset
// object stored, bound or not (object-storage.md); registered_at is when it
// was last registered to be stored, so the cleanup after a publication never
// takes an object being stored for unused, and deleted_at is written by that
// cleanup.
func migration20261102000000PlayDraftAssets() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20261102000000_play_draft_assets",
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`CREATE TABLE play_asset_objects (
					id BIGSERIAL PRIMARY KEY,
					sha256 TEXT NOT NULL UNIQUE,
					size BIGINT NOT NULL,
					mime_type TEXT NOT NULL,
					object_key TEXT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					registered_at TIMESTAMPTZ NOT NULL,
					deleted_at TIMESTAMPTZ NULL
				)`,
				// Serves the cleanup's scan of objects still stored.
				`CREATE INDEX play_asset_objects_cleanup_idx ON play_asset_objects (registered_at) WHERE deleted_at IS NULL`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE play_asset_objects`).Error
		},
	}
}
