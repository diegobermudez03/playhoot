package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20261102100000PlayDropAssetUploads drops the asset upload
// reservations: asset bytes now reach the backend in the upload request
// itself, since the backend reads and checks them anyway.
func migration20261102100000PlayDropAssetUploads() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20261102100000_play_drop_asset_uploads",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE IF EXISTS play_asset_uploads`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			statements := []string{
				`CREATE TABLE play_asset_uploads (
					id BIGSERIAL PRIMARY KEY,
					uuid UUID NOT NULL UNIQUE,
					game_version_id BIGINT NOT NULL,
					asset_key TEXT NOT NULL,
					asset_type TEXT NOT NULL,
					staging_object_key TEXT NOT NULL,
					expires_at TIMESTAMPTZ NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE INDEX play_asset_uploads_version_idx ON play_asset_uploads (game_version_id)`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			return nil
		},
	}
}
