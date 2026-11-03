package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/play/runtime/sandbox/scriptcheck"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/gameversion"
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
			game, err := seedGameID(tx, seedSpaceDriftGameUUID)
			if err != nil {
				return err
			}
			version, err := seedCreateVersion(tx, seedExecutableVersion(seedSpaceDriftFrontendVersionUUID, game, seedSpaceDriftManifest,
				seedSpaceDriftScriptLocator, scriptcheck.ProfileHandwritten, seedSpaceDriftFrontendPackageLocator, gameversion.AspectRatio16x9))
			if err != nil {
				return err
			}
			return seedSetCurrentVersion(tx, game, version)
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
