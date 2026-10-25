package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20261026100000PlayGameVersionSignedURLs stores, per game version,
// the current batch of signed URLs to its frontend package and assets, shared
// by every Session of the version and replaced as a whole
// (play/docs/DATA_MODEL.md).
func migration20261026100000PlayGameVersionSignedURLs() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20261026100000_play_game_version_signed_urls",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`CREATE TABLE play_game_version_signed_urls (
				id BIGSERIAL PRIMARY KEY,
				game_version_id BIGINT NOT NULL UNIQUE,
				frontend_url TEXT NOT NULL,
				asset_urls JSONB NOT NULL,
				expires_at TIMESTAMPTZ NOT NULL,
				signed_at TIMESTAMPTZ NOT NULL
			)`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE play_game_version_signed_urls`).Error
		},
	}
}
