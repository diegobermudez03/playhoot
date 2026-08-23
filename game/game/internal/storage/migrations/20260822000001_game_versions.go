package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func migration20260822000001GameVersions() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260822000001_game_versions",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				CREATE TABLE game_versions (
					id BIGSERIAL PRIMARY KEY,
					uuid UUID NOT NULL UNIQUE,
					game_id BIGINT NOT NULL REFERENCES games (id),
					version_number BIGINT NOT NULL,
					script JSONB NOT NULL,
					published_at TIMESTAMPTZ NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					disabled_at TIMESTAMPTZ NULL,
					CONSTRAINT game_versions_game_id_version_number_key
						UNIQUE (game_id, version_number)
				)
			`).Error; err != nil {
				return err
			}

			return tx.Exec(`
				ALTER TABLE games
				ADD CONSTRAINT games_current_version_id_fkey
				FOREIGN KEY (current_version_id) REFERENCES game_versions (id)
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				ALTER TABLE games
				DROP CONSTRAINT games_current_version_id_fkey
			`).Error; err != nil {
				return err
			}

			return tx.Exec(`DROP TABLE game_versions`).Error
		},
	}
}
