package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// seedSpaceDriftFrontendVersionUUID is Space Drift's second EXECUTABLE
// version: the same manifest and backend script as the first, with the real
// frontend package built with the Playhoot Game SDK (seeddata/README.md)
// instead of the placeholder page. It becomes the Game's current version, so
// new Sessions use it; Sessions already pinned to the first keep it.
const seedSpaceDriftFrontendVersionUUID = "33333333-3333-4333-8333-0000000000c2"

// seed20261020000000SpaceDriftFrontend adds that version, asked by the human
// so the web frontend can play Space Drift end to end.
func seed20261020000000SpaceDriftFrontend() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "seed_20261020000000_space_drift_frontend",
		Migrate: func(tx *gorm.DB) error {
			statements := []struct {
				sql  string
				args []any
			}{
				{
					sql: `INSERT INTO play_game_versions (uuid, game_id, status, manifest,
							script_object_key, script_sha256, script_size,
							frontend_object_key, frontend_sha256, frontend_size, executable_at)
						SELECT ?, id, 'EXECUTABLE', ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP FROM play_games WHERE game_uuid = ?`,
					args: []any{
						seedSpaceDriftFrontendVersionUUID, seedSpaceDriftManifest,
						seedSpaceDriftScriptLocator.ObjectKey, seedSpaceDriftScriptLocator.SHA256, seedSpaceDriftScriptLocator.Size,
						seedSpaceDriftFrontendPackageLocator.ObjectKey, seedSpaceDriftFrontendPackageLocator.SHA256, seedSpaceDriftFrontendPackageLocator.Size,
						seedSpaceDriftGameUUID,
					},
				},
				{
					sql: `UPDATE play_games SET current_version_id = (SELECT id FROM play_game_versions WHERE uuid = ?)
						WHERE game_uuid = ?`,
					args: []any{seedSpaceDriftFrontendVersionUUID, seedSpaceDriftGameUUID},
				},
			}
			for _, statement := range statements {
				if err := tx.Exec(statement.sql, statement.args...).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				UPDATE play_games SET current_version_id = (SELECT id FROM play_game_versions WHERE uuid = ?)
				WHERE game_uuid = ?
			`, seedSpaceDriftVersionUUID, seedSpaceDriftGameUUID).Error; err != nil {
				return err
			}
			return tx.Exec(`DELETE FROM play_game_versions WHERE uuid = ?`, seedSpaceDriftFrontendVersionUUID).Error
		},
	}
}
