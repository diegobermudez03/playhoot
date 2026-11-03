package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20261026000000PlayGameVersionAssets stores the media a game
// version's frontend uses, by logical key (play/docs/DATA_MODEL.md). Playhoot's
// asset rules are enforced by the application (business/gameversion), not
// here. A row without object_key records an asset that no longer exists in
// storage.
func migration20261026000000PlayGameVersionAssets() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20261026000000_play_game_version_assets",
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`CREATE TABLE play_game_version_assets (
					id BIGSERIAL PRIMARY KEY,
					game_version_id BIGINT NOT NULL,
					key TEXT NOT NULL,
					type TEXT NOT NULL,
					mime_type TEXT NOT NULL,
					object_key TEXT NULL,
					sha256 TEXT NOT NULL,
					size BIGINT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					UNIQUE (game_version_id, key)
				)`,
				// Cleanup finds whether any version still uses a stored object.
				`CREATE INDEX play_game_version_assets_sha256_idx ON play_game_version_assets (sha256)`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE play_game_version_assets`).Error
		},
	}
}
