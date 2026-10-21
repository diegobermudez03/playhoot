package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// seed20261022000000AspectRatios gives the EXECUTABLE test versions seeded
// before aspect ratios existed the ratio they were made for, decided by the
// human: Space Drift (and the placeholder playable game) landscape, Parqués's
// board square.
func seed20261022000000AspectRatios() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "seed_20261022000000_aspect_ratios",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`UPDATE play_game_versions SET aspect_ratio = '16:9' WHERE uuid IN (?, ?, ?)`,
				"11111111-1111-4111-8111-0000000000a1", seedSpaceDriftVersionUUID, seedSpaceDriftFrontendVersionUUID).Error; err != nil {
				return err
			}
			return tx.Exec(`UPDATE play_game_versions SET aspect_ratio = '1:1' WHERE uuid = ?`, seedParquesVersionUUID).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`UPDATE play_game_versions SET aspect_ratio = NULL WHERE uuid IN (?, ?, ?, ?)`,
				"11111111-1111-4111-8111-0000000000a1", seedSpaceDriftVersionUUID, seedSpaceDriftFrontendVersionUUID, seedParquesVersionUUID).Error
		},
	}
}
