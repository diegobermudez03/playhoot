package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func migration20260822000000Games() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260822000000_games",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				CREATE TABLE games (
					id BIGSERIAL PRIMARY KEY,
					uuid UUID NOT NULL UNIQUE,
					name VARCHAR(32) NOT NULL,
					description VARCHAR(255) NOT NULL,
					owner_uuid UUID NOT NULL,
					current_version_id BIGINT NULL,
					logo_image_url TEXT NOT NULL,
					visibility TEXT NOT NULL,
					is_published BOOLEAN NOT NULL DEFAULT FALSE,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					deleted_at TIMESTAMPTZ NULL
				)
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE games`).Error
		},
	}
}
