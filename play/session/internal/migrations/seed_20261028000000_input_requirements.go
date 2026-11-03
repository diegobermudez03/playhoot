package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// seedInputRequirementsVersionUUIDs are every seeded game version, DRAFTs
// included. Their games are played by pressing or dragging only (Space
// Drift's controls accept touch), so a desktop or a touch device can play
// them.
var seedInputRequirementsVersionUUIDs = []string{
	"11111111-1111-4111-8111-0000000000a1",
	"11111111-1111-4111-8111-0000000000a2",
	"22222222-2222-4222-8222-0000000000b1",
	seedSpaceDriftVersionUUID,
	seedSpaceDriftFrontendVersionUUID,
	seedSpaceDriftAssetsVersionUUID,
	seedParquesVersionUUID,
}

// seed20261028000000InputRequirements gives the versions seeded before input
// requirements existed the interactions their games use, decided by the
// human.
func seed20261028000000InputRequirements() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "seed_20261028000000_input_requirements",
		Migrate: func(tx *gorm.DB) error {
			versions, err := seedVersionIDs(tx, seedInputRequirementsVersionUUIDs)
			if err != nil {
				return err
			}
			return seedCreateInputRequirements(tx, versions, seedPressOnly())
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DELETE FROM play_game_version_input_requirements
				WHERE game_version_id IN (SELECT id FROM play_game_versions WHERE uuid IN ?)`,
				seedInputRequirementsVersionUUIDs).Error
		},
	}
}
