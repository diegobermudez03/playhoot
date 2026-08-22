package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func migration20260822000003GameHistories() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260822000003_game_histories",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				CREATE TABLE game_histories (
					id BIGSERIAL PRIMARY KEY,
					game_id BIGINT NOT NULL REFERENCES games (id),
					name VARCHAR(32) NULL,
					description VARCHAR(255) NULL,
					logo_image_url TEXT NULL,
					visibility TEXT NULL,
					is_published BOOLEAN NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
				)
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE game_histories`).Error
		},
	}
}
