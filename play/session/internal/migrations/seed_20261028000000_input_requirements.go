package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/play/internal/business/inputrequirements"
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
			pressOnly := inputrequirements.Requirements{
				UsesPointerPress: true,
				Devices:          inputrequirements.DevicesFor(false, false, true),
			}
			if err := pressOnly.Validate(); err != nil {
				return err
			}
			return tx.Exec(`INSERT INTO play_game_version_input_requirements
					(game_version_id, uses_keyboard, uses_pointer_hover, uses_pointer_press, devices)
				SELECT id, ?, ?, ?, ? FROM play_game_versions WHERE uuid IN ?`,
				pressOnly.UsesKeyboard, pressOnly.UsesPointerHover, pressOnly.UsesPointerPress, string(pressOnly.Devices),
				seedInputRequirementsVersionUUIDs).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DELETE FROM play_game_version_input_requirements
				WHERE game_version_id IN (SELECT id FROM play_game_versions WHERE uuid IN ?)`,
				seedInputRequirementsVersionUUIDs).Error
		},
	}
}
