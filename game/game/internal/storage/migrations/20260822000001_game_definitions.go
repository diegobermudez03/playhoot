package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func migration20260822000001GameDefinitions() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260822000001_game_definitions",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				CREATE TABLE game_definitions (
					id BIGSERIAL PRIMARY KEY,
					uuid UUID NOT NULL UNIQUE,
					game_id BIGINT NOT NULL,
					version_number BIGINT NOT NULL,
					script JSONB NOT NULL,
					published_at TIMESTAMPTZ NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					disabled_at TIMESTAMPTZ NULL,
					CONSTRAINT game_definitions_game_id_version_number_key
						UNIQUE (game_id, version_number)
				)
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE game_definitions`).Error
		},
	}
}
