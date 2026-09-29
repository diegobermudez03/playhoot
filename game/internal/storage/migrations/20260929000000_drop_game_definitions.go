package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20260929000000DropGameDefinitions drops
// game_definitions/game_definition_histories and games.current_definition_id:
// these existed only to hold a Game-Language-shaped script per version.
// Session Runtime now owns its own executable-artifact table
// (session_game_version_artifacts) independently of Game Management, and
// nothing reads these tables live anymore.
func migration20260929000000DropGameDefinitions() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260929000000_drop_game_definitions",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`ALTER TABLE games DROP COLUMN current_definition_id`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`DROP TABLE game_definition_histories`).Error; err != nil {
				return err
			}
			return tx.Exec(`DROP TABLE game_definitions`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`
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
			`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`
				CREATE TABLE game_definition_histories (
					id BIGSERIAL PRIMARY KEY,
					game_definition_id BIGINT NOT NULL,
					script JSONB NULL,
					published_at TIMESTAMPTZ NULL,
					disabled_at TIMESTAMPTZ NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
				)
			`).Error; err != nil {
				return err
			}
			return tx.Exec(`ALTER TABLE games ADD COLUMN current_definition_id BIGINT NULL`).Error
		},
	}
}
