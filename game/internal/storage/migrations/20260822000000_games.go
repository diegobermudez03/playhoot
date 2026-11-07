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
					name TEXT NOT NULL,
					description TEXT NOT NULL,
					owner_uuid UUID NOT NULL,
					visibility TEXT NOT NULL,
					published_at TIMESTAMPTZ NULL,
					-- Whether its owner lets others start games from it once
					-- published, and whether Playhoot features it to AIs.
					template_allowed BOOLEAN NOT NULL DEFAULT FALSE,
					template_featured BOOLEAN NOT NULL DEFAULT FALSE,
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
