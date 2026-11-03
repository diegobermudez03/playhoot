package migrations

import (
	_ "embed"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/play/runtime/sandbox/scriptcheck"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/gameversion"
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
			game, err := seedCreateGame(tx, seedTapRaceGameUUID)
			if err != nil {
				return err
			}
			version, err := seedCreateVersion(tx, seedExecutableVersion(seedTapRaceVersionUUID, game, seedTapRaceManifest,
				seedTapRaceScriptLocator, scriptcheck.ProfileSDKV1, seedTapRaceFrontendPackageLocator, gameversion.AspectRatio16x9))
			if err != nil {
				return err
			}
			if err := seedCreateInputRequirements(tx, []uint{version}, seedPressOnly()); err != nil {
				return err
			}
			return seedSetCurrentVersion(tx, game, version)
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
