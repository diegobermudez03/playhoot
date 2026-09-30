package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20260930000000SessionGameVersionContentLocators moves game version
// content out of the database. session_game_version_artifacts drops its
// inline backend_script/frontend_script text and its assets JSONB list, and
// instead records, for each script, where the immutable object lives in
// private object storage plus its SHA-256 and size so every read can be
// verified. Declared assets get their own table, one row per (version,
// logical key), holding the same locator plus the MIME type; the game only
// ever knows the logical key, never an object key or URL.
//
// The new script columns are NOT NULL with no default, so this migration
// requires session_game_version_artifacts to be empty - it does not backfill
// existing rows, because no populated environment exists.
func migration20260930000000SessionGameVersionContentLocators() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260930000000_session_game_version_content_locators",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				ALTER TABLE session_game_version_artifacts
					DROP COLUMN backend_script,
					DROP COLUMN frontend_script,
					DROP COLUMN assets,
					ADD COLUMN backend_script_key TEXT NOT NULL,
					ADD COLUMN backend_script_sha256 TEXT NOT NULL,
					ADD COLUMN backend_script_size BIGINT NOT NULL,
					ADD COLUMN frontend_script_key TEXT NOT NULL,
					ADD COLUMN frontend_script_sha256 TEXT NOT NULL,
					ADD COLUMN frontend_script_size BIGINT NOT NULL
			`).Error; err != nil {
				return err
			}
			return tx.Exec(`
				CREATE TABLE session_game_version_assets (
					id BIGSERIAL PRIMARY KEY,
					definition_uuid UUID NOT NULL,
					key TEXT NOT NULL,
					kind TEXT NOT NULL,
					object_key TEXT NOT NULL,
					sha256 TEXT NOT NULL,
					size BIGINT NOT NULL,
					content_type TEXT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					CONSTRAINT session_game_version_assets_definition_key UNIQUE (definition_uuid, key),
					CONSTRAINT session_game_version_assets_definition_uuid_fkey
						FOREIGN KEY (definition_uuid) REFERENCES session_game_version_artifacts (definition_uuid)
				)
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`DROP TABLE session_game_version_assets`).Error; err != nil {
				return err
			}
			return tx.Exec(`
				ALTER TABLE session_game_version_artifacts
					DROP COLUMN backend_script_key,
					DROP COLUMN backend_script_sha256,
					DROP COLUMN backend_script_size,
					DROP COLUMN frontend_script_key,
					DROP COLUMN frontend_script_sha256,
					DROP COLUMN frontend_script_size,
					ADD COLUMN backend_script TEXT NOT NULL,
					ADD COLUMN frontend_script TEXT NOT NULL,
					ADD COLUMN assets JSONB NULL
			`).Error
		},
	}
}
