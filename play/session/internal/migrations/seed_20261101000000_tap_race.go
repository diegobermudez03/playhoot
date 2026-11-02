package migrations

import (
	_ "embed"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/play/internal/business/inputrequirements"
	"github.com/diegobermudez03/playhoot/play/session/internal/objectstore"
)

// Tap Race test-data identifiers, so a person testing by hand can create its
// Sessions.
const (
	// seedTapRaceGameUUID is a playable Game whose EXECUTABLE version is the
	// first game built with the Playhoot SDK: one to eight players tap, the
	// first to 20 taps wins, or whoever leads after 30 seconds.
	seedTapRaceGameUUID    = "55555555-5555-4555-8555-555555555555"
	seedTapRaceVersionUUID = "55555555-5555-4555-8555-0000000000e1"

	// seedTapRaceManifest is the manifest the SDK build derived from the
	// game's definition (seeddata/README.md).
	seedTapRaceManifest = `{"schema_version":1,"players":{"min":1,"max":8},"inputs":{},"commands":{"tap":{"payload":{}}},"entrypoints":["update"]}`
)

// seedTapRaceScript and seedTapRaceFrontendPackage are Tap Race's backend
// script (profile sdk-v1) and frontend package, built by the frontend
// repository's game tools (seeddata/README.md).
var (
	//go:embed seeddata/tap_race_server.js
	seedTapRaceScript []byte
	//go:embed seeddata/tap_race_frontend.html
	seedTapRaceFrontendPackage []byte

	seedTapRaceScriptLocator          = objectstore.LocatorFor(backendScriptPrefix, seedTapRaceScript)
	seedTapRaceFrontendPackageLocator = objectstore.LocatorFor(frontendPackagePrefix, seedTapRaceFrontendPackage)
)

// seed20261101000000TapRace adds the Tap Race Game and its EXECUTABLE
// version, to play an SDK-built game end to end (game creation project,
// CHECKLIST B).
func seed20261101000000TapRace() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "seed_20261101000000_tap_race",
		Migrate: func(tx *gorm.DB) error {
			pressOnly := inputrequirements.Requirements{
				UsesPointerPress: true,
				Devices:          inputrequirements.DevicesFor(false, false, true),
			}
			if err := pressOnly.Validate(); err != nil {
				return err
			}
			statements := []struct {
				sql  string
				args []any
			}{
				{sql: `INSERT INTO play_games (game_uuid) VALUES (?)`, args: []any{seedTapRaceGameUUID}},
				{
					sql: `INSERT INTO play_game_versions (uuid, game_id, status, manifest,
							script_object_key, script_sha256, script_size, script_profile,
							frontend_object_key, frontend_sha256, frontend_size, aspect_ratio, executable_at)
						SELECT ?, id, 'EXECUTABLE', ?, ?, ?, ?, 'sdk-v1', ?, ?, ?, '16:9', CURRENT_TIMESTAMP FROM play_games WHERE game_uuid = ?`,
					args: []any{
						seedTapRaceVersionUUID, seedTapRaceManifest,
						seedTapRaceScriptLocator.ObjectKey, seedTapRaceScriptLocator.SHA256, seedTapRaceScriptLocator.Size,
						seedTapRaceFrontendPackageLocator.ObjectKey, seedTapRaceFrontendPackageLocator.SHA256, seedTapRaceFrontendPackageLocator.Size,
						seedTapRaceGameUUID,
					},
				},
				{
					sql: `INSERT INTO play_game_version_input_requirements
							(game_version_id, uses_keyboard, uses_pointer_hover, uses_pointer_press, devices)
						SELECT id, ?, ?, ?, ? FROM play_game_versions WHERE uuid = ?`,
					args: []any{
						pressOnly.UsesKeyboard, pressOnly.UsesPointerHover, pressOnly.UsesPointerPress, string(pressOnly.Devices),
						seedTapRaceVersionUUID,
					},
				},
				{
					sql: `UPDATE play_games SET current_version_id = (SELECT id FROM play_game_versions WHERE uuid = ?)
						WHERE game_uuid = ?`,
					args: []any{seedTapRaceVersionUUID, seedTapRaceGameUUID},
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
				DELETE FROM play_game_version_input_requirements
				WHERE game_version_id IN (SELECT v.id FROM play_game_versions v JOIN play_games g ON g.id = v.game_id WHERE g.game_uuid = ?)
			`, seedTapRaceGameUUID).Error; err != nil {
				return err
			}
			if err := tx.Exec(`
				DELETE FROM play_game_versions
				WHERE game_id IN (SELECT id FROM play_games WHERE game_uuid = ?)
			`, seedTapRaceGameUUID).Error; err != nil {
				return err
			}
			return tx.Exec(`DELETE FROM play_games WHERE game_uuid = ?`, seedTapRaceGameUUID).Error
		},
	}
}
