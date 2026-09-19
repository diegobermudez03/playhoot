package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func migration20260822000002GameImages() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260822000002_game_images",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				CREATE TABLE game_images (
					id BIGSERIAL PRIMARY KEY,
					game_id BIGINT NOT NULL,
					image_url TEXT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					removed_at TIMESTAMPTZ NULL
				)
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE game_images`).Error
		},
	}
}
